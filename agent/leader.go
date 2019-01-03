// Copyright 2018 MOBIKE, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/log"
)

var (
	leaderPath       = "master"
	termPath         = join(electionPath, "master", "term")
	idPath           = join(electionPath, "master", "id")
	lastUUIDPath     = join(electionPath, "master", "last_uuid")
	lastGTIDPath     = join(electionPath, "master", "last_gtid")
	lastTxnIDPath    = join(electionPath, "master", "last_txnid")
	metaVersionPath  = join(electionPath, "master", "meta_version")
	mytermPathPrefix = join(electionPath, "terms")
)

// Leader represents the Leader info get from etcd
type Leader struct {
	name string
	kv   *mvccpb.KeyValue

	meta LeaderMeta

	// spm holds the value under /{spmPath},
	// should be empty string or an ID of a node
	spm string
}

// LeaderMeta is the meta-info of the leader, which is persist in etcd, with no expiration
type LeaderMeta struct {
	name      string
	term      uint64
	lastUUID  string
	lastGTID  string
	lastTxnID uint64

	// version is used to identify different LeaderMeta Schema
	// used for backward/forward compatibility
	version int
}

func (s *Server) getLeaderAndMyTerm() (*Leader, uint64, error) {

	client := s.node.RawClient()
	ctx, _ := context.WithTimeout(s.ctx, s.cfg.EtcdDialTimeout)
	leader := &Leader{
		meta: LeaderMeta{},
	}
	var myTerm uint64

	txnResp, err := client.Txn(ctx).Then(
		Concatenate(s.leaderGetOps(), s.myTermGetOps(), s.spmGetOps())...).Commit()
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	for _, resp := range txnResp.Responses {
		kvs := resp.GetResponseRange().GetKvs()
		if len(kvs) == 0 {
			continue
		}
		kv := kvs[0]
		switch string(kv.Key) {
		case s.fullPathOf(leaderPath):
			leader.kv = kv
			leader.name = string(kv.Value)
		case s.fullPathOf(termPath):
			term, err := strconv.Atoi(string(kv.Value))
			if err != nil {
				return nil, 0, errors.Trace(err)
			}
			leader.meta.term = uint64(term)
		case s.fullPathOf(idPath):
			leader.meta.name = string(kv.Value)
		case s.fullPathOf(lastUUIDPath):
			leader.meta.lastUUID = string(kv.Value)
		case s.fullPathOf(lastGTIDPath):
			leader.meta.lastGTID = string(kv.Value)
		case s.fullPathOf(metaVersionPath):
			v, err := strconv.Atoi(string(kv.Value))
			if err != nil {
				log.Errorf("has error in parsing meta version: %s. err: %v",
					string(kv.Value), err)
			} else {
				leader.meta.version = v
			}
		case s.fullPathOf(lastTxnIDPath):
			ltID, err := strconv.ParseUint(string(kv.Value), 10, 64)
			if err != nil {
				log.Errorf("has error in parsing last txnID: %s. err: %v",
					string(kv.Value), err)
			} else {
				leader.meta.lastTxnID = ltID
			}

		case s.fullPathOf(spmPath):
			leader.spm = string(kv.Value)
		case s.fullPathOf(s.myTermPath()):
			myTerm, err = strconv.ParseUint(string(kv.Value), 10, 64)
			if err != nil {
				log.Errorf("fail to parse term %s from etcd", string(kv.Value))
				return nil, 0, errors.NotValidf("fail to parse term %s from etcd", string(kv.Value))
			}
		}
	}
	return leader, myTerm, nil

}

func (s *Server) deleteLeaderKey() error {
	resp, err := s.node.RawClient().Txn(s.ctx).
		If(s.leaderCmp()).
		Then(clientv3.OpDelete(s.fullLeaderPath()),
			clientv3.OpDelete(s.fullPathOf(metaVersionPath))).
		Commit()
	if err != nil {
		return errors.Trace(err)
	}
	if !resp.Succeeded {
		return errors.New("resign leader failed, we are not leader already")
	}
	return nil

}

func (s *Server) isSameLeader(leaderID string) bool {
	return leaderID == s.node.ID()
}

func (s *Server) watchLeader(revision int64) error {
	client := s.node.RawClient()
	watcher := client.NewWatcher()
	defer watcher.Close()

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	for {
		var rch clientv3.WatchChan
		if revision == 0 {
			rch = watcher.Watch(ctx, s.fullLeaderPath())
		} else {
			rch = watcher.Watch(ctx, s.fullLeaderPath(), clientv3.WithRev(revision))
		}
		for watchResp := range rch {
			if err := watchResp.Err(); err != nil {
				if revision == 0 {
					log.Error("watch ", leaderPath, " with the latest revision has an error ", err)
				} else {
					log.Error("watch ", leaderPath, " with revision ", revision, " has an error ", err)
				}
				return err
			}

			if watchResp.Canceled {
				log.Infof("watch '%s' is canceled", leaderPath)
				return nil
			}

			for _, ev := range watchResp.Events {
				if ev.Type == mvccpb.DELETE {
					log.Info("leader is deleted")
					return nil
				}
			}
		}

		// if rch closes, detect whether the ctx is closed.
		select {
		case <-ctx.Done():
			return nil
		default:
			// if the ctx is not closed, try to establish a new watch
			time.Sleep(100 * time.Millisecond)
			log.Info("ctx is not close but watch response channel is closed, try to establish a new watch")
		}
	}
}

// resumeLeader generate a new lease and tries to resume the leader loop with that lease,
// return true and leaseResponse if success
// return false and nil if not success
func (s *Server) resumeLeader() (bool, *clientv3.LeaseGrantResponse, error) {
	client := s.node.RawClient()
	// Init a new lease
	lessor := client.NewLease()
	defer lessor.Close()
	ctx, cancelFunc := context.WithCancel(s.ctx)
	defer cancelFunc()
	lr, err := lessor.Grant(ctx, s.cfg.LeaderLeaseTTL)
	if err != nil {
		log.Error(err)
		return false, nil, errors.Trace(err)
	}

	resp, err := client.Txn(ctx).
		If(s.leaderCmp()).
		Then(s.leaderPutOps(lr.ID, s.term)...).
		Commit()

	if err != nil {
		log.Error("error when resume leader ", err)
		return false, nil, errors.Trace(err)
	}

	if !resp.Succeeded {
		return false, nil, nil
	}
	s.updateLeaseExpireTS()
	return true, lr, nil
}

// campaignLeader tries to campaign the leaser,
// return true and leaseResponse if success
// return false and nil if not success
func (s *Server) campaignLeader() (bool, *clientv3.LeaseGrantResponse, error) {
	client := s.node.RawClient()
	// Init a new lease
	lessor := client.NewLease()
	defer lessor.Close()
	ctx, cancelFunc := context.WithCancel(s.ctx)
	defer cancelFunc()
	lr, err := lessor.Grant(ctx, s.cfg.LeaderLeaseTTL)
	if err != nil {
		log.Error(err)
		return false, nil, errors.Trace(err)
	}

	resp, err := client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(s.fullLeaderPath()), "=", 0)).
		Then(append(s.leaderPutOps(lr.ID, s.term+1), s.myTermPutOps(true)...)...).
		Commit()

	if err != nil {
		log.Info("error when campaign leader ", err)
		return false, nil, errors.Trace(err)
	}

	if !resp.Succeeded {
		return false, nil, nil
	}
	return true, lr, nil
}

func (s *Server) keepLeaderAliveLoop(leaderLeaseID int64) {
	defer func() {
		if atomic.LoadInt32(&s.isClosed) != 1 {
			// if server.isClosed == 1, service must have already been set readonly successfully.
			s.setServiceReadonlyOrShutdown()
		}
	}()

	// start lease expire monitor
	go s.leaseExpireMonitorLoop()

	client := s.node.RawClient()
	// now current node is the leader, keep alive lease and set leader flag to true
	lessor := client.NewLease()
	defer lessor.Close()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	log.Infof("current node is the leader '%s'", s.node.ID())

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(s.ctx, 100*time.Millisecond)
			lkar, err := lessor.KeepAliveOnce(ctx, clientv3.LeaseID(leaderLeaseID))
			cancel()
			for err != nil {
				log.Warn("lease keep alive has error, so retry. leaseID: ", leaderLeaseID, "error: ", err)
				// add retry logic, now only retry once
				time.Sleep(100 * time.Millisecond)
				ctx, cancel = context.WithTimeout(s.ctx, 100*time.Millisecond)
				lkar, err = lessor.KeepAliveOnce(ctx, clientv3.LeaseID(leaderLeaseID))
				cancel()
			}
			s.updateLeaseExpireTS()
			log.Info("lease has remain ttl ", lkar.TTL)
		case <-s.leaderStopCh:
			log.Info("receive leader alive end channel")
			return
		case <-s.ctx.Done():
			log.Warn("server is closed, so exit keepLeaderAlive")
			return
		}
	}
}

func (s *Server) updateLeaseExpireTS() {

	atomic.StoreInt64(&s.leaseExpireTS, time.Now().
		Add(time.Duration(s.cfg.LeaderLeaseTTL)*time.Second).
		Add(-time.Duration(s.cfg.ShutdownThreshold)*time.Second).Unix())

}

// TODO check reset logic
func (s *Server) leaseExpireMonitorLoop() {

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			expire := atomic.LoadInt64(&s.leaseExpireTS)
			now := time.Now().Unix()
			delta := expire - now
			if delta <= 0 {
				log.Error("lease expire ts has reached, lease may be expired, so force close the server")
				log.Errorf("current ts is %d while lease expire ts is %d", now, expire)
				s.forceClose()
			}
			timer.Reset(time.Duration(delta) * time.Second)
		case <-s.leaderStopCh:
			log.Info("receive leader stop channel")
			return
		case <-s.ctx.Done():
			log.Warn("server is closed, so exit leaseExpireMonitorLoop")
			return
		}
	}

}

func (s *Server) leaderCmp() clientv3.Cmp {
	return clientv3.Compare(
		clientv3.Value(s.node.RawClient().KeyWithRootPath(leaderPath)),
		"=",
		leaderValue)
}

func (s *Server) leaderGetOps() []clientv3.Op {
	return []clientv3.Op{clientv3.OpGet(s.fullPathOf(leaderPath)),
		clientv3.OpGet(s.fullPathOf(termPath)),
		clientv3.OpGet(s.fullPathOf(idPath)),
		clientv3.OpGet(s.fullPathOf(metaVersionPath)),
		clientv3.OpGet(s.fullPathOf(lastUUIDPath)),
		clientv3.OpGet(s.fullPathOf(lastTxnIDPath)),
		clientv3.OpGet(s.fullPathOf(lastGTIDPath))}
}

func (s *Server) leaderPutOps(leaseID clientv3.LeaseID, term uint64) []clientv3.Op {
	var opPutLeaderPath clientv3.Op
	var opPutMetaVersionPath clientv3.Op
	if leaseID < 0 {
		opPutLeaderPath = clientv3.OpPut(s.fullPathOf(leaderPath), leaderValue)
		opPutMetaVersionPath = clientv3.OpPut(s.fullPathOf(metaVersionPath), fmt.Sprint(globalSchemaVersion))
	} else {
		opPutLeaderPath = clientv3.OpPut(s.fullPathOf(leaderPath), leaderValue, clientv3.WithLease(leaseID))
		opPutMetaVersionPath = clientv3.OpPut(s.fullPathOf(metaVersionPath),
			fmt.Sprint(globalSchemaVersion), clientv3.WithLease(leaseID))
	}

	return []clientv3.Op{opPutLeaderPath,
		clientv3.OpPut(s.fullPathOf(termPath), fmt.Sprint(term)),
		clientv3.OpPut(s.fullPathOf(idPath), s.node.ID()),
		opPutMetaVersionPath,
		clientv3.OpPut(s.fullPathOf(lastTxnIDPath), fmt.Sprint(s.lastTxnID)),
		clientv3.OpPut(s.fullPathOf(lastUUIDPath), s.lastUUID),
		clientv3.OpPut(s.fullPathOf(lastGTIDPath), s.lastGTID)}
}

func (s *Server) myTermGetOps() []clientv3.Op {
	return []clientv3.Op{clientv3.OpGet(s.fullPathOf(s.myTermPath()))}
}

func (s *Server) myTermPutOps(increment bool) []clientv3.Op {
	var term uint64
	if increment {
		term = s.term + 1
	}
	return []clientv3.Op{clientv3.OpPut(s.fullPathOf(s.myTermPath()), fmt.Sprint(term))}
}

func (s *Server) persistMyTerm() error {
	ctx, _ := context.WithTimeout(s.ctx, s.cfg.EtcdDialTimeout)
	return s.node.RawClient().Put(ctx, s.myTermPath(), fmt.Sprint(s.term))
}

func (s *Server) myTermPath() string {
	return join(mytermPathPrefix, s.node.ID())
}

func (s *Server) fullLeaderPath() string {
	return s.fullPathOf(leaderPath)
}

func (s *Server) fullPathOf(relativePath string) string {
	return s.node.RawClient().KeyWithRootPath(relativePath)
}

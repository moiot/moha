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
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/log"
)

var spmPath = "single_point_master"

// campaignLeaderAsSinglePoint campaigns leader with no lease
func (s *Server) campaignLeaderAsSinglePoint() (bool, error) {
	log.Info("try campaignLeaderAsSinglePoint")

	client := s.node.RawClient()
	ctx, cancelFunc := context.WithCancel(s.ctx)
	defer cancelFunc()

	thenOps := Concatenate(s.leaderPutOps(-1, s.term+1),
		s.myTermPutOps(true),
		// add single_point_master signature
		s.spmPutOps())
	resp, err := client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(s.fullLeaderPath()), "=", 0)).
		Then(thenOps...).
		Commit()

	if err != nil {
		log.Info("error when campaign leader ", err)
		return false, errors.Trace(err)
	}

	if !resp.Succeeded {
		return false, nil
	}
	return true, nil
}

// resumeLeaderAsSinglePoint resumes leader with no lease
func (s *Server) resumeLeaderAsSinglePoint() (bool, error) {
	log.Info("try resumeLeaderAsSinglePoint")

	client := s.node.RawClient()
	ctx, cancelFunc := context.WithCancel(s.ctx)
	defer cancelFunc()

	thenOps := Concatenate(s.leaderPutOps(-1, s.term),
		s.myTermPutOps(false),
		// add single_point_master signature
		s.spmPutOps())
	resp, err := client.Txn(ctx).
		If(s.leaderCmp()).
		Then(thenOps...).
		Commit()

	if err != nil {
		log.Info("error when campaign leader ", err)
		return false, errors.Trace(err)
	}

	if !resp.Succeeded {
		return false, nil
	}
	return true, nil
}

// becomeSinglePointMaster make current node to be the signle point master,
// and blocks until single point master mode terminates
func (s *Server) becomeSinglePointMaster(resume bool) {

	log.Info("enter single point mode")
	defer log.Info("exit single point mode")

	var etcdTxnFunc func() (bool, error)
	if resume {
		etcdTxnFunc = s.resumeLeaderAsSinglePoint
		s.setIsLeaderToTrue()
	} else {
		etcdTxnFunc = s.campaignLeaderAsSinglePoint
	}

	// campaign leader with no lease, till err is nil
	campaignSuccess, err := etcdTxnFunc()
	for err != nil {
		log.Errorf("error while campaignLeaderAsSinglePoint: %+v", err)
		time.Sleep(100 * time.Millisecond)
		campaignSuccess, err = etcdTxnFunc()
	}

	if !campaignSuccess {
		return
	}

	// now success in campaigning, set MySQL readwrite
	log.Info("campaignLeaderAsSinglePoint succeeds")
	defer s.removeSPMFromEtcd()
	err = s.promoteToMaster()
	if err != nil {
		log.Error("fail to set read write, err: ", err)
		s.serviceManager.SetReadOnly()
		return
	}
	writeSPMCh := make(chan interface{})
	go s.writeSPMToLog(writeSPMCh)
	defer s.stopWriteSPMToLog(writeSPMCh)
	s.setIsSPMToTrue()
	defer s.setIsSPMToFalse()
	// after success, watch registryPath for new instance joining
	s.waitUntilSlaveAppears()

	// if registryPath has other agents, exit single point mode by "resumeLeader" procedure
	// so firstly set current MySQL readonly to avoid brain-split
	err = s.serviceManager.SetReadOnly()
	for err != nil {
		log.Error("has error during s.serviceManager.SetReadOnly() when exiting single point mode. ", err)
		time.Sleep(1 * time.Second)
		err = s.serviceManager.SetReadOnly()
	}

}

func (s *Server) waitUntilSlaveAppears() {

	// TODO wait one register lease TTL, so that all partitioned slave are deregistered
	time.Sleep(time.Duration(s.cfg.RegisterTTL) * time.Second)

	for true {
		allAgents, err := s.pollAllRegisteredAgents()
		for err != nil {
			log.Errorf("error when pollAllRegisteredAgents, err is %+v", err)
			time.Sleep(5 * time.Second)
			allAgents, err = s.pollAllRegisteredAgents()
		}
		allAgentsCount := len(allAgents)
		if allAgentsCount > 1 {
			log.Infof("more than one agents appears %+v. so wait %d seconds and check twice", allAgents, s.cfg.RegisterTTL)
			// wait for another register lease TTL, in case that the slave just appears once and then loses connection
			time.Sleep(time.Duration(s.cfg.RegisterTTL) * time.Second)
			log.Info("wait time done, pollAllRegisteredAgents again")
			allAgents, err := s.pollAllRegisteredAgents()
			if err != nil {
				log.Errorf("error when pollAllRegisteredAgents after second TTL wait, err is %+v", err)
				continue
			}
			if len(allAgents) > 1 {
				log.Infof("more than one agents appears %+v. so waitUntilSlaveAppears return", allAgents)
				return
			}
			log.Infof("there are only %d alive agents, so continue single leader mode", len(allAgents))
		}
		time.Sleep(5 * time.Second)
	}
}

func (s *Server) pollAllRegisteredAgents() (map[string][]byte, error) {

	ctx, cancel := context.WithTimeout(s.ctx, s.cfg.EtcdDialTimeout)
	defer cancel()

	return s.node.RawClient().PrefixGet(ctx, registryPath)
}

func (s *Server) writeSPMToLog(writeSPMCh chan interface{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-writeSPMCh:
			log.Info("receive writeSPMCh, stop writeSPMToLog")
			return
		case <-ticker.C:
			log.Infof("server %s is still in single point master mode", s.node.ID())
		}
	}
}

func (s *Server) stopWriteSPMToLog(writeSPMCh chan interface{}) {
	close(writeSPMCh)
}

func (s *Server) removeSPMFromEtcd() error {
	ctx, cancel := context.WithTimeout(s.ctx, s.cfg.EtcdDialTimeout)
	defer cancel()
	return s.node.RawClient().Delete(ctx, spmPath, false)
}

func (s *Server) spmGetOps() []clientv3.Op {
	return []clientv3.Op{clientv3.OpGet(s.fullPathOf(spmPath))}
}

func (s *Server) spmPutOps() []clientv3.Op {
	return []clientv3.Op{clientv3.OpPut(s.fullPathOf(spmPath), s.node.ID())}
}

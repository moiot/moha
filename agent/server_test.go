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
	"database/sql"
	"encoding/json"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/coreos/bbolt"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/integration"
	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/etcd"
	"github.com/moiot/moha/pkg/log" // import for size-effect
	. "gopkg.in/check.v1"
)

var (
	testServerCluster *integration.ClusterV3

	mockClient         *clientv3.Client
	etcdClient         *etcd.Client
	fullLeaderPath     string
	ctx                context.Context
	mockNode           *MockNode
	mockDB             *sql.DB
	mockServiceManager ServiceManager
	mockCfg            *Config

	rootPath = "mock_cluster_nodes_for_leader_test"
)

func init() {
	// set leaderValue
	leaderValue = "unique_id"
}

// TestAgentServer hock into "go test" runner.
func TestAgentServer(t *testing.T) {
	testServerCluster = integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer testServerCluster.Terminate(t)
	SetUp()
	TestingT(t)
}

type testAgentServerSuite struct{}

var _ = Suite(&testAgentServerSuite{})

func SetUp() {

	mockClient = testServerCluster.RandClient()
	etcdClient = etcd.NewClient(mockClient, rootPath)
	fullLeaderPath = etcdClient.KeyWithRootPath(leaderPath)
	ctx, _ = context.WithCancel(context.TODO())
	mockNode = &MockNode{client: etcdClient}

	mockServiceManager = &MockServiceManager{
		masterStatus: &Position{
			File: "mysql-bin.000005",
			Pos:  "188858056",
			GTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
		},
		serverUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		slaveStatus: &Position{
			File:            "mysql-bin.000005",
			Pos:             "188858056",
			GTID:            "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
			SlaveSQLRunning: true,
			SlaveIORunning:  true,
		},
		masterUUID:   "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		executedGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
		endTxnID:     47,
	}

	mockCfg = &Config{
		LeaderLeaseTTL:      10,
		ShutdownThreshold:   3,
		RegisterTTL:         1,
		EtcdUsername:        "root",
		EtcdDialTimeout:     10 * time.Second,
		OnlyFollow:          false,
		InternalServiceHost: leaderValue,
		ExternalServiceHost: leaderValue,
		EtcdURLs:            testServerCluster.HTTPMembers()[0].PeerURLs[0],
	}
	log.Info("etcdurl is ", mockCfg.EtcdURLs)
	cfg := *mockCfg
	cfg.DataDir = "./SetUpSuite"
	cfg.EtcdRootPath = "SetUpSuite"

	s, err := NewServer(&cfg)
	if err != nil {
		log.Errorf("has error in init mock server: %v", err)
		os.Exit(1)
	}
	err = s.node.RawClient().Put(s.ctx, "test_key", "test_value")

	defer os.RemoveAll(cfg.DataDir)

}

func (t *testAgentServerSuite) TearDownSuite(c *C) {}

func (t *testAgentServerSuite) TestServerStart(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	err := s.Start()
	c.Assert(err, NotNil)
}

func (t *testAgentServerSuite) TestUploadPromotionBinlog(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	err := s.uploadPromotionBinlog()
	log.Warn("error is ", err)
	c.Assert(err, IsNil)

	err = s.uploadPromotionBinlog()
	c.Assert(err, IsNil)
}

func (t *testAgentServerSuite) TestUpdateBinlogPosAsMaster(c *C) {

	s := newMockServer()
	s.setIsLeaderToTrue()
	defer os.RemoveAll(s.cfg.DataDir)

	err := s.loadUUID()
	log.Error("error is ", err)
	c.Assert(err, IsNil)
	err = s.updateBinlogPos()
	log.Error("error is ", err)
	c.Assert(err, IsNil)

	c.Assert(latestPos.UUID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2")
	c.Assert(latestPos.File, Equals, "mysql-bin.000005")
	c.Assert(latestPos.Pos, Equals, "188858056")
	c.Assert(latestPos.GTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")
}

func (t *testAgentServerSuite) TestUpdateBinlogPosAsSlave(c *C) {
	s := newMockServer()
	s.setIsLeaderToFalse()
	defer os.RemoveAll(s.cfg.DataDir)

	sm, ok := s.serviceManager.(*MockServiceManager)
	c.Assert(ok, Equals, true)
	sm.masterStatus = &Position{
		File: "mysql-bin.000005",
		Pos:  "188858056",
		GTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
	}
	sm.serverUUID = "53ea0ed1-9bf8-11e6-8bea-64006a897c73"

	err := s.loadUUID()
	c.Assert(err, IsNil)
	err = s.updateBinlogPos()
	log.Error("error is ", err)
	c.Assert(err, IsNil)

	c.Assert(latestPos.UUID, Equals, "53ea0ed1-9bf8-11e6-8bea-64006a897c73")
	c.Assert(latestPos.File, Equals, "mysql-bin.000005")
	c.Assert(latestPos.Pos, Equals, "188858056")
	c.Assert(latestPos.GTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")
}

func (t *testAgentServerSuite) TestLeaderLoop(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("leader loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.leaderLoop()
	}()
	time.Sleep(2 * time.Second)
	s.Close()

}

func (t *testAgentServerSuite) TestLeaderLoopAsFollower(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.node.RawClient().Put(ctx, leaderPath, "another_leader")

	s.leaderStopCh = make(chan interface{})

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("leader loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.leaderLoop()
	}()
	time.Sleep(1 * time.Second)

	s.node.RawClient().Delete(ctx, leaderPath, false)
	time.Sleep(1 * time.Second)

	s.Close()

}

func (t *testAgentServerSuite) TestLeaderLoopAsCampaigner(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.deleteLeaderKey()
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("leader loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.leaderLoop()
	}()
	time.Sleep(1 * time.Second)

	s.Close()

}

func (t *testAgentServerSuite) TestLeaderLoopAsResumer(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.node.RawClient().Put(s.ctx, leaderPath, s.node.ID())
	leader, _, err := s.getLeaderAndMyTerm()
	c.Assert(err, IsNil)

	log.Info("current leader is ", leader)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("leader loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.leaderLoop()
	}()
	time.Sleep(1 * time.Second)
	_, _, err = s.node.RawClient().Get(s.ctx, "single_point_master")
	log.Info("full path of single_point_master is ", s.fullPathOf("single_point_master"))
	log.Info("error is ", err)
	c.Assert(err, NotNil)
	c.Assert(errors.IsNotFound(err), Equals, true)

	s.Close()
}

func (t *testAgentServerSuite) TestLeaderLoopAsSPMResumer(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.node.RawClient().Put(s.ctx, leaderPath, s.node.ID())
	s.node.RawClient().Put(s.ctx, "single_point_master", s.node.ID())
	leader, _, err := s.getLeaderAndMyTerm()
	c.Assert(err, IsNil)

	log.Info("current leader is ", leader)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("leader loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.leaderLoop()
	}()
	time.Sleep(1 * time.Second)
	bs, _, err := s.node.RawClient().Get(s.ctx, "single_point_master")
	c.Assert(err, IsNil)
	c.Assert(string(bs), Equals, s.node.ID())

	s.Close()
}

func (t *testAgentServerSuite) TestDoChangeMasterIsFormerIsNew(c *C) {

	// server depends on etcd and Service manager
	log.Info("begin to test Server.doChangeMaster")
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	// condition 1: current node is the former leader and new leader
	atomic.StoreInt32(&s.isLeader, 1)
	err := s.node.RawClient().Put(ctx, leaderPath, leaderValue)
	c.Assert(err, IsNil)

	err = s.doChangeMaster(&Leader{name: leaderValue})
	c.Assert(err, IsNil)
}

func (t *testAgentServerSuite) TestDoChangeMasterIsFormerNotNew(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	// condition 2: current node is the former leader but not new leader
	atomic.StoreInt32(&s.isLeader, 1)
	err := s.node.RawClient().Put(s.ctx, leaderPath, "another_leader")

	c.Assert(err, IsNil)

	err = s.doChangeMaster(&Leader{name: "another_leader"})
	log.Info(funcName(1), " error is ", err)
	c.Assert(err, IsNil)

}

func (t *testAgentServerSuite) TestDoChangeMasterNotFormerIsNew(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	// condition 3: current node is not the former leader but is new leader
	atomic.StoreInt32(&s.isLeader, 0)
	err := s.node.RawClient().Put(s.ctx, leaderPath, leaderValue)

	c.Assert(err, IsNil)

	err = s.doChangeMaster(&Leader{name: leaderValue})
	c.Assert(err, IsNil)

}

func (t *testAgentServerSuite) TestDoChangeMasterNotFormerNotNew(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	// condition 4: current node is not the former leader and not new leader
	atomic.StoreInt32(&s.isLeader, 0)
	err := s.node.RawClient().Put(s.ctx, leaderPath, "another_leader")

	c.Assert(err, IsNil)

	err = s.doChangeMaster(&Leader{name: "another_leader"})
	c.Assert(err, IsNil)

}

func (t *testAgentServerSuite) TestPreWatchWithTerm0(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.term = 0
	leader := &Leader{}
	err := s.preWatch(leader)
	c.Assert(err, IsNil)
	bs, _, err := s.node.RawClient().Get(s.ctx, s.myTermPath())
	c.Assert(err, IsNil)
	c.Assert(string(bs), Equals, "0")

}

func (t *testAgentServerSuite) TestPreWatchWithTermEquals(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.term = 5
	leader := &Leader{
		meta: LeaderMeta{term: 5},
	}
	err := s.preWatch(leader)
	c.Assert(err, IsNil)
	bs, _, err := s.node.RawClient().Get(s.ctx, s.myTermPath())
	c.Assert(err, IsNil)
	c.Assert(string(bs), Equals, "5")
}

func (t *testAgentServerSuite) TestPreWatchWithTermLessThan2(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.term = 3
	leader := &Leader{
		meta: LeaderMeta{term: 5},
	}
	err := s.preWatch(leader)
	c.Assert(err, NotNil)
	c.Assert(errors.IsNotValid(err), Equals, true)
}

func (t *testAgentServerSuite) TestPreWatchWithTermLarger(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.term = 10
	s.lastUUID = "abc"
	s.lastGTID = "abc:12,1bd:20"
	leader := &Leader{
		meta: LeaderMeta{
			lastUUID: "abc",
			lastGTID: "abc:12,1bd:20",
			term:     5,
		},
	}
	s.preWatch(leader)
	//TODO This case should not happen. So no need to verify
}

func (t *testAgentServerSuite) TestPreWatchWithTermLessThan1(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.term = 4
	s.lastUUID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c"
	s.lastGTID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c:1-12"

	leader := &Leader{
		meta: LeaderMeta{
			lastUUID: "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c",
			lastGTID: "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c:1-12",
			term:     5,
		},
	}
	err := s.preWatch(leader)
	c.Assert(err, IsNil)
	c.Assert(s.term, Equals, uint64(5))
	bs, _, err := s.node.RawClient().Get(s.ctx, s.myTermPath())
	c.Assert(err, IsNil)
	c.Assert(string(bs), Equals, "5")

	// with invalid gtid
	s.term = 4
	s.lastUUID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c"
	s.lastGTID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c:1-15"
	err = s.preWatch(leader)
	c.Assert(err, NotNil)
	c.Assert(errors.IsNotValid(err), Equals, true)

	s.term = 4
	s.lastUUID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8606c"
	s.lastGTID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8606c:1-15"
	err = s.preWatch(leader)
	c.Assert(err, NotNil)
	c.Assert(errors.IsNotValid(err), Equals, true)

	s.term = 4
	s.lastUUID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8605c"
	s.lastGTID = "4dbb4cbc-a4e4-11e7-b054-bc3f8ff8606c:1-15"
	err = s.preWatch(leader)
	c.Assert(err, NotNil)

}

func (t *testAgentServerSuite) TestPreCampaignAsSlave(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.cfg.CampaignWaitTime = 1 * time.Second

	var electionLog LogForElection
	electionLog.Term = s.term + 1
	electionLog.LastUUID = s.lastUUID
	electionLog.LastGTID = s.lastGTID
	bs, err := json.Marshal(electionLog)
	c.Assert(err, IsNil)
	s.node.RawClient().Put(s.ctx, join(electionPath, "nodes", "another_agent"), string(bs))

	startTime := time.Now()
	isSingleMaster := s.preCampaign(false)
	c.Assert(isSingleMaster, Equals, false)
	duration := time.Now().Sub(startTime)
	c.Assert(duration >= 2*s.cfg.CampaignWaitTime, Equals, true)
	c.Assert(duration < 3*s.cfg.CampaignWaitTime, Equals, true)

}

func (t *testAgentServerSuite) TestStartBinlogMonitorLoop(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("binlog monitor loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.startBinlogMonitorLoop()
	}()
	time.Sleep(2 * time.Second)

	s.Close()
}

func (t *testAgentServerSuite) TestStartWriteFDLoop(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	go func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("write fd loop panic. error: %s, stack: %s", err, debug.Stack())
			}
		}()
		s.startWriteFDLoop()
	}()
	time.Sleep(2 * time.Second)

	s.Close()
}

type MockNode struct {
	client *etcd.Client
	id     string
}

func (n *MockNode) ID() string { return "unique_id" }
func (n *MockNode) Register(ctx context.Context) error {
	return n.RawClient().Put(ctx, join("slave", n.ID()), "place_holder")
}
func (n *MockNode) Unregister(ctx context.Context) error       { return nil }
func (n *MockNode) Heartbeat(ctx context.Context) <-chan error { return nil }
func (n *MockNode) RawClient() *etcd.Client                    { return n.client }
func (n *MockNode) NodesStatus(ctx context.Context) ([]*NodeStatus, error) {
	return []*NodeStatus{
		{
			NodeID:       "unique_id",
			InternalHost: "host1",
			ExternalHost: "host1",
		},
		{
			NodeID:       "another_leader",
			InternalHost: "host1",
			ExternalHost: "host1",
		},
	}, nil
}

func (n *MockNode) NodeStatus(ctx context.Context, nodeID string) (*NodeStatus, error) {
	return &NodeStatus{
		NodeID:       "unique_id",
		InternalHost: "host1",
		ExternalHost: "host1",
	}, nil
}

type MockServiceManager struct {
	slaveStatus              *Position
	masterStatus             *Position
	masterUUID, executedGTID string
	endTxnID                 uint64
	serverUUID               string
}

func (m *MockServiceManager) SetReadOnly() error {
	return nil
}
func (m *MockServiceManager) SetReadWrite() error {
	return nil
}
func (m *MockServiceManager) PromoteToMaster() error {
	return nil
}
func (m *MockServiceManager) RedirectMaster(masterHost, masterPort string) error {
	return nil
}

func (m *MockServiceManager) Close() error {
	return nil
}

func (m *MockServiceManager) WaitForRunningProcesses(second int) error {
	return nil
}

func (m *MockServiceManager) SetReadOnlyManually() (bool, error) {
	return true, nil
}

func (m *MockServiceManager) WaitCatchMaster(gtid string) chan interface{} {
	r := make(chan interface{})
	close(r)
	return r
}

func (m *MockServiceManager) LoadSlaveStatusFromDB() (*Position, error) {
	return m.slaveStatus, nil
}
func (m *MockServiceManager) LoadMasterStatusFromDB() (*Position, error) {
	return m.masterStatus, nil
}
func (m *MockServiceManager) LoadReplicationInfoOfMaster() (masterUUID, executedGTID string, etid uint64, err error) {
	return m.masterUUID, m.executedGTID, m.endTxnID, nil
}
func (m *MockServiceManager) LoadReplicationInfoOfSlave() (masterUUID, executedGTID string, etid uint64, err error) {
	return m.masterUUID, m.executedGTID, m.endTxnID, nil
}
func (m *MockServiceManager) GetServerUUID() (string, error) {
	return m.serverUUID, nil
}

func funcName(skip int) string {
	pc, _, _, _ := runtime.Caller(skip)
	n := runtime.FuncForPC(pc).Name()
	ns := strings.Split(n, ".")
	return ns[len(ns)-1]
}

func newMockServer() *Server {
	cfg := *mockCfg
	cfg.DataDir = funcName(2)
	cfg.EtcdRootPath = funcName(2)
	s, _ := NewServer(&cfg)
	s.serviceManager = mockServiceManager
	s.node.Register(s.ctx)
	etcdRegistry := NewEtcdRegistry(s.node.RawClient(), 2*time.Second)
	etcdRegistry.RegisterNode(s.ctx,
		"another_leader",
		"host1",
		"host1", 100)
	log.Infof("new mockserver cfg is %+v", cfg)
	return s
}

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
	"testing"

	"context"

	"sync/atomic"

	"net/http"

	"database/sql"

	"time"

	"runtime/debug"

	"strings"

	"os"

	"runtime"

	"git.mobike.io/database/mysql-agent/pkg/etcd"
	"git.mobike.io/database/mysql-agent/pkg/log"
	// import for size-effect
	_ "github.com/coreos/bbolt"
	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/integration"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"
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

	db, mock, err := sqlmock.New()
	mockDB = db
	if err != nil {
		log.Error("error when mock mysql ", err)
	}
	mock.ExpectQuery("SHOW MASTER STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"File", "Position", "Binlog_Do_DB", "Binlog_Ignore_DB", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "", "", "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46"))
	mockServiceManager = &MockServiceManager{}

	mockCfg = &Config{
		LeaderLeaseTTL:      10,
		ShutdownThreshold:   3,
		RegisterTTL:         5,
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
	err = s.node.RawClient().Put(s.ctx, "test_key", "test_value")

	defer os.RemoveAll(cfg.DataDir)

}

func (t *testAgentServerSuite) TearDownSuite(c *C) {}

func (t *testAgentServerSuite) TestUploadPromotionBinlog(c *C) {

	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectQuery("SHOW SLAVE STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"Relay_Master_Log_File", "Exec_Master_Log_Pos", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46"))
	mock.ExpectQuery("SHOW SLAVE STATUS").
		WillReturnRows(sqlmock.NewRows([]string{}).AddRow())
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.db = db

	err = s.uploadPromotionBinlog()
	log.Warn("error is ", err)
	c.Assert(err, IsNil)

	err = s.uploadPromotionBinlog()
	c.Assert(err, IsNil)
}

func (t *testAgentServerSuite) TestChangeMaster(c *C) {
	w := &MockResponseWriter{}
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.isLeader = 1
	s.leaderStopCh = make(chan interface{})
	s.ChangeMaster(w, nil)
	c.Assert(string(w.content), Equals, "current node is leader and has given up leader successfully\n")

	s.isLeader = 0
	s.ChangeMaster(w, nil)
	c.Assert(string(w.content), Equals, "current node is not leader, change master should be applied on leader\n")
}

func (t *testAgentServerSuite) TestSetReadOnly(c *C) {
	w := &MockResponseWriter{}

	s := newMockServer()
	s.isLeader = 1
	s.leaderStopCh = make(chan interface{})
	s.db = mockDB
	defer os.RemoveAll(s.cfg.DataDir)
	s.SetReadOnly(w, nil)
	c.Assert(string(w.content), Equals, "set current node readonly success\n")

	s.isLeader = 0

	s.SetReadOnly(w, nil)
	c.Assert(string(w.content), Equals, "current node is not leader, so no need to set readonly\n")
}

func (t *testAgentServerSuite) TestSetReadWrite(c *C) {
	w := &MockResponseWriter{}
	s := newMockServer()
	s.isLeader = 1
	s.leaderStopCh = make(chan interface{})
	s.db = mockDB
	defer os.RemoveAll(s.cfg.DataDir)
	s.SetReadWrite(w, nil)
	c.Assert(string(w.content), Equals, "set current node readwrite success\n")

	s.isLeader = 0

	s.SetReadWrite(w, nil)
	c.Assert(string(w.content), Equals, "current node is not leader, so no need to set readwrite\n")
}

func (t *testAgentServerSuite) TestUpdateBinlogPos(c *C) {
	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectQuery("SHOW MASTER STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"File", "Position", "Binlog_Do_DB", "Binlog_Ignore_DB", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "", "", "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46"))
	mock.ExpectQuery("SELECT @@server_uuid").
		WillReturnRows(sqlmock.
			NewRows([]string{"@@server_uuid"}).
			AddRow("53ea0ed1-9bf8-11e6-8bea-64006a897c73"))
	mock.ExpectQuery("SHOW SLAVE STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"Relay_Master_Log_File", "Exec_Master_Log_Pos", "Executed_Gtid_Set",
				"Master_UUID", "IO_Thread", "SQL_Thread"}).
			AddRow("mysql-bin.000005", 188858056, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
				"85ab69d1-b21f-11e6-9c5e-64006a8978d2", "Yes", "Yes"))
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.db = db

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

	s.db = mockDB
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
	leader, _, _ := s.getLeader()
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

	err = s.doChangeMaster()
	c.Assert(err, IsNil)
}

func (t *testAgentServerSuite) TestDoChangeMasterIsFormerNotNew(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	// condition 2: current node is the former leader but not new leader
	atomic.StoreInt32(&s.isLeader, 1)
	err := s.node.RawClient().Put(s.ctx, leaderPath, "another_leader")

	c.Assert(err, IsNil)

	err = s.doChangeMaster()
	log.Info(funcName(), " error is ", err)
	c.Assert(err, IsNil)

}

func (t *testAgentServerSuite) TestDoChangeMasterNotFormerIsNew(c *C) {

	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)

	mock.ExpectQuery("SHOW SLAVE STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"Relay_Master_Log_File", "Exec_Master_Log_Pos", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46"))

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.db = db
	// condition 3: current node is not the former leader but is new leader
	atomic.StoreInt32(&s.isLeader, 0)
	err = s.node.RawClient().Put(s.ctx, leaderPath, leaderValue)

	c.Assert(err, IsNil)

	err = s.doChangeMaster()
	c.Assert(err, IsNil)

}

func (t *testAgentServerSuite) TestDoChangeMasterNotFormerNotNew(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	// condition 4: current node is not the former leader and not new leader
	atomic.StoreInt32(&s.isLeader, 0)
	err := s.node.RawClient().Put(s.ctx, leaderPath, "another_leader")

	c.Assert(err, IsNil)

	err = s.doChangeMaster()
	c.Assert(err, IsNil)

}

func (t *testAgentServerSuite) TestStartBinlogMonitorLoop(c *C) {

	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.db = mockDB

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
	s.db = mockDB

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

func (n *MockNode) ID() string                                 { return "unique_id" }
func (n *MockNode) Register(ctx context.Context) error         { return nil }
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

type MockServiceManager struct{}

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

type MockResponseWriter struct {
	content []byte
}

func (m *MockResponseWriter) Header() http.Header {
	return nil
}
func (m *MockResponseWriter) Write(b []byte) (int, error) {
	m.content = b
	return len(b), nil
}
func (m *MockResponseWriter) WriteHeader(int) {}

func funcName() string {
	pc, _, _, _ := runtime.Caller(1)
	n := runtime.FuncForPC(pc).Name()
	ns := strings.Split(n, ".")
	return ns[len(ns)-1]
}

func newMockServer() *Server {
	cfg := *mockCfg
	cfg.DataDir = funcName()
	cfg.EtcdRootPath = funcName()
	s, _ := NewServer(&cfg)
	s.serviceManager = mockServiceManager
	s.node.Register(s.ctx)
	etcdRegistry := NewEtcdRegistry(s.node.RawClient(), 2*time.Second)
	etcdRegistry.RegisterNode(s.ctx,
		"another_leader",
		"host1",
		"host1", 100)
	return s
}

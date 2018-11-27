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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"

	"git.mobike.io/database/mysql-agent/pkg/log"
	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"git.mobike.io/database/mysql-agent/pkg/systemcall"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// latestPos records the latest MySQL binlog of the agent works on.
	latestPos Position

	fetchBinlogInterval = 1 * time.Second

	// leaderValue stores the value that will be put to leaderpath
	// if CURRENT LeaderFollowerRegistry is the leader
	leaderValue string
)

// Server maintains agent's status at runtime.
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc

	// node maintains the status of MySQL and interact with etcd registry.
	node Node

	serviceManager ServiceManager

	isLeader   int32
	onlyFollow bool

	// leaseExpireTS is the timestamp that lease might expire if lease is failed to renew.
	// leaseExpireTS := time.Now() + leaseTTL - shutdownThreshold
	leaseExpireTS int64

	fdStopCh chan interface{}

	isClosed     int32
	leaderStopCh chan interface{}
	httpSrv      *http.Server

	shutdownCh chan interface{}

	db *sql.DB

	cfg *Config
}

func checkConfig(cfg *Config) error {
	if cfg == nil {
		return errors.NotValidf("cfg is nil")
	}

	if cfg.DataDir == "" {
		return errors.NotValidf("DataDir is empty")
	}

	if cfg.EtcdRootPath == "" {
		return errors.NotValidf("EtcdRootPath is empty")
	}

	if cfg.LeaderLeaseTTL == 0 {
		return errors.NotValidf("LeaderLeaseTTL is 0")
	}

	if cfg.ShutdownThreshold == 0 {
		return errors.NotValidf("ShutdownThreshold is 0")
	}

	if cfg.RegisterTTL == 0 {
		return errors.NotValidf("RegisterTTL is 0")
	}

	if cfg.EtcdUsername == "" {
		return errors.NotValidf("EtcdUsername is empty")
	}
	return nil
}

// NewServer returns an instance of agent server.
func NewServer(cfg *Config) (*Server, error) {

	err := checkConfig(cfg)
	if err != nil {
		log.Error("cfg is illegal, err: ", err)
		return nil, err
	}

	n, err := NewAgentNode(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		ctx:        ctx,
		cancel:     cancel,
		node:       n,
		cfg:        cfg,
		shutdownCh: make(chan interface{}),
	}, nil
}

// Start runs Server, and maintains heartbeat to etcd.
func (s *Server) Start() error {
	// mark started
	atomic.StoreInt32(&s.isClosed, 0)
	initMetrics()
	leaderValue = s.node.ID()
	go s.startWriteFDLoop()

	// init the HTTP server.
	httpSvr, err := s.initHTTPServer()
	if err != nil {
		return err
	}
	s.httpSrv = httpSvr
	// run HTTP Server
	go s.httpSrv.ListenAndServe()

	// try to start(double fork and exec) a new mysql if current mysql is down
	if !isPortAlive(s.cfg.DBConfig.Host, fmt.Sprint(s.cfg.DBConfig.Port)) {
		log.Info("config file is ", s.cfg.configFile)
		log.Info("mysql is not alive, try to start")
		log.Info("double fork and exec ", s.cfg.ForkProcessFile, s.cfg.ForkProcessArgs)
		err := systemcall.DoubleForkAndExecute(s.cfg.configFile)
		if err != nil {
			log.Error("error while double fork ", s.cfg.ForkProcessFile, " error is ", err)
			return errors.Trace(err)
		}
		stopCh := make(chan interface{})
		select {
		case <-waitUtilPortAlive(s.cfg.DBConfig.Host, fmt.Sprint(s.cfg.DBConfig.Port), stopCh):
			log.Info("mysql has been started")
			break
		case <-time.After(time.Duration(s.cfg.ForkProcessWaitSecond) * time.Second):
			stopCh <- "timeout"
			return errors.New("cannot start mysql, timeout")
		}
	} else {
		log.Info("mysql is alive, according to port alive detection")
	}

	// try to connect MySQL
	var db *sql.DB
	startTime := time.Now()
	for true {
		if time.Now().Sub(startTime) > time.Duration(s.cfg.ForkProcessWaitSecond)*time.Second {
			log.Errorf("timeout to connect to MySQL by user %s", s.cfg.DBConfig.User)
			return errors.Errorf("timeout to connect to MySQL by user %s", s.cfg.DBConfig.User)
		}
		db, err = mysql.CreateDB(s.cfg.DBConfig)
		if err != nil {
			log.Errorf("fail to connect MySQL in agent start: %+v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		err = mysql.Select1(db)
		if err != nil {
			log.Errorf("fail to select 1 from MySQL in agent start: %+v", err)
			db.Close()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	log.Info("db is successfully connected and select 1 is OK")
	s.db = db

	// init ServiceManager
	sm := &mysqlServiceManager{
		db:                  s.db,
		mysqlNet:            s.cfg.DBConfig.ReplicationNet,
		replicationUser:     s.cfg.DBConfig.ReplicationUser,
		replicationPassword: s.cfg.DBConfig.ReplicationPassword,
	}
	s.serviceManager = sm

	// before register this node, set service to readonly first
	for true {
		err = s.serviceManager.SetReadOnly()
		if err == nil {
			break
		}
		log.Errorf("has error in SetReadOnly. self spin. err: %+v ", err)
		time.Sleep(100 * time.Millisecond)
	}
	// register this node.
	if err := s.node.Register(s.ctx); err != nil {
		return errors.Annotate(err, "fail to register node to etcd")
	}

	// start heartbeat loop.
	errc := s.node.Heartbeat(s.ctx)
	go func() {
		for err := range errc {
			log.Error("heartbeat has error ", err)
		}
		log.Info("heartbeat is closed")
	}()

	// start monitor binlog goroutine.
	go s.startBinlogMonitorLoop()
	go s.leaderLoop()

	// now init nearly done, begin to write to fd
	s.writeFD()

	<-s.shutdownCh
	log.Info("receive from s.shutdownCh, exit Start() function")
	return nil

}

// Close gracefully releases resource of agent server.
func (s *Server) Close() {
	// mark closed
	if !atomic.CompareAndSwapInt32(&s.isClosed, 0, 1) {
		log.Info("isClose has already been set to 1, another goroutine is closing the server.")
		return
	}

	// lock service to avoid brain-split
	s.setServiceReadonlyOrShutdown()
	s.serviceManager.Close()

	// un-register this node.
	if err := s.node.Unregister(s.ctx); err != nil {
		log.Error(errors.ErrorStack(err))
	}

	// notify other goroutines to exit
	if s.cancel != nil {
		s.cancel()
	}

	// delete leader key
	s.deleteLeaderKey()

	if s.httpSrv != nil {
		s.httpSrv.Shutdown(nil)
	}

	close(s.shutdownCh)
}

func (s *Server) forceClose() {
	os.Exit(3)
}

func (s *Server) initHTTPServer() (*http.Server, error) {
	listenURL, err := url.Parse(s.cfg.ListenAddr)
	if err != nil {
		return nil, errors.Annotate(err, "fail to parse listen address")
	}

	// listen & serve.
	httpSrv := &http.Server{Addr: fmt.Sprintf(":%s", listenURL.Port())}
	// TODO add the new status function to detect all agents' status
	// http.HandleFunc("/status", s.Status)
	http.HandleFunc("/changeMaster", s.ChangeMaster)
	http.HandleFunc("/setReadOnly", s.SetReadOnly)
	http.HandleFunc("/setReadWrite", s.SetReadWrite)
	http.HandleFunc("/setOnlyFollow", s.SetOnlyFollow)
	return httpSrv, nil
}

func (s *Server) writeFD() {
	systemcall.WriteToEventfd(s.cfg.fd, 1)
}

func (s *Server) startWriteFDLoop() {
	// stop write fd goroutine, if there is.
	s.resetWriteFDLoop()
	log.Info("start write fd loop")
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-s.fdStopCh:
			log.Info("receive sendFDCh, stop sending eventfd")
			ticker.Stop()
			return
		case <-ticker.C:
			s.writeFD()
			agentHeartbeat.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName}).Inc()
		}
	}
}

func (s *Server) resetWriteFDLoop() {
	log.Info("reset write fd loop")
	if s.fdStopCh != nil {
		close(s.fdStopCh)
	}
	s.fdStopCh = make(chan interface{})
}

func (s *Server) startBinlogMonitorLoop() {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("monitor loop panic. error: %s, stack: %s", err, debug.Stack())
		}

		s.Close()
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(fetchBinlogInterval):
			err := DoWithRetry(s.updateBinlogPos, "updateBinlogPos", 0, fetchBinlogInterval/2)
			if err != nil {
				log.Warn("has error when updateBinlogPos. err ", err, " ignore.")
			}
		}
	}
}

func (s *Server) updateBinlogPos() error {
	pos, gtidSet, err := mysql.GetMasterStatus(s.db)
	if err != nil {
		// error in getting MySQL binlog position
		// TODO change alive status or retry
		return err
	}
	if latestPos.UUID == "" {
		uuid, err := mysql.GetServerUUID(s.db)
		if err != nil {
			// error in getting MySQL binlog position
			// TODO change alive status or retry
			return err
		}
		latestPos.UUID = uuid
	}
	latestPos.File = pos.Name
	latestPos.Pos = fmt.Sprint(pos.Pos)
	latestPos.GTID = gtidSet.String()

	// below are monitor logic
	var gtidStr, masterUUID string
	if s.amILeader() {
		agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "sql_thread"}).Set(0)
		agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "io_thread"}).Set(0)
		gtidStr, masterUUID = latestPos.GTID, latestPos.UUID

	} else {
		rSet, err := mysql.GetSlaveStatus(s.db)
		if err != nil {
			return errors.Trace(err)
		}
		if rSet["Slave_SQL_Running"] == "Yes" {
			agentSlaveStatus.With(prometheus.Labels{
				"cluster_name": s.cfg.ClusterName,
				"type":         "sql_thread"}).Set(1)
		} else {
			agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "sql_thread"}).Set(-1)
		}
		if rSet["Slave_IO_Running"] == "Yes" {
			agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "io_thread"}).Set(1)
		} else {
			agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "io_thread"}).Set(-1)
		}
		gtidStr, masterUUID = rSet["Executed_Gtid_Set"], rSet["Master_UUID"]
	}

	endTxnID, err := mysql.GetTxnIDFromGTIDStr(gtidStr, masterUUID)
	if err != nil {
		log.Warnf("error when GetTxnIDFromGTIDStr gtidStr: %s, gtid: %s, error is %v", gtidStr, masterUUID, err)
		return nil
	}
	// assume the gtidset is continuous, only pick the last one
	agentSlaveStatus.
		With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "executed_gtid"}).
		Set(float64(endTxnID))

	return nil
}

// leaderLoop includes the leader campaign, master keep alive and slave watch
func (s *Server) leaderLoop() {
	// use defer to catch panic
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("leader loop panic. error: %s, stack: %s", err, debug.Stack())
		}
		s.Close()
	}()

	for {
		//TODO add loop invariant assertion: readonly = 1

		leader, kv, err := s.getLeader()

		if err != nil {
			isClosed := atomic.LoadInt32(&s.isClosed)
			log.Printf("isClosed: %d", isClosed)
			if isClosed == 1 {
				return
			}
			log.Errorf("get leader err %v", err)
			// TODO wait or exit?
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// if the leader already exists, no need to campaign
		if leader != "" {
			if s.isSameLeader(leader) {
				// if the leader is the current agent, then we need to keep the lease alive again
				// txn().if(leader is current Agent).then(putWithLease).else()
				// if txn success, set mysql readwrite
				log.Info("current node is still the leader, try to resume leader keepalive")
				startTime := time.Now()
				campaignSuccess, lr, err := s.resumeLeader()
				if err != nil {
					log.Errorf("resume leader err %v", err)
					// TODO wait or exit?
					time.Sleep(200 * time.Millisecond)
					continue
				}
				if !campaignSuccess {
					continue
				}
				err = s.serviceManager.SetReadWrite()
				if err != nil {
					log.Error("fail to set read write, err: ", err)
					s.serviceManager.SetReadOnly()
					continue
				}
				if time.Now().Sub(startTime) > time.Duration(s.cfg.LeaderLeaseTTL)*time.Second {
					log.Error("timeout between resume leader success and renew Leader lease, lease may expire")
					s.serviceManager.SetReadOnly()
					continue
				}
				s.setIsLeaderToTrue()
				// assert: s.leaderStopCh is nil or closed
				s.leaderStopCh = make(chan interface{})
				s.keepLeaderAliveLoop(int64(lr.ID))
				continue
			} else {
				// now the leader exists and is not current node, therefore just watch
				log.Infof("leader is %s, watch it", leader)
				err = s.doChangeMaster()
				if err != nil {
					log.Error("error while slave redirects master, goto leader loop start point ", err)
					continue
				}
				err = s.watchLeader(kv.ModRevision)
				if err != nil {
					log.Info("try to watch leader with the latest revision")
					err = s.watchLeader(0)
				}
				log.Info("leader changed, try to campaign leader")
				continue
			}
		}
		// now the `leader` is zero value, indicating that there is no leader,
		// therefore a leader campaign is followed
		// assume binlog has already catch up, no need to wait. so comment waitUntilBinlogCatchup
		// <-s.waitUntilBinlogCatchup()
		if s.isOnlyFollow() {
			log.Info("current node is an only follow node, do not participaate in leader campaign")
			time.Sleep(1 * time.Second)
			continue
		}
		startTime := time.Now()
		campaignSuccess, lr, err := s.campaignLeader()
		if err != nil {
			// if leader campaign has error occurred,
			// one situation is that the leader is another node, with no error,
			// in which case, current node just get leader again and start a new watch
			//
			// one other situation may be that current node succeed in campaign
			// but fail in some proceeding process, timeout in return maybe.
			// In this case, current solution is try to delete current leader if the leader is current node.
			log.Errorf("campaign leader err, someone else may be the leader %s", errors.ErrorStack(err))
			s.deleteLeaderKey()
			continue
		}
		if !campaignSuccess {
			log.Info("campaign leader fail, someone else may be the leader")
			continue
		}
		// now current node is the leader
		err = s.promoteToMaster()
		if err != nil {
			log.Error("fail to set read write, err: ", err)
			s.setServiceReadonlyOrShutdown()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if time.Now().Sub(startTime) > time.Duration(s.cfg.LeaderLeaseTTL)*time.Second {
			log.Error("timeout between campaign leader success and renew Leader lease, lease may expire")
			s.serviceManager.SetReadOnly()
			continue
		}
		// assert: s.leaderStopCh is nil or closed
		s.leaderStopCh = make(chan interface{})
		agentMasterSwitch.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName}).Inc()
		s.keepLeaderAliveLoop(int64(lr.ID))
	}

}

func (s *Server) setServiceReadonlyOrShutdown() {
	err := s.serviceManager.SetReadOnly()
	if err != nil {
		log.Warn("has error when set service readonly, so force close, err ", err)
		s.forceClose()
	}
}

// ChangeMaster triggers change master by endpoint
func (s *Server) ChangeMaster(w http.ResponseWriter, r *http.Request) {
	if r != nil {
		log.Info("receive ChangeMaster from ", r.RemoteAddr)
	}
	if s.amILeader() {
		// set service readonly to avoid brain-split
		s.setServiceReadonlyOrShutdown()
		// delete leader key
		s.deleteLeaderKey()
		// wait so that other agents are more likely to be the leader
		time.Sleep(500 * time.Millisecond)
		// inform leaderStopCh to stop keeping alive loop
		close(s.leaderStopCh)
		log.Info("current node is leader and has given up leader successfully")
		w.Write([]byte("current node is leader and has given up leader successfully\n"))
	} else {
		log.Info("current node is not leader, change master should be applied on leader")
		w.Write([]byte("current node is not leader, change master should be applied on leader\n"))
	}
}

// SetReadOnly sets mysql readonly
func (s *Server) SetReadOnly(w http.ResponseWriter, r *http.Request) {
	log.Info("receive setReadOnly request")
	if s.amILeader() {
		success, err := s.serviceManager.SetReadOnlyManually()
		if err != nil {
			log.Info("set current node readonly fail")
			log.Error("error when set current node readonly ", err)
			w.Write([]byte("set current node readonly fail\n"))
			w.Write([]byte("error when set current node readonly" + err.Error() + "\n"))
			return
		}
		if success {
			log.Info("set current node readonly success")
			w.Write([]byte(fmt.Sprint("set current node readonly success\n")))
		} else {
			log.Info("set current node readonly fail")
			w.Write([]byte(fmt.Sprint("set current node readonly fail\n")))
		}
		return
	}
	log.Info("set current node readonly fail")
	log.Info("current node is not leader, so no need to set readonly")
	w.Write([]byte("set current node readonly fail\n"))
	w.Write([]byte("current node is not leader, so no need to set readonly\n"))
}

// SetReadWrite sets mysql readwrite
func (s *Server) SetReadWrite(w http.ResponseWriter, r *http.Request) {
	log.Info("receive setReadWrite request")
	if s.amILeader() {
		err := s.serviceManager.SetReadWrite()
		if err != nil {
			log.Info("set current node readwrite fail")
			log.Error("error when set current node readwrite ", err)
			w.Write([]byte("set current node readwrite fail\n"))
			w.Write([]byte("error when set current node readwrite" + err.Error() + "\n"))
			return
		}
		log.Info("set current node readwrite success")
		w.Write([]byte("set current node readwrite success\n"))
		return
	}
	log.Info("set current node readwrite fail")
	log.Info("current node is not leader, so no need to set readwrite")
	w.Write([]byte("set current node readwrite fail\n"))
	w.Write([]byte("current node is not leader, so no need to set readwrite\n"))
}

// SetOnlyFollow sets the s.onlyFollow to true or false,
// depending on the param `onlyFollow` passed in
func (s *Server) SetOnlyFollow(w http.ResponseWriter, r *http.Request) {
	log.Info("receive setOnlyFollow request")
	operation, ok := r.URL.Query()["onlyFollow"]
	if !ok || len(operation) < 1 {
		log.Errorf("has no onlyFollow in query %+v ", r.URL.Query())
		w.Write([]byte(fmt.Sprintf("has no onlyFollow in query %+v ", r.URL.Query())))
		return
	}
	switch operation[0] {
	case "true":
		log.Info("set onlyFollow to true")
		s.onlyFollow = true
		w.Write([]byte("set onlyFollow to true"))
		break
	case "false":
		log.Info("set onlyFollow to false")
		s.onlyFollow = false
		w.Write([]byte("set onlyFollow to false"))
		break
	default:
		log.Info("set onlyFollow operation is undefined ", operation[0])
		w.Write([]byte("set onlyFollow operation is undefined " + operation[0]))
	}
}

func (s *Server) setIsLeaderToTrue() {
	atomic.StoreInt32(&s.isLeader, 1)
	agentIsLeader.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName}).Set(1)
}

func (s *Server) setIsLeaderToFalse() bool {
	agentIsLeader.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName}).Set(0)
	return atomic.CompareAndSwapInt32(&s.isLeader, 1, 0)
}

func (s *Server) amILeader() bool {
	return atomic.LoadInt32(&s.isLeader) == 1
}

// isOnlyFollow returns whether current server is the node
// that only follows master, not participating in campaigning leader.
// if variable in memory, onlyFollow, is true, then server is onlyFollow,
// else it depends on s.cfg.OnlyFollow
func (s *Server) isOnlyFollow() bool {
	if s.onlyFollow {
		return true
	}
	return s.cfg.OnlyFollow
}

// doChangeMaster does different things in the following different conditions:
// if a node is the former leader and current leader, do nothing
// if a node is the former leader but not current leader, downgrade
// if a node is not the former leader but current leader, promote to master
// if a node is not the former leader nor current leader, redirect master to new leader.
func (s *Server) doChangeMaster() error {
	// get new master info from etcd
	newLeaderID, _, err := s.getLeader()
	if err != nil {
		return errors.Trace(err)
	}
	log.Infof("the new leader id is %s", newLeaderID)
	log.Info("the new leader is current node? ", s.isSameLeader(newLeaderID))
	log.Info("current node is former leader? ", s.amILeader())

	if s.isSameLeader(newLeaderID) {
		// current node is the new leader
		return s.promoteToMaster()
	}
	// now current node is not the new leader
	newLeader, err := s.node.NodeStatus(s.ctx, newLeaderID)
	if err != nil {
		return errors.Trace(err)
	}
	log.Infof("new leader info is %+v", newLeader)

	masterHost, masterPort, err := parseHost(newLeader.InternalHost)
	if err != nil {
		return err
	}
	log.Infof("masterHost: %s, masterPort: %s", masterHost, masterPort)

	if s.setIsLeaderToFalse() {
		// current node is former leader and is not the new leader
		// downgrade to follower
		log.Info("current node is former master, downgrade to slave")
	}
	// current node is not leader any more, became slave and direct to new master
	return s.becomeSlave(masterHost, masterPort)
}

func (s *Server) promoteToMaster() error {
	// current node is the new leader
	if s.amILeader() {
		// current node is former leader
		log.Info("former leader is also new leader")
		return s.serviceManager.SetReadWrite()
	}
	// if current node is not the former leader, promote

	log.Info("current node is the new leader but not the former, begin to promote")
	s.setIsLeaderToTrue()
	// stop io/sql thread
	agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "sql_thread"}).Set(0)
	agentSlaveStatus.With(prometheus.Labels{"cluster_name": s.cfg.ClusterName, "type": "io_thread"}).Set(0)
	s.serviceManager.PromoteToMaster()
	// upload current mysql binlog info
	err := s.uploadPromotionBinlog()
	if err != nil {
		log.Error("has error when upload position during master promotion, "+
			"info may be inconsistent ", err)
	}
	return s.serviceManager.SetReadWrite()
}

// TODO move to service manager to decouple mysql and agent
func (s *Server) uploadPromotionBinlog() error {
	rSet, err := mysql.GetSlaveStatus(s.db)
	if err != nil {
		return errors.Trace(err)
	}
	if rSet["Relay_Master_Log_File"] == "" {
		log.Warn("do not have Relay_Master_Log_File, this node is not a former slave, "+
			"it may be the initialization of this cluster, slave status is ", rSet)
		return nil
	}
	var currentPos Position
	currentPos.File = rSet["Relay_Master_Log_File"]
	currentPos.Pos = rSet["Exec_Master_Log_Pos"]
	currentPos.GTID = rSet["Executed_Gtid_Set"]
	currentPos.UUID = latestPos.UUID
	key := join("switch", fmt.Sprint(time.Now().Unix()), s.node.ID())
	latestPosJSONBytes, err := json.Marshal(currentPos)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.node.RawClient().Put(s.ctx, key, string(latestPosJSONBytes))
	return errors.Trace(err)
}

func (s *Server) becomeSlave(masterHost, masterPort string) error {
	// After new master has been promoted
	err := s.serviceManager.RedirectMaster(masterHost, masterPort)
	retry := 0
	for err != nil && retry < 10 {
		log.Error("redirect master has error, self-spin ", err)
		time.Sleep(100 * time.Millisecond)
		err = s.serviceManager.RedirectMaster(masterHost, masterPort)
		retry++
	}

	return err
}

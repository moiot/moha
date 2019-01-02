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

package checker

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"git.mobike.io/database/mysql-agent/pkg/etcd"
	"git.mobike.io/database/mysql-agent/pkg/log"
	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"git.mobike.io/database/mysql-agent/pkg/types"
	"github.com/juju/errors"
)

var (
	leaderPath = "master"

	// azSet is the set of all AZs, for example {az1, az2, az3}
	azSet map[string]bool

	// ipAZMapping is the ip -> AZ mapping
	ipAZMapping map[string]string

	// azNodeMapping is the az -> nodes mapping
	azNodeMapping map[string]map[string]bool

	// azIPMapping is the az -> IPs mapping
	azIPMapping map[string]map[string]bool
)

// Server is the checker server
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg *Config

	// etcdClient interacts with etcd
	etcdClient *etcd.Client

	// mysqlID is the ID of db connection
	mysqlID string
	// mysqlConnection is the db connection
	mysqlConnection *sql.DB
	dbLock          sync.Mutex

	chaosJobDone int32

	dmlJobs map[string]DMLJob
}

// NewServer creates a new server
func NewServer(cfg *Config) (*Server, error) {

	// init etcd client
	uv, err := etcd.NewURLsValue(cfg.EtcdURLs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cli, err := etcd.NewClientFromCfg(uv.StringSlice(), cfg.EtcdDialTimeout,
		cfg.EtcdRootPath, cfg.ClusterName, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// init context related
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		ctx:        ctx,
		cancel:     cancel,
		etcdClient: cli,
		cfg:        cfg,
	}, nil

}

// Start starts the checker
func (s *Server) Start() error {
	log.Info("check server starts")

	log.Info("the ID Container mapping is ", s.cfg.IDContainerMapping)
	log.Info("the Container AZ mapping is ", s.cfg.ContainerAZMapping)

	// init AZ related data structures
	azSet = make(map[string]bool)
	ipAZMapping = make(map[string]string)
	azNodeMapping = make(map[string]map[string]bool)
	azIPMapping = make(map[string]map[string]bool)

	for container, az := range s.cfg.ContainerAZMapping {
		azSet[az] = true
		ip, err := IPOf(container)
		if err != nil {
			log.Errorf("has error when fetching ip of %s, %+v", container, err)
			os.Exit(1)
		}
		ipAZMapping[ip] = az

		nodes, ok := azNodeMapping[az]
		if !ok {
			nodes = make(map[string]bool)
			azNodeMapping[az] = nodes
		}
		nodes[container] = true

		ips, ok := azIPMapping[az]
		if !ok {
			ips = make(map[string]bool)
			azIPMapping[az] = ips
		}
		ips[ip] = true
	}
	log.Info("the azSet is ", azSet)
	log.Info("the ipAZMapping is ", ipAZMapping)
	log.Info("the azNodeMapping is ", azNodeMapping)
	log.Info("the azIPMapping is ", azIPMapping)

	if !strings.HasSuffix(s.cfg.PartitionTemplate, " ") {
		s.cfg.PartitionTemplate += " "
	}
	log.Info("the PartitionTemplate is ", s.cfg.PartitionTemplate)

	// init jobs
	s.dmlJobs = make(map[string]DMLJob)
	s.dmlJobs["simple"] = SimpleDMLJob{}
	s.dmlJobs["long_txn"] = LongTxnJob{}

	masterCh := s.pollMaster()

	go func() {
		for {
			select {
			case newMasterID := <-masterCh:
				log.Info("new master ID is ", newMasterID)
				s.setDBConnection(newMasterID)
			}
		}
	}()

	err := s.initDB()
	// initDB ensures that master MySQL has been elected
	// so all below functions/goroutines are able to get master connections

	if err != nil {
		return err
	}
	var wg sync.WaitGroup

	for jn, j := range s.dmlJobs {
		log.Info("begin to run job ", jn)
		wg.Add(1)
		go func(jobName string, job DMLJob) {
			ticker := time.NewTicker(job.GetInterval())
			c := 0
			defer ticker.Stop()
			for range ticker.C {
				_, conn := s.getDBConnection()
				if err := job.RunDML(conn, c); err != nil {
					if !strings.Contains(err.Error(), "Error 1290:") {
						log.Error("error when running ", jobName, err)
					}
					continue
				}
				c++
				if c == job.GetMaxCounter() {
					for atomic.LoadInt32(&s.chaosJobDone) == 0 {
						time.Sleep(500 * time.Millisecond)
					}
					time.Sleep(job.GetCheckWaitTime())
					err := s.check(jobName, job)
					if err != nil {
						os.Exit(2)
					}
					wg.Done()
					return
				}
			}
		}(jn, j)
	}

	go func() {
		err := s.runChaosJob()
		if err != nil {
			log.Errorf("has error when running chaos job %s. error is %v",
				s.cfg.ChaosJob, err)
			os.Exit(1)
		}
	}()

	wg.Wait()
	log.Info("all checks are done so close")
	return nil

}

func (s *Server) pollMaster() chan string {
	connectionCh := make(chan string)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		masterID := ""
		// get leader changer info
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				var newMasterID string
				ctx, cancel := context.WithTimeout(s.ctx, s.cfg.EtcdDialTimeout)
				masterBytes, _, err := s.etcdClient.Get(ctx, "master")
				if err != nil {
					if errors.IsNotFound(err) {
						log.Error(err)
					}
					newMasterID = ""
				} else {
					newMasterID = string(masterBytes)
				}
				cancel()
				if newMasterID == masterID {
					continue
				}
				masterID = newMasterID
				if masterID == "" {
					continue
				}
				//if newMasterID == "127.0.0.1:3309" {
				//	log.Error("mysql-node-3 is a only follow node, cannot be a leader")
				//	os.Exit(2)
				//}
				log.Info("new master id is ", masterID)
				connectionCh <- masterID
			}
		}
	}()
	return connectionCh
}

func (s *Server) buildDBConnFromID(leaderID string) (*sql.DB, error) {
	hs := strings.Split(leaderID, ":")
	var p int
	if len(hs) < 2 {
		p = 3306
	} else {
		p, _ = strconv.Atoi(hs[1])
	}
	dbConfig := types.DBConfig{
		Host:     hs[0],
		User:     s.cfg.DBConfig.User,
		Password: s.cfg.DBConfig.Password,
		Port:     p,
		Timeout:  s.cfg.DBConfig.Timeout,
	}
	log.Info("new dbConfig ", dbConfig)
	return mysql.CreateDB(dbConfig)
}

func (s *Server) buildDBRootConnFromID(leaderID string) (*sql.DB, error) {
	hs := strings.Split(leaderID, ":")
	var p int
	if len(hs) < 2 {
		p = 3306
	} else {
		p, _ = strconv.Atoi(hs[1])
	}
	dbConfig := types.DBConfig{
		Host:     hs[0],
		User:     s.cfg.RootUser,
		Password: s.cfg.RootPassword,
		Port:     p,
		Timeout:  s.cfg.DBConfig.Timeout,
	}
	log.Info("new dbConfig ", dbConfig)
	return mysql.CreateDB(dbConfig)
}

func (s *Server) getDBConnection() (string, *sql.DB) {
	s.dbLock.Lock()
	defer s.dbLock.Unlock()
	return s.mysqlID, s.mysqlConnection
}

func (s *Server) setDBConnection(newMasterID string) {
	s.dbLock.Lock()
	defer s.dbLock.Unlock()
	dbConnection, err := s.buildDBConnFromID(newMasterID)
	if err != nil {
		log.Error("buildDBConnFromID has error ", err)
		os.Exit(1)
	}
	log.Info("buildDBConnFromID creates new connection ", dbConnection)
	if s.mysqlConnection != nil {
		s.mysqlConnection.Close()
	}
	s.mysqlID = newMasterID
	s.mysqlConnection = dbConnection
}

// initDB is used to drop schema and create schema again.
// initDB should use `root` as user while connecting to DB
func (s *Server) initDB() error {
	log.Info("begin to init DB")
	var masterID string
	startTime := time.Now()
	log.Info("wait until agent starts")
	for masterID == "" {
		masterID, _ = s.getDBConnection()
		if masterID == "" {
			log.Info("still no master")
			if time.Now().Sub(startTime) > 10*time.Minute {
				log.Error("10 minutes with no master, exit(2)")
				os.Exit(2)
			}
			time.Sleep(1000 * time.Millisecond)
		}
	}
	conn, err := s.buildDBRootConnFromID(masterID)
	if err != nil {
		log.Error("has error when buildDBRootConnFromID ", masterID, err)
		os.Exit(1)
	}

	log.Info("wait until mysql works fine")
	startTime = time.Now()
	_, err = conn.Query("SELECT 1")
	for err != nil {
		if time.Now().Sub(startTime) > 10*time.Minute {
			log.Error("10 minutes have passed before mysql works fine, exit(2)")
			os.Exit(2)
		}
		log.Warn("select 1 has error, sleep. ", err)
		time.Sleep(500 * time.Millisecond)
		_, conn = s.getDBConnection()
		_, err = conn.Query("SELECT 1")
	}

	log.Info("init database")
	// the first time to run dml, init the table first
	_, err = conn.Exec("DROP SCHEMA IF EXISTS checker;")
	if err != nil {
		return errors.Trace(err)
	}
	_, err = conn.Exec("CREATE SCHEMA checker;")
	if err != nil {
		log.Error(err)
		return errors.Trace(err)
	}

	for _, job := range s.dmlJobs {
		err = job.Prepare(conn)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// sleep for slave applying master change
	time.Sleep(5 * time.Second)

	return nil
}

// check runs the check logic of the job. it checks
// 1. the master data
// 2. the master/slave consistency
func (s *Server) check(jobName string, job DMLJob) error {
	time.Sleep(1 * time.Second)
	log.Info("dml check Master consistency for job ", jobName)
	masterID, masterConn := s.getDBConnection()
	masterRecord := job.Check(masterConn)
	if masterRecord != job.GetMaxCounter() {
		// now master has less records than MaxCounter
		// it may not fail only if the old master has unexpected issue, for example, fail to renew lease.
		if atomic.LoadInt32(&s.chaosJobDone) != 2 && atomic.LoadInt32(&s.chaosJobDone) != 3 {
			log.Errorf("job: %s, insert row is %d expected %d for %s",
				jobName, masterRecord, job.GetMaxCounter(), masterID)
			return errors.New("fail to pass the check")
		}
		log.Warnf("dml check Master consistency for job %s not passed, %d expected %d. but continue!",
			jobName, masterRecord, job.GetMaxCounter())
	} else {
		log.Infof("dml check Master consistency for job %s passed!", jobName)
	}

	if atomic.LoadInt32(&s.chaosJobDone) == 3 {
		log.Info("skip M/S consistency check")
		return nil
	}

	// M/S consistency check change to check all nodes under /slave
	registeredNodes, err := s.etcdClient.PrefixGet(s.ctx, "slave")
	if err != nil {
		log.Error("has error when get all registered nodes from etcd. ", err)
		return err
	}
	log.Info("dml check M/S consistency for job ", jobName)
	for id := range registeredNodes {
		id = strings.TrimPrefix(id, "slave/")
		if id == masterID {
			continue
		}
		conn, err := s.buildDBConnFromID(id)
		if err != nil {
			return err
		}
		r := job.Check(conn)
		if r != masterRecord {
			log.Errorf("job: %s, insert row is %d expected %d for %s", jobName, r, masterRecord, id)
			return errors.New("fail to pass the check")
		}
	}
	log.Infof("dml check M/S consistency for job %s passed!", jobName)

	return nil
}

func (s *Server) getContainerFromID(id string) string {
	return s.cfg.IDContainerMapping[id]
}

func (s *Server) runChaosJob() error {
	log.Info("begin to run chaos job ", s.cfg.ChaosJob)
	job, ok := ChaosRegistry[s.cfg.ChaosJob]
	if !ok {
		log.Errorf("cannot find chaos job %s from %v", s.cfg.ChaosJob, ChaosRegistry)
		return errors.New("cannot find target chaos job")
	}
	return job(s)
}

// Status describes the status stored in etcd
type Status struct {
	NodeID       string
	InternalHost string
	ExternalHost string
	IsAlive      bool
}

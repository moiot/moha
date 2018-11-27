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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.mobike.io/database/mysql-agent/pkg/etcd"
	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
)

var (
	leaderPath = "master"
)

// Server is the checker server
type Server struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg *Config

	allLeaders map[string]bool

	// etcdClient interacts with etcd
	etcdClient *etcd.Client

	// db is the db connection
	mysqlConnection *sql.DB
	dbLock          sync.Mutex
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
		allLeaders: make(map[string]bool),
	}, nil

}

// Start starts the checker
func (s *Server) Start() error {
	log.Info("check server starts")

	// get current leader
	leaderID, _, err := s.etcdClient.Get(s.ctx, leaderPath)

	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	log.Info("leader id is ", string(leaderID))
	if leaderID != nil {
		dbConnection, err := s.masterFromLeaderID(string(leaderID))
		if err != nil {
			return err
		}
		s.setDBConnection(dbConnection)
	}
	connectionCh, errorCh := s.getMaster()

	go func() {
		for {
			select {
			case conn := <-connectionCh:
				log.Infof("new connection is %v", conn)
				s.setDBConnection(conn)
			case err := <-errorCh:
				log.Infof("new err is %v", err)
				s.setDBConnection(nil)
			}
		}
	}()

	checkDoneCh := make(chan interface{})
	checkCount := 0
	err = s.initDB()
	if err != nil {
		return err
	}
	checkCount++
	go func() {
		log.Info("run dml ")
		ticker := time.NewTicker(100 * time.Millisecond)
		c := 0
		defer ticker.Stop()
		for range ticker.C {
			if err := s.runDML(c); err != nil {
				log.Error("error when running dml ", err)
				continue
			}
			c++
			if c == s.cfg.MaxCounter {
				err := s.check()
				if err != nil {
					os.Exit(2)
				}
				checkDoneCh <- 1
				return
			}
		}
	}()

	checkCount++
	go func() {
		log.Info("run time-costing txn ")
		total := 10
		c := 0
		for i := 0; i < total; i++ {
			if err := s.runTimeCostingTxn(c); err != nil {
				log.Error(err)
			} else {
				c++
			}
		}
		err := s.checkTimeCostingTxn(c)
		if err != nil {
			os.Exit(2)
		}
		checkDoneCh <- 2
	}()

	go func() {
		log.Info("run changeMaster")
		ticker := time.NewTicker(5000 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			if err := s.runChangeMaster(); err != nil {
				log.Error(err)
			}
		}

	}()

	log.Info("checkCount is ", checkCount)
	for {
		select {
		case r := <-checkDoneCh:
			log.Info("receive from checkDoneCh: ", r)
			checkCount--
			if checkCount == 0 {
				log.Info("check is done so close")
				return nil
			}
		}
	}

}

func (s *Server) getMaster() (chan *sql.DB, chan error) {
	connectionCh := make(chan *sql.DB)
	errorCh := make(chan error)

	go func() {
		watcher := s.etcdClient.NewWatcher()
		watchCh := watcher.Watch(s.ctx,
			s.etcdClient.KeyWithRootPath(leaderPath))

		// get leader changer info
		for {
			select {
			case <-s.ctx.Done():
				return
			case w := <-watchCh:
				var newMasterID string

				for _, e := range w.Events {
					// get the last event
					log.Info("get new leader event ", e)
					if e.Type == mvccpb.DELETE {
						newMasterID = ""
						continue
					}
					if string(e.Kv.Key) == s.etcdClient.KeyWithRootPath(leaderPath) {
						newMasterID = string(e.Kv.Value)
					}
				}
				if newMasterID == "" {
					continue
				}
				if newMasterID == "127.0.0.1:3309" {
					log.Error("mysql-node-3 is a only follow node, cannot be a leader")
					os.Exit(2)
				}
				log.Info("new master id is ", newMasterID)
				s.allLeaders[newMasterID] = true
				dbConnection, err := s.masterFromLeaderID(newMasterID)
				if err != nil {
					log.Error("masterFromLeaderID has error ", err)
					errorCh <- err
					continue
				}
				log.Info("masterFromLeaderID creates new connection ", dbConnection)
				connectionCh <- dbConnection
			}

		}
	}()

	return connectionCh, errorCh

}

func (s *Server) masterFromLeaderID(leaderID string) (*sql.DB, error) {
	hs := strings.Split(leaderID, ":")
	var p int
	if len(hs) < 2 {
		p = 3306
	} else {
		p, _ = strconv.Atoi(hs[1])
	}
	dbConfig := mysql.DBConfig{
		Host:     hs[0],
		User:     s.cfg.DBConfig.User,
		Password: s.cfg.DBConfig.Password,
		Port:     p,
		Timeout:  s.cfg.DBConfig.Timeout,
	}
	log.Info("new dbConfig ", dbConfig)
	return mysql.CreateDB(dbConfig)
}

func (s *Server) getDBConnection() *sql.DB {
	s.dbLock.Lock()
	defer s.dbLock.Unlock()
	return s.mysqlConnection
}

func (s *Server) setDBConnection(db *sql.DB) {
	s.dbLock.Lock()
	defer s.dbLock.Unlock()
	if s.mysqlConnection != nil {
		s.mysqlConnection.Close()
	}
	s.mysqlConnection = db
}

func (s *Server) runChangeMaster() error {
	// get leader
	leaderID, _, err := s.etcdClient.Get(s.ctx, leaderPath)
	if err != nil {
		return nil
	}

	parsedURL, err := url.Parse("http://" + string(leaderID))
	// hack logic
	u := parsedURL.Hostname()
	port := "1" + parsedURL.Port()

	_, err = http.Get(fmt.Sprintf("http://%s:%s/setReadOnly", u, port))
	time.Sleep(3 * time.Second)
	_, err = http.Get(fmt.Sprintf("http://%s:%s/changeMaster", u, port))
	return err
}

func (s *Server) initDB() error {
	var conn *sql.DB
	startTime := time.Now()
	for conn == nil {
		conn = s.getDBConnection()
		if conn == nil {
			log.Info("still no master")
			if time.Now().Sub(startTime) > 10*time.Minute {
				log.Error("10 minutes with no master, exit(2)")
				os.Exit(2)
			}

			time.Sleep(1000 * time.Millisecond)
		}
	}

	log.Info("wait until mysql works fine")
	startTime = time.Now()
	_, err := conn.Query("SELECT 1")
	for err != nil {
		if time.Now().Sub(startTime) > 10*time.Minute {
			log.Error("10 minutes have passed before mysql works fine, exit(2)")
			os.Exit(2)
		}
		log.Warn("select 1 has error, sleep. ", err)
		time.Sleep(500 * time.Millisecond)
		conn = s.getDBConnection()
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

	_, err = conn.Exec(`
					CREATE TABLE checker.check (
					id INT UNSIGNED NOT NULL,
					value VARCHAR(45) NOT NULL
				    )ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
	if err != nil {
		log.Error(err)
		return errors.Trace(err)
	}

	_, err = conn.Exec(`
					CREATE TABLE checker.long_txn (
					id INT UNSIGNED NOT NULL,
					value VARCHAR(45) NOT NULL
				    )ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
	if err != nil {
		log.Error(err)
		return errors.Trace(err)
	}

	return nil
}

func (s *Server) runDML(c int) error {
	conn := s.getDBConnection()
	_, err := conn.Exec("INSERT INTO checker.check(`id`, `value`) VALUES (?, ?)", c, c)
	return err
}

func (s *Server) check() error {
	time.Sleep(1 * time.Second)
	log.Info("dml check M/S consistency")
	allLeaders := s.allLeaders
	for id := range allLeaders {
		conn, err := s.masterFromLeaderID(id)
		if err != nil {
			return err
		}
		row := conn.QueryRow("select count(0) from checker.check")
		r := 0
		row.Scan(&r)
		if r != s.cfg.MaxCounter {
			log.Errorf("insert row is %d expected %d for %s", r, s.cfg.MaxCounter, id)
			return errors.New("fail to pass the check")
		}
	}
	log.Info("dml check M/S consistency passed!")

	return nil
}

func (s *Server) runTimeCostingTxn(c int) error {
	conn := s.getDBConnection()
	txn, err := conn.Begin()
	_, err = txn.Exec("INSERT INTO checker.long_txn(`id`, `value`) VALUES (?, ?)", c, c)
	if err != nil {
		log.Error("has error in time costing txn insert", err)
		txn.Rollback()
		return err
	}
	_, err = txn.Exec("SELECT SLEEP(2)")
	if err != nil {
		txn.Rollback()
		return err
	}
	err = txn.Commit()
	return err

}

func (s *Server) checkTimeCostingTxn(c int) error {
	log.Info("time costing txn check M/S consistency")
	allLeaders := s.allLeaders
	for id := range allLeaders {
		conn, err := s.masterFromLeaderID(id)
		if err != nil {
			return err
		}
		row := conn.QueryRow("select count(0) from checker.long_txn")
		r := 0
		row.Scan(&r)
		if r != c {
			log.Errorf("insert row is %d expected %d for %s", r, c, id)
			return errors.New("fail to pass the check")
		}
	}
	log.Info("time costing txn check M/S consistency success!")
	return nil
}

// Status describes the status stored in etcd
type Status struct {
	NodeID       string
	InternalHost string
	ExternalHost string
	IsAlive      bool
}

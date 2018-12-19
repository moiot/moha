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
	"encoding/json"

	"context"

	"strings"

	"git.mobike.io/database/mysql-agent/pkg/log"
	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"github.com/juju/errors"
)

const electionPath = "election"

// LogForElection describes the necessary information (log) for election
type LogForElection struct {
	Term     uint64 `json:"term"`
	LastUUID string `json:"last_uuid"`
	LastGTID string `json:"last_gtid"`
}

func (s *Server) loadSlaveLogFromMySQL() error {
	rSet, err := mysql.GetSlaveStatus(s.db)
	if err != nil {
		return errors.Trace(err)
	}
	if rSet["Master_UUID"] == "" {
		log.Warn("do not have Master_UUID, this node is not a former slave, "+
			"it may be the initialization of this cluster, slave status is ", rSet)
		return errors.Errorf("do not have Master_UUID, this node is not a former slave, "+
			"it may be the initialization of this cluster, slave status is %+v", rSet)
	}
	s.lastUUID = rSet["Master_UUID"]
	s.lastGTID = rSet["Executed_Gtid_Set"]
	return nil
}

func (s *Server) loadMasterLogFromMySQL() error {
	_, gtidSet, err := mysql.GetMasterStatus(s.db)
	if err != nil {
		return errors.Trace(err)
	}
	uuid, err := mysql.GetServerUUID(s.db)
	if err != nil {
		return errors.Trace(err)
	}
	s.lastUUID = uuid
	s.lastGTID = gtidSet.String()
	return nil
}

func (s *Server) uploadLogForElection() error {
	var electionLog LogForElection
	electionLog.Term = s.term + 1
	electionLog.LastUUID = s.lastUUID
	electionLog.LastGTID = s.lastGTID
	key := join(electionPath, "nodes", s.node.ID())
	latestPosJSONBytes, err := json.Marshal(electionLog)
	if err != nil {
		return errors.Trace(err)
	}
	err = s.node.RawClient().Put(s.ctx, key, string(latestPosJSONBytes))
	return errors.Trace(err)
}

func (s *Server) uploadLogForElectionAsSlave() error {
	err := s.loadSlaveLogFromMySQL()
	if err != nil {
		return errors.Trace(err)
	}
	return s.uploadLogForElection()
}

func (s *Server) uploadLogForElectionAsFormerMaster() error {
	err := s.loadMasterLogFromMySQL()
	if err != nil {
		return errors.Trace(err)
	}
	return s.uploadLogForElection()
}

// getAllLogsForElection retrieves all election logs from etcd
func (s *Server) getAllLogsForElection() (map[string]LogForElection, error) {
	ctx, _ := context.WithTimeout(s.ctx, s.cfg.EtcdDialTimeout)
	kvs, err := s.node.RawClient().PrefixGet(ctx, join(electionPath, "nodes"))
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := make(map[string]LogForElection)

	for k, v := range kvs {
		var logForElection LogForElection
		err = json.Unmarshal(v, &logForElection)
		if err != nil {
			log.Error("error while unmarshalling LogForElection: ", string(v), err)
			continue
		}
		k = strings.TrimLeft(k, join(electionPath, "nodes/"))
		r[k] = logForElection
	}
	return r, nil
}

// isLatestLog compares current agent log with all uploaded log
// returns true if current agent has the latest log, else false
// TODO add isOnlyAlive check for Single Point Master mode
func (s *Server) isLatestLog(logs map[string]LogForElection) (isLatest bool, isSinglePoint bool) {
	myLog, ok := logs[s.node.ID()]
	if !ok {
		// if fail to find my election log from etcd, return false directly
		// this situation should not happen
		log.Errorf("fail to find current node's election log from etcd. serverID: %s, logs: %+v",
			s.node.ID(), logs)
		return false, false
	}
	endTnxID, err := mysql.GetTxnIDFromGTIDStr(myLog.LastGTID, myLog.LastUUID)
	if err != nil {
		// this should not happen
		log.Errorf("error when GetTxnIDFromGTIDStr: %s, %s. err: %v", s.lastGTID, s.lastUUID, err)
		return false, false
	}
	isSinglePoint = true
	for serverID, logForElection := range logs {
		if s.node.ID() == serverID {
			// serverID in map is current server, do not compare
			continue
		}
		if myLog.Term < logForElection.Term {
			log.Infof("%s with log %+v has bigger term ( %d vs %d ), so server %s is not the latest",
				serverID, logForElection, logForElection.Term, s.term, s.node.ID())
			return false, false
		} else if myLog.Term > logForElection.Term {
			log.Infof("%s has bigger term ( %d vs %d ) than server %s",
				s.node.ID(), myLog.Term, logForElection.Term, serverID)
			continue
		}
		// if current agent compares gtid with another agent, it means that they are of the same term,
		// and as a consequence, they are neither not single point
		isSinglePoint = false
		// TODO uuid check?
		anotherTxnID, err := mysql.GetTxnIDFromGTIDStr(logForElection.LastGTID, logForElection.LastUUID)
		if err != nil {
			log.Errorf("error when GetTxnIDFromGTIDStr for server %s: %s, %s. err: %+v ",
				serverID, logForElection.LastGTID, logForElection.LastUUID, err)
			continue
		}
		if endTnxID < anotherTxnID {
			log.Infof("server %s has later txnid (%d vs %d), so server %s is not the latest",
				serverID, anotherTxnID, endTnxID, s.node.ID())
			return false, false
		}
	}
	return true, isSinglePoint
}

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
	"database/sql"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/log"
	"github.com/moiot/moha/pkg/mysql"
	"github.com/moiot/moha/pkg/types"
)

// ServiceManager defines some functions to manage service
type ServiceManager interface {
	// IsReadOnly returns true if current service is read-only, else false
	IsReadOnly() bool
	// SetReadOnly set service readonly
	SetReadOnly() error
	// SetReadWrite set service readwrite
	SetReadWrite() error
	// PromoteToMaster promotes a slave to master
	PromoteToMaster() error
	// RedirectMaster make a slave point to another master
	RedirectMaster(masterHost, masterPort string) error
	// SetReadOnlyManually sets service readonly, but only executed manually
	SetReadOnlyManually() (bool, error)

	LoadSlaveStatusFromDB() (*Position, error)
	LoadMasterStatusFromDB() (*Position, error)
	LoadReplicationInfoOfMaster() (masterUUID, executedGTID string, endTxnID uint64, err error)
	LoadReplicationInfoOfSlave() (masterUUID, executedGTID string, endTxnID uint64, err error)
	GetServerUUID() (string, error)

	Close() error
}

// NewMySQLServiceManager returns the instance of mysqlServiceManager
func NewMySQLServiceManager(dbConfig types.DBConfig, timeout time.Duration) (ServiceManager, error) {
	// try to connect MySQL
	var db *sql.DB
	var err error
	startTime := time.Now()
	for true {
		if time.Now().Sub(startTime) > timeout {
			log.Errorf("timeout to connect to MySQL by user %s", dbConfig.User)
			return nil, errors.Errorf("timeout to connect to MySQL by user %s", dbConfig.User)
		}
		db, err = mysql.CreateDB(dbConfig)
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
	// init ServiceManager
	sm := &mysqlServiceManager{
		db:                  db,
		mysqlNet:            dbConfig.ReplicationNet,
		replicationUser:     dbConfig.ReplicationUser,
		replicationPassword: dbConfig.ReplicationPassword,
	}
	return sm, nil
}

type mysqlServiceManager struct {
	db                  *sql.DB
	mysqlNet            string
	replicationUser     string
	replicationPassword string

	readonly int32
}

func (m *mysqlServiceManager) IsReadOnly() bool {
	return atomic.LoadInt32(&m.readonly) == 1
}

func (m *mysqlServiceManager) SetReadOnly() error {
	err := mysql.SetReadOnly(m.db)
	if err != nil {
		return err
	}
	atomic.StoreInt32(&m.readonly, 1)
	return nil
}
func (m *mysqlServiceManager) SetReadWrite() error {
	err := mysql.SetReadWrite(m.db)
	if err != nil {
		return err
	}
	atomic.StoreInt32(&m.readonly, 0)
	return nil
}
func (m *mysqlServiceManager) PromoteToMaster() error {
	return mysql.PromoteToMaster(m.db, m.replicationUser, m.mysqlNet)
}
func (m *mysqlServiceManager) RedirectMaster(masterHost, masterPort string) error {
	return mysql.RedirectMaster(m.db,
		m.replicationUser, m.replicationPassword,
		masterHost, masterPort)
}
func (m *mysqlServiceManager) SetReadOnlyManually() (bool, error) {
	ddl, err := mysql.GetRunningDDLCount(m.db)
	if err != nil {
		return false, errors.Trace(err)
	}
	if ddl != 0 {
		return false, errors.Errorf("has running ddl before setting readonly")
	}
	err = m.SetReadOnly()
	if err != nil {
		return false, m.rollbackReadOnly(err)
	}
	ddl, err = mysql.GetRunningDDLCount(m.db)
	if err != nil {
		return false, m.rollbackReadOnly(err)
	}
	if ddl != 0 {
		return false, m.rollbackReadOnly(errors.Errorf("has running ddl after setting readonly"))
	}
	return true, nil

}

func (m *mysqlServiceManager) rollbackReadOnly(readonlyErr error) error {
	anotherErr := m.SetReadWrite()
	if anotherErr != nil {
		log.Error("has error when rollback readonly. err: ", anotherErr)
		return errors.Wrap(readonlyErr, anotherErr)
	}
	return readonlyErr
}

func (m *mysqlServiceManager) Close() error {
	return mysql.CloseDB(m.db)
}

func (m *mysqlServiceManager) LoadMasterStatusFromDB() (*Position, error) {
	pos, gtidSet, err := mysql.GetMasterStatus(m.db)
	if err != nil {
		// error in getting MySQL binlog position
		// TODO change alive status or retry
		return nil, err
	}
	currentPos := &Position{}

	currentPos.File = pos.Name
	currentPos.Pos = fmt.Sprint(pos.Pos)
	currentPos.GTID = gtidSet.String()
	if uuid, err := mysql.GetServerUUID(m.db); err == nil {
		currentPos.UUID = uuid
		// assume the gtidset is continuous, only pick the last one
		endTxnID, err := mysql.GetTxnIDFromGTIDStr(currentPos.GTID, uuid)
		if err != nil {
			log.Warnf("error when GetTxnIDFromGTIDStr gtidStr: %s, gtid: %s, error is %v",
				currentPos.GTID, uuid, err)
			return nil, errors.Errorf("error when GetTxnIDFromGTIDStr gtidStr: %s, gtid: %s, error is %v",
				currentPos.GTID, uuid, err)
		}
		currentPos.EndTxnID = uint64(endTxnID)
	} else {
		return nil, errors.Errorf("fail to get server UUID %v", err)
	}

	return currentPos, nil
}

func (m *mysqlServiceManager) LoadSlaveStatusFromDB() (*Position, error) {
	rSet, err := mysql.GetSlaveStatus(m.db)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if rSet["Relay_Master_Log_File"] == "" {
		log.Warn("do not have Relay_Master_Log_File, this node is not a former slave, "+
			"it may be the initialization of this cluster, slave status is ", rSet)
		return nil, errors.New("do not have Relay_Master_Log_File")
	}
	currentPos := &Position{}
	currentPos.File = rSet["Relay_Master_Log_File"]
	currentPos.Pos = rSet["Exec_Master_Log_Pos"]
	currentPos.GTID = rSet["Executed_Gtid_Set"]
	currentPos.UUID = latestPos.UUID

	// Seconds_Behind_Master value loading
	if rSet["Seconds_Behind_Master"] == "" {
		log.Errorf("Seconds_Behind_Master is empty, show slave status result is %+v", rSet)
	} else {
		sbm, err := strconv.Atoi(rSet["Seconds_Behind_Master"])
		if err != nil {
			log.Warnf("has error in atoi Seconds_Behind_Master: %s. still use previous value: %d. error is %+v",
				rSet["Seconds_Behind_Master"], latestPos.SecondsBehindMaster, err)
			currentPos.SecondsBehindMaster = latestPos.SecondsBehindMaster
		} else {
			currentPos.SecondsBehindMaster = sbm
		}
	}

	if rSet["Slave_SQL_Running"] == "Yes" {
		currentPos.SlaveSQLRunning = true
	} else {
		currentPos.SlaveSQLRunning = false
	}
	if rSet["Slave_IO_Running"] == "Yes" {
		currentPos.SlaveIORunning = true
	} else {
		currentPos.SlaveIORunning = false
	}

	if rSet["Master_UUID"] != "" {
		// assume the gtidset is continuous, only pick the last one
		endTxnID, err := mysql.GetTxnIDFromGTIDStr(currentPos.GTID, rSet["Master_UUID"])
		if err != nil {
			log.Warnf("error when GetTxnIDFromGTIDStr gtidStr: %s, gtid: %s, error is %v",
				currentPos.GTID, rSet["Master_UUID"], err)
			return nil, errors.Errorf("error when GetTxnIDFromGTIDStr gtidStr: %s, gtid: %s, error is %v",
				currentPos.GTID, rSet["Master_UUID"], err)
		}
		currentPos.EndTxnID = uint64(endTxnID)
	}

	return currentPos, nil
}

func (m *mysqlServiceManager) LoadReplicationInfoOfSlave() (masterUUID, executedGTID string, endTxnID uint64, err error) {
	rSet, err := mysql.GetSlaveStatus(m.db)
	if err != nil {
		return masterUUID, executedGTID, endTxnID, errors.Trace(err)
	}
	if rSet["Master_UUID"] == "" {
		log.Warn("do not have Master_UUID, this node is not a former slave, "+
			"it may be the initialization of this cluster, slave status is ", rSet)
		return masterUUID, executedGTID, endTxnID, errors.Errorf("do not have Master_UUID, "+
			"this node is not a former slave, it may be the initialization of this cluster, slave status is %+v", rSet)
	}
	endTxnIDInt64, err := mysql.GetTxnIDFromGTIDStr(rSet["Executed_Gtid_Set"], rSet["Master_UUID"])
	if err != nil {
		return masterUUID, executedGTID, endTxnID, errors.Trace(err)
	}

	return rSet["Master_UUID"], rSet["Executed_Gtid_Set"], uint64(endTxnIDInt64), nil
}

func (m *mysqlServiceManager) LoadReplicationInfoOfMaster() (masterUUID, executedGTID string, endTxnID uint64, err error) {
	_, gtidSet, err := mysql.GetMasterStatus(m.db)
	if err != nil {
		return masterUUID, executedGTID, endTxnID, errors.Trace(err)
	}
	uuid, err := mysql.GetServerUUID(m.db)
	if err != nil {
		return masterUUID, executedGTID, endTxnID, errors.Trace(err)
	}
	endTxnIDInt64, err := mysql.GetTxnIDFromGTIDStr(gtidSet.String(), uuid)
	if err != nil {
		return masterUUID, executedGTID, endTxnID, errors.Trace(err)
	}

	return uuid, gtidSet.String(), uint64(endTxnIDInt64), nil
}

func (m *mysqlServiceManager) GetServerUUID() (string, error) {
	return mysql.GetServerUUID(m.db)
}

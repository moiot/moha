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

	"time"

	"git.mobike.io/database/mysql-agent/pkg/log"
	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"github.com/juju/errors"
)

// ServiceManager defines some functions to manage service
type ServiceManager interface {
	// SetReadOnly set service readonly
	SetReadOnly() error
	// SetReadWrite set service readwrite
	SetReadWrite() error
	// PromoteToMaster promotes a slave to master
	PromoteToMaster() error
	// RedirectMaster make a slave point to another master
	RedirectMaster(masterHost, masterPort string) error
	// WaitForRunningProcesses block until running processes are done, or are killed after timeout seconds
	WaitForRunningProcesses(timeout int) error
	// WaitCatchMaster returns a channel which is closed when current service catches the given gtid
	WaitCatchMaster(gtid string) chan interface{}
	// SetReadOnlyManually sets service readonly, but only executed manually
	SetReadOnlyManually() (bool, error)

	Close() error
}

type mysqlServiceManager struct {
	db                  *sql.DB
	mysqlNet            string
	replicationUser     string
	replicationPassword string
}

func (m *mysqlServiceManager) SetReadOnly() error {
	return mysql.SetReadOnly(m.db)
}
func (m *mysqlServiceManager) SetReadWrite() error {
	return mysql.SetReadWrite(m.db)
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

func (m *mysqlServiceManager) WaitCatchMaster(gtid string) chan interface{} {
	r := make(chan interface{})
	go func() {
		err := mysql.WaitCatchMaster(m.db, gtid)
		if err != nil {
			log.Warn("error when WaitCatchMaster ", err)
		}
		close(r)
	}()
	return r
}

func (m *mysqlServiceManager) WaitForRunningProcesses(timeout int) error {

	pids, err := mysql.GetRunningProcesses(m.db)
	if err != nil {
		return errors.Trace(err)
	}
	if len(pids) == 0 {
		return nil
	}
	time.Sleep(time.Duration(timeout) * time.Second)
	pids, err = mysql.GetRunningProcesses(m.db)
	if err != nil {
		return errors.Trace(err)
	}
	for _, pid := range pids {
		err = mysql.KillProcess(m.db, pid)
		if err != nil {
			log.Warn("[ignore] error when killing pid ", pid, err)
		}
	}
	return nil
}

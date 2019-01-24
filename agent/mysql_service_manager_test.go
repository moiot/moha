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
	"gopkg.in/DATA-DOG/go-sqlmock.v1"
	. "gopkg.in/check.v1"
)

var _ = Suite(&testMySQLServiceManagerSuite{})

type testMySQLServiceManagerSuite struct{}

func (t *testMySQLServiceManagerSuite) TestSetReadOnly(c *C) {

	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectExec("SET GLOBAL super_read_only = 1;").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SET GLOBAL read_only = 1;").WillReturnResult(sqlmock.NewResult(0, 0))

	testedManager := &mysqlServiceManager{
		db: db,
	}

	err = testedManager.SetReadOnly()
	c.Assert(err, IsNil)

	isReadonly := testedManager.IsReadOnly()
	c.Assert(isReadonly, Equals, true)
}

func (t *testMySQLServiceManagerSuite) TestSetReadWrite(c *C) {

	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectExec("SET GLOBAL read_only = 0;").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SET GLOBAL super_read_only = 0;").WillReturnResult(sqlmock.NewResult(0, 0))

	testedManager := &mysqlServiceManager{
		db: db,
	}

	err = testedManager.SetReadWrite()
	c.Assert(err, IsNil)

	isReadonly := testedManager.IsReadOnly()
	c.Assert(isReadonly, Equals, false)
}

func (t *testMySQLServiceManagerSuite) TestPromoteToMaster(c *C) {

	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectExec("STOP SLAVE;").WillReturnResult(sqlmock.NewResult(0, 0))

	testedManager := &mysqlServiceManager{
		db: db,
	}

	err = testedManager.PromoteToMaster()
	c.Assert(err, IsNil)
}

func (t *testMySQLServiceManagerSuite) TestRedirectMaster(c *C) {

	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectExec("STOP SLAVE;").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`CHANGE MASTER TO
        master_user='master',master_host='master_host', MASTER_PORT=3306, master_password='master_password', MASTER_AUTO_POSITION = 1;`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("START SLAVE;").WillReturnResult(sqlmock.NewResult(0, 0))

	testedManager := &mysqlServiceManager{
		db:                  db,
		replicationUser:     "master",
		replicationPassword: "master_password",
	}

	err = testedManager.RedirectMaster("master_host", "3306")
	c.Assert(err, IsNil)
}

func (t *testMySQLServiceManagerSuite) TestLoadMasterStatusFromDB(c *C) {
	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectQuery("SHOW MASTER STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"File", "Position", "Binlog_Do_DB", "Binlog_Ignore_DB", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "", "", "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46"))

	mock.ExpectQuery("SELECT @@server_uuid").
		WillReturnRows(sqlmock.
			NewRows([]string{"@@server_uuid"}).
			AddRow("85ab69d1-b21f-11e6-9c5e-64006a8978d2"))

	testedManager := &mysqlServiceManager{
		db: db,
	}
	pos, err := testedManager.LoadMasterStatusFromDB()
	c.Assert(err, IsNil)

	c.Assert(pos.File, Equals, "mysql-bin.000005")
	c.Assert(pos.Pos, Equals, "188858056")
	c.Assert(pos.GTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")
	c.Assert(pos.UUID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2")
	c.Assert(pos.EndTxnID, Equals, uint64(47))
}

func (t *testMySQLServiceManagerSuite) TestLoadSlaveStatusFromDB(c *C) {
	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectQuery("SHOW SLAVE STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"Relay_Master_Log_File", "Exec_Master_Log_Pos", "Executed_Gtid_Set",
				"Master_UUID", "Slave_IO_Running", "Slave_SQL_Running", "Seconds_Behind_Master"}).
			AddRow("mysql-bin.000005", 188858056, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
				"85ab69d1-b21f-11e6-9c5e-64006a8978d2", "Yes", "No", "5"))
	testedManager := &mysqlServiceManager{
		db: db,
	}
	pos, err := testedManager.LoadSlaveStatusFromDB()
	c.Assert(err, IsNil)

	c.Assert(pos.File, Equals, "mysql-bin.000005")
	c.Assert(pos.Pos, Equals, "188858056")
	c.Assert(pos.GTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")
	c.Assert(pos.SlaveIORunning, Equals, true)
	c.Assert(pos.SlaveSQLRunning, Equals, false)
	c.Assert(pos.SecondsBehindMaster, Equals, 5)
	c.Assert(pos.EndTxnID, Equals, uint64(47))
}

func (t *testMySQLServiceManagerSuite) TestLoadReplicationInfoOfMaster(c *C) {
	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectQuery("SHOW MASTER STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"File", "Position", "Binlog_Do_DB", "Binlog_Ignore_DB", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "", "", "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46"))
	mock.ExpectQuery("SELECT @@server_uuid").
		WillReturnRows(sqlmock.
			NewRows([]string{"@@server_uuid"}).
			AddRow("85ab69d1-b21f-11e6-9c5e-64006a8978d2"))

	testedManager := &mysqlServiceManager{
		db: db,
	}

	masterUUID, executedGTID, endTxnID, err :=
		testedManager.LoadReplicationInfoOfMaster()
	c.Assert(err, IsNil)
	c.Assert(masterUUID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2")
	c.Assert(executedGTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")
	c.Assert(endTxnID, Equals, uint64(47))

}

func (t *testMySQLServiceManagerSuite) TestLoadReplicationInfoOfSlave(c *C) {
	db, mock, err := sqlmock.New()
	c.Assert(err, IsNil)
	mock.ExpectQuery("SHOW SLAVE STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"Relay_Master_Log_File", "Exec_Master_Log_Pos", "Executed_Gtid_Set",
				"Master_UUID", "Slave_IO_Running", "Slave_SQL_Running", "Seconds_Behind_Master"}).
			AddRow("mysql-bin.000005", 188858056, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
				"85ab69d1-b21f-11e6-9c5e-64006a8978d2", "Yes", "No", "5"))

	testedManager := &mysqlServiceManager{
		db: db,
	}

	masterUUID, executedGTID, endTxnID, err :=
		testedManager.LoadReplicationInfoOfSlave()
	c.Assert(err, IsNil)
	c.Assert(masterUUID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2")
	c.Assert(executedGTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")
	c.Assert(endTxnID, Equals, uint64(47))

}

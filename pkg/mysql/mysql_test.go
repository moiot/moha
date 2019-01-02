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

package mysql_test

import (
	_ "database/sql"
	"testing"

	"github.com/moiot/moha/pkg/log"
	"github.com/moiot/moha/pkg/mysql"
	"gopkg.in/DATA-DOG/go-sqlmock.v1"
	. "gopkg.in/check.v1"
)

type testMySQLSuite struct{}

var _ = Suite(&testMySQLSuite{})

// Hock into "go test" runner.
func TestMySQL(t *testing.T) {
	TestingT(t)
}

func (s *testMySQLSuite) TestGetMasterStatus(c *C) {

	// mock mysql
	db, mock, err := sqlmock.New()
	if err != nil {
		log.Error("error when mock mysql ", err)
		c.Fail()
	}
	defer db.Close()

	// mock behaviour
	mock.ExpectQuery("SHOW MASTER STATUS").
		WillReturnRows(sqlmock.
			NewRows([]string{"File", "Position", "Binlog_Do_DB", "Binlog_Ignore_DB", "Executed_Gtid_Set"}).
			AddRow("mysql-bin.000005", 188858056, "", "", "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46:49-50"))

	binlogPos, gtid, err := mysql.GetMasterStatus(db)
	c.Assert(err, IsNil)
	c.Assert(binlogPos.Name, Equals, "mysql-bin.000005")
	c.Assert(binlogPos.Pos, Equals, uint32(188858056))
	// because master is reset.
	c.Assert(gtid.Sets, HasLen, 1)
	c.Assert(gtid.Sets["85ab69d1-b21f-11e6-9c5e-64006a8978d2"].Intervals, HasLen, 2)
	c.Assert(gtid.Sets["85ab69d1-b21f-11e6-9c5e-64006a8978d2"].Intervals[0].Start, Equals, int64(1))
	c.Assert(gtid.Sets["85ab69d1-b21f-11e6-9c5e-64006a8978d2"].Intervals[0].Stop, Equals, int64(47))
}

func (s *testMySQLSuite) TestGetTxnIDFromGTIDStr(c *C) {
	txnID, err := mysql.GetTxnIDFromGTIDStr("dde19958-0296-11e9-99b2-0242ac130008:0", "dde19958-0296-11e9-99b2-0242ac130008")
	c.Assert(err, IsNil)
	c.Assert(txnID, Equals, int64(1))

}

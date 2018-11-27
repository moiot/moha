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

package mysql

import (
	"database/sql"
	"fmt"

	"git.mobike.io/database/mysql-agent/pkg/log"
	_ "github.com/go-sql-driver/mysql" // import mysql for side-effects
	"github.com/juju/errors"
	gmysql "github.com/siddontang/go-mysql/mysql"
)

// DBConfig is the DB configuration.
type DBConfig struct {
	Host     string `toml:"host" json:"host"`
	User     string `toml:"user" json:"user"`
	Password string `toml:"password" json:"password"`
	Port     int    `toml:"port" json:"port"`

	Timeout string `toml:"timeout" json:"timeout"`

	ReplicationUser     string `toml:"replication_user" json:"replication_user"`
	ReplicationPassword string `toml:"replication_password" json:"replication_password"`
	ReplicationNet      string `toml:"replication_net" json:"replication_net"`
}

// GTIDSet wraps mysql.MysqlGTIDSet
type GTIDSet struct {
	*gmysql.MysqlGTIDSet
}

func parseGTIDSet(gtidStr string) (GTIDSet, error) {
	gs, err := gmysql.ParseMysqlGTIDSet(gtidStr)
	if err != nil {
		return GTIDSet{}, errors.Trace(err)
	}

	return GTIDSet{gs.(*gmysql.MysqlGTIDSet)}, nil
}

// GetTxnIDFromGTIDStr get the last txn id from the gtidset and serverUUID
func GetTxnIDFromGTIDStr(gtidStr, serverUUID string) (int64, error) {

	gtidSet, err := parseGTIDSet(gtidStr)
	if err != nil {
		return 0, errors.Annotatef(err, " error when parsing gtidSet %s ", gtidStr)
	}
	uuidSet, ok := gtidSet.Sets[serverUUID]
	if !ok {
		return 0, errors.Errorf("cannot find gtid %s from gtidSet %s", serverUUID, gtidSet.String())
	}
	intervalLen := len(uuidSet.Intervals)
	if intervalLen == 0 {
		return 0, errors.Errorf("cannot find intervals with gtid %s from gtidSet %s", serverUUID, gtidSet.String())
	}
	// assume the gtidset is continuous, only pick the last one
	return uuidSet.Intervals[intervalLen-1].Stop, nil

}

// CreateDB creates db connection using the cfg
func CreateDB(cfg DBConfig) (*sql.DB, error) {
	dbDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8&interpolateParams=true&readTimeout=%s&writeTimeout=%s&timeout=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Timeout, cfg.Timeout, cfg.Timeout)
	db, err := sql.Open("mysql", dbDSN)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return db, nil
}

// CloseDB closes the db connection
func CloseDB(db *sql.DB) error {
	if db == nil {
		return nil
	}

	return errors.Trace(db.Close())
}

// Select1 is used to test connection
func Select1(db *sql.DB) error {
	_, err := db.Query("SELECT 1;")
	return err
}

// GetServerUUID returns the uuid of current mysql server
func GetServerUUID(db *sql.DB) (string, error) {
	var masterUUID string
	rows, err := db.Query(`SELECT @@server_uuid;`)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer rows.Close()

	// Show an example.
	/*
	   MySQL [test]> SELECT @@server_uuid;
	   +--------------------------------------+
	   | @@server_uuid                        |
	   +--------------------------------------+
	   | 53ea0ed1-9bf8-11e6-8bea-64006a897c73 |
	   +--------------------------------------+
	*/
	for rows.Next() {
		err = rows.Scan(&masterUUID)
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	if rows.Err() != nil {
		return "", errors.Trace(rows.Err())
	}
	return masterUUID, nil
}

// GetSlaveStatus runs `show slave status` ans return the result as a map[string]string
func GetSlaveStatus(db *sql.DB) (map[string]string, error) {
	r, err := db.Query("SHOW SLAVE STATUS ")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer r.Close()
	columns, err := r.Columns()
	if err != nil {
		return nil, errors.Trace(err)
	}
	data := make([][]byte, len(columns))
	dataP := make([]interface{}, len(columns))
	for i := 0; i < len(columns); i++ {
		dataP[i] = &data[i]
	}
	if r.Next() {
		r.Scan(dataP...)
	}

	resultMap := make(map[string]string)
	for i, column := range columns {
		resultMap[column] = string(data[i])
	}

	return resultMap, nil

}

// GetMasterStatus shows master status of MySQL.
func GetMasterStatus(db *sql.DB) (gmysql.Position, GTIDSet, error) {
	var (
		binlogPos gmysql.Position
		gs        GTIDSet
	)
	rows, err := db.Query(`SHOW MASTER STATUS`)
	if err != nil {
		return binlogPos, gs, errors.Trace(err)
	}
	defer rows.Close()

	rowColumns, err := rows.Columns()
	if err != nil {
		return binlogPos, gs, errors.Trace(err)
	}

	// Show an example.
	/*
		MySQL [test]> SHOW MASTER STATUS;
		+-----------+----------+--------------+------------------+--------------------------------------------+
		| File      | Position | Binlog_Do_DB | Binlog_Ignore_DB | Executed_Gtid_Set                          |
		+-----------+----------+--------------+------------------+--------------------------------------------+
		| ON.000001 |     4822 |              |                  | 85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46
		+-----------+----------+--------------+------------------+--------------------------------------------+
	*/
	var (
		gtid       string
		binlogName string
		pos        uint32
		nullPtr    interface{}
	)
	for rows.Next() {
		if len(rowColumns) == 5 {
			err = rows.Scan(&binlogName, &pos, &nullPtr, &nullPtr, &gtid)
		} else {
			err = rows.Scan(&binlogName, &pos, &nullPtr, &nullPtr)
		}
		if err != nil {
			return binlogPos, gs, errors.Trace(err)
		}

		binlogPos = gmysql.Position{
			Name: binlogName,
			Pos:  pos,
		}

		gs, err = parseGTIDSet(gtid)
		if err != nil {
			return binlogPos, gs, errors.Trace(err)
		}
	}
	if rows.Err() != nil {
		return binlogPos, gs, errors.Trace(rows.Err())
	}

	return binlogPos, gs, nil
}

// SetReadOnly add read lock to db
func SetReadOnly(db *sql.DB) error {
	// _, err := db.Exec("FLUSH TABLES;")
	_, err := db.Exec("SET GLOBAL read_only = 1;")

	return err
}

// SetReadWrite release read lock to db
func SetReadWrite(db *sql.DB) error {
	_, err := db.Exec("SET GLOBAL read_only = 0;")
	return err
}

// PromoteToMaster promotes the db to master
func PromoteToMaster(db *sql.DB, replicationUser string, mysqlNet string) error {
	_, err := db.Exec("STOP SLAVE;")
	// _, err = db.Exec("RESET MASTER;")
	// _, err = db.Exec(fmt.Sprintf("GRANT REPLICATION SLAVE ON *.* TO '%s'@'%s';", replicationUser, mysqlNet))

	return err
}

// RedirectMaster directs db to a different master
func RedirectMaster(db *sql.DB, replicationUser, replicationPassword, masterHost string, masterPort string) error {
	_, err := db.Exec("STOP SLAVE;")

	changeMasterSQL := fmt.Sprintf(`CHANGE MASTER TO
        master_user='%s',master_host='%s', MASTER_PORT=%s, master_password='%s', MASTER_AUTO_POSITION = 1;`,
		replicationUser, masterHost, masterPort, replicationPassword)
	log.Info("run ", changeMasterSQL)
	_, err = db.Exec(changeMasterSQL)
	_, err = db.Exec("START SLAVE;")

	return err
}

// GetRunningProcesses get all running processes
func GetRunningProcesses(db *sql.DB) ([]string, error) {
	rs, err := db.Query("SELECT id FROM INFORMATION_SCHEMA.PROCESSLIST where USER NOT IN ('root', 'replication', 'system user')")
	if err != nil {
		return nil, err
	}
	pids := make([]string, 0)
	for rs.Next() {
		var pid string
		rs.Scan(&pid)
		pids = append(pids, pid)
	}
	return pids, nil
}

// GetRunningDDLCount returns the count of ddls that are running
func GetRunningDDLCount(db *sql.DB) (int, error) {
	rs, err := db.Query("SELECT COUNT(0) FROM INFORMATION_SCHEMA.PROCESSLIST WHERE USER NOT IN ('root', 'replication', 'system user') AND INFO LIKE 'ALTER%'")
	if err != nil {
		return 0, errors.Trace(err)
	}
	var count int
	if rs.Next() {
		rs.Scan(&count)
	}
	return count, nil
}

// KillProcess kill the process given the pid
func KillProcess(db *sql.DB, pid string) error {
	_, err := db.Exec("kill " + pid)
	return err
}

// WaitCatchMaster blocks until mysql catches given gtid
func WaitCatchMaster(db *sql.DB, gtid string) error {
	_, err := db.Exec(fmt.Sprintf("SELECT WAIT_UNTIL_SQL_THREAD_AFTER_GTIDS('%s')", gtid))
	return err
}

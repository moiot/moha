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
	"database/sql"

	"time"

	"github.com/moiot/moha/pkg/log"
)

// DMLJob is the abstraction of the writing and checking job
type DMLJob interface {
	Prepare(db *sql.DB) error
	RunDML(db *sql.DB, counter int) error
	Check(db *sql.DB) int
	GetInterval() time.Duration
	GetMaxCounter() int
	GetCheckWaitTime() time.Duration
}

// SimpleDMLJob is the DMLJob that runs simple DML
type SimpleDMLJob struct{}

// Prepare is used for DB prepare
func (job SimpleDMLJob) Prepare(db *sql.DB) error {
	_, err := db.Exec(`
					CREATE TABLE checker.check (
					id INT UNSIGNED NOT NULL,
					value VARCHAR(45) NOT NULL
				    )ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
	return err
}

// RunDML is run dml
func (job SimpleDMLJob) RunDML(db *sql.DB, c int) error {
	_, err := db.Exec("INSERT INTO checker.check(`id`, `value`) VALUES (?, ?)", c, c)
	return err

}

// Check is how this job is checked
func (job SimpleDMLJob) Check(db *sql.DB) int {
	row := db.QueryRow("select count(0) from checker.check")
	r := -1
	row.Scan(&r)
	return r
}

// GetInterval returns the interval of running dml
func (job SimpleDMLJob) GetInterval() time.Duration {
	return 100 * time.Millisecond
}

// GetMaxCounter return the times that the dml will be run
func (job SimpleDMLJob) GetMaxCounter() int {
	return 500
}

// GetCheckWaitTime returns the time to wait before check
func (job SimpleDMLJob) GetCheckWaitTime() time.Duration {
	return 5 * time.Second
}

// LongTxnJob is the DMLJob that runs time costing DML
type LongTxnJob struct{}

// Prepare is used for DB prepare
func (job LongTxnJob) Prepare(db *sql.DB) error {
	_, err := db.Exec(`
					CREATE TABLE checker.long_txn (
					id INT UNSIGNED NOT NULL,
					value VARCHAR(45) NOT NULL
				    )ENGINE=InnoDB DEFAULT CHARSET=utf8;`)
	return err
}

// RunDML is run dml
func (job LongTxnJob) RunDML(db *sql.DB, c int) error {
	conn := db
	txn, err := conn.Begin()
	if err != nil {
		log.Error("has error in time costing txn begin txn ", err)
		return err
	}
	_, err = txn.Exec("INSERT INTO checker.long_txn(`id`, `value`) VALUES (?, ?)", c, c)
	if err != nil {
		log.Error("has error in time costing txn insert", err)
		txn.Rollback()
		return err
	}
	_, err = txn.Exec("SELECT SLEEP(1)")
	err = txn.Commit()
	if err != nil {
		err = txn.Rollback()
		return err
	}
	return err

}

// Check is how this job is checked
func (job LongTxnJob) Check(db *sql.DB) int {
	row := db.QueryRow("select count(0) from checker.long_txn")
	r := -1
	row.Scan(&r)
	return r
}

// GetInterval returns the interval of running dml
func (job LongTxnJob) GetInterval() time.Duration {
	return 3 * time.Second
}

// GetMaxCounter return the times that the dml will be run
func (job LongTxnJob) GetMaxCounter() int {
	return 5
}

// GetCheckWaitTime returns the time to wait before check
func (job LongTxnJob) GetCheckWaitTime() time.Duration {
	return 30 * time.Second
}

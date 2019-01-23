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

package postgresql

import (
	"database/sql"
	"fmt"

	"github.com/juju/errors"
	_ "github.com/lib/pq" // import pq for side-effects
	"github.com/moiot/moha/pkg/types"
)

// CreateDB creates db connection using the cfg
func CreateDB(cfg types.DBConfig) (*sql.DB, error) {
	dbDSN := fmt.Sprintf("postgres://%s:%s@%s:%d/?sslmode=disable&connect_timeout=%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Timeout)
	db, err := sql.Open("postgres", dbDSN)
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

// ReloadConf make PG reload its conf
func ReloadConf(db *sql.DB) error {
	_, err := db.Query("SELECT pg_reload_conf();")
	return err
}

// GetCurrentLSN return the result of SELECT pg_current_wal_lsn()
func GetCurrentLSN(db *sql.DB) (uint64, error) {
	rs, err := db.Query("SELECT pg_current_wal_lsn();")
	if err != nil {
		return 0, errors.Trace(err)
	}
	defer rs.Close()
	var r uint64
	if rs.Next() {
		rs.Scan(&r)
	}
	if err != nil {
		return 0, errors.Trace(err)
	}
	return r, nil
}

// GetLastReplayLSN returns the result of SELECT pg_last_wal_replay_lsn()
func GetLastReplayLSN(db *sql.DB) (uint64, error) {
	rs, err := db.Query("SELECT pg_last_wal_replay_lsn();")
	if err != nil {
		return 0, errors.Trace(err)
	}
	defer rs.Close()
	var r uint64
	if rs.Next() {
		rs.Scan(&r)
	}
	if err != nil {
		return 0, errors.Trace(err)
	}
	return r, nil
}

// Checkpoint executes `CHECKPOINT;` in PG
func Checkpoint(db *sql.DB) error {
	_, err := db.Exec("CHECKPOINT;")
	return err
}

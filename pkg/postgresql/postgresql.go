package postgresql

import (
	"database/sql"
	"fmt"

	"git.mobike.io/database/mysql-agent/pkg/types"
	"github.com/juju/errors"
	_ "github.com/lib/pq" // import pq for side-effects
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

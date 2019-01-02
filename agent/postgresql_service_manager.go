package agent

import (
	"bytes"
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"syscall"
	"time"

	"git.mobike.io/database/mysql-agent/pkg/log"
	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"git.mobike.io/database/mysql-agent/pkg/postgresql"
	"git.mobike.io/database/mysql-agent/pkg/types"
	"github.com/juju/errors"
)

const redirectTemplate = `standby_mode = 'on'
primary_conninfo = 'user=%s password=%s host=%s port=%s'
recovery_target_timeline = 'latest'`

// NewPostgreSQLServiceManager returns the instance of mysqlServiceManager
func NewPostgreSQLServiceManager(dbConfig types.DBConfig, timeout time.Duration) (ServiceManager, error) {
	// try to connect PostgreSQL
	var db *sql.DB
	var err error
	startTime := time.Now()
	for true {
		if time.Now().Sub(startTime) > timeout {
			log.Errorf("timeout to connect to PostgreSQL by user %s", dbConfig.User)
			return nil, errors.Errorf("timeout to connect to PostgreSQL by user %s", dbConfig.User)
		}
		db, err = postgresql.CreateDB(dbConfig)
		if err != nil {
			log.Errorf("fail to connect PostgreSQL in agent start: %+v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		err = mysql.Select1(db)
		if err != nil {
			log.Errorf("fail to select 1 from PostgreSQL in agent start: %+v", err)
			db.Close()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		break
	}
	log.Info("db is successfully connected and select 1 is OK")
	// init ServiceManager
	sm := &postgresqlServiceManager{
		db:                  db,
		replicationUser:     dbConfig.ReplicationUser,
		replicationPassword: dbConfig.ReplicationPassword,
	}
	return sm, nil
}

type postgresqlServiceManager struct {
	db                  *sql.DB
	replicationUser     string
	replicationPassword string
}

func (m *postgresqlServiceManager) SetReadOnly() error {

	// change postgresql.conf
	_, _, err := runCommand("sed", "-i",
		"s/default_transaction_read_only = off/default_transaction_read_only = on/",
		"/var/lib/postgresql/data/postgresql.conf")
	// reload postgresql config
	log.Info("reload postgresql config")
	err = postgresql.ReloadConf(m.db)
	return err
}
func (m *postgresqlServiceManager) SetReadWrite() error {
	// change postgresql.conf
	_, _, err := runCommand("sed", "-i",
		"s/default_transaction_read_only = on/default_transaction_read_only = off/",
		"/var/lib/postgresql/data/postgresql.conf")
	// reload postgresql config
	log.Info("reload postgresql config")
	err = postgresql.ReloadConf(m.db)
	return err
}
func (m *postgresqlServiceManager) PromoteToMaster() error {

	stdout, stderr, err := runCommand("pg_ctl", "promote")
	if err != nil {
		log.Error("has error when pg_ctl promote. ",
			err, stdout, stderr)
	}
	return err
}
func (m *postgresqlServiceManager) RedirectMaster(masterHost, masterPort string) error {
	// stop postgresql process
	log.Info("run pg_ctl stop")
	stdout, stderr, err := runCommand("pg_ctl", "stop")
	if err != nil {
		log.Error("has error when pg_ctl stop. ",
			err, stdout, stderr)
		// return errors.Trace(err)
	} else {
		log.Info("run pg_ctl stop success")
	}
	// pg_rewind
	log.Info("run pg_rewind")
	stdout, stderr, err = runCommand("pg_rewind",
		"--target-pgdata=/var/lib/postgresql/data",
		fmt.Sprintf("--source-server=host=%s port=%s user=postgres dbname=postgres",
			masterHost, masterPort))
	if err != nil {
		log.Error("has error when pg_rewind. ",
			err, stdout, stderr)
		log.Info("try using pg_basebackup")
		err = os.RemoveAll("/var/lib/postgresql/data")
		if err != nil {
			log.Error("has error when rm -rf /var/lib/postgresql/data ", err)
		}
		stdout, stderr, err := runCommand("pg_basebackup",
			"--write-recovery-conf",
			"--pgdata=/var/lib/postgresql/data",
			"--wal-method=fetch",
			fmt.Sprintf("--username=%s", m.replicationUser),
			fmt.Sprintf("--host=%s", masterHost),
			fmt.Sprintf("--port=%s", masterPort),
			"--progress",
			"--verbose")
		if err != nil {
			log.Error("has error when pg_basebackup. ",
				err, stdout, stderr)
		} else {
			log.Info("pg_basebackup success")
		}
	} else {
		log.Info("run pg_rewind success")
		log.Info("write recovery.conf")
		// change recover.conf
		config := fmt.Sprintf(redirectTemplate,
			m.replicationUser, m.replicationPassword, masterHost, masterPort)
		err = ioutil.WriteFile("/var/lib/postgresql/data/recovery.conf",
			[]byte(config), os.ModePerm)
		if err != nil {
			log.Error("has error when write recovery.conf. ", err)
			return errors.Trace(err)
		}
	}
	log.Info("run pg_ctl start")
	stdout, stderr, err = runCommand("pg_ctl", "start", "--log=/var/lib/postgresql/data/pg_ctl.log")
	if err != nil {
		log.Error("has error when pg_ctl start. ",
			err, stdout, stderr)
	} else {
		log.Info("run pg_ctl start success")
	}
	return err
}

func (m *postgresqlServiceManager) SetReadOnlyManually() (bool, error) {
	err := m.SetReadOnly()
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *postgresqlServiceManager) Close() error {
	return postgresql.CloseDB(m.db)
}

func (m *postgresqlServiceManager) LoadMasterStatusFromDB() (*Position, error) {
	lsn, err := postgresql.GetCurrentLSN(m.db)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pos := &Position{}
	pos.EndTxnID = lsn

	return pos, nil
}

func (m *postgresqlServiceManager) LoadSlaveStatusFromDB() (*Position, error) {
	lsn, err := postgresql.GetLastReplayLSN(m.db)
	if err != nil {
		return nil, errors.Trace(err)
	}
	pos := &Position{}
	pos.EndTxnID = lsn
	pos.SlaveIORunning = true
	pos.SlaveSQLRunning = true

	return pos, nil
}

func (m *postgresqlServiceManager) LoadReplicationInfoOfSlave() (masterUUID, executedGTID string, endTxnID uint64, err error) {
	lsn, err := postgresql.GetLastReplayLSN(m.db)
	if err != nil {
		return "", "", 0, errors.Trace(err)
	}
	return "", "", lsn, nil

}

func (m *postgresqlServiceManager) LoadReplicationInfoOfMaster() (masterUUID, executedGTID string, endTxnID uint64, err error) {
	lsn, err := postgresql.GetCurrentLSN(m.db)
	if err != nil {
		return "", "", 0, errors.Trace(err)
	}
	return "", "", lsn, nil
}

func (m *postgresqlServiceManager) GetServerUUID() (string, error) {
	return "", nil
}

// runCommand run the command with user `postgres`
func runCommand(name string, arg ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(name, arg...)

	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{Uid: 999, Gid: 999}
	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	cmd.Stdout = &stdoutBuf
	err = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

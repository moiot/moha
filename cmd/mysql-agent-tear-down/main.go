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

package main

import (
	"os"

	"io/ioutil"

	"syscall"

	"strconv"

	"strings"

	"time"

	"github.com/moiot/moha/agent"

	"github.com/moiot/moha/pkg/log"
)

func main() {
	cfg := agent.NewConfig()
	if err := cfg.Parse(os.Args[1:]); err != nil {
		log.Fatalf("verifying flags error, %v. See '%s --help'", err, agent.DefaultName)
	}
	log.Info("tear-down init logger")
	err := agent.InitLogger(cfg)
	if err != nil {
		log.Error("error when init logger. error: ", err)
	}

	mysqlStopCh := make(chan interface{})
	go func() {
		waitProcessToStop(os.Getenv("MYSQL_PID_FILE"))
		close(mysqlStopCh)
	}()
	log.Info("tear-down begin to wait4 mysql")
	var mysqlStopSuccess bool
	select {
	case <-mysqlStopCh:
		log.Info("detect mysql has been stopped by wait4 or error happens")
		mysqlStopSuccess = true
		break
	case <-time.After(3 * time.Second):
		log.Info("timeout when wait4 mysql")
		break
	}
	if mysqlStopSuccess {
		log.Info("mysql stop success, start clean")
		err = agent.StartClean(cfg)
		if err != nil {
			log.Error("fail to run agent-cleaner err: ", err)
		}
	} else {
		log.Warn("mysql stop timeout, force shutdown and lease remains until ttl expires")
	}

}

func waitProcessToStop(pidFile string) error {
	log.Info("load pid file from ", pidFile)
	bs, err := ioutil.ReadFile(pidFile)
	if err != nil {
		log.Error("error loading mysql pid file, err ", err)
		log.Error("skip mysql waiting")
		return err
	}
	str := strings.TrimRight(string(bs), "\n")
	pid, err := strconv.Atoi(str)
	if err != nil {
		log.Error("error converting pid, ", str, " to int , err ", err)
		log.Error("skip mysql waiting")
		return err
	}
	var wstatus syscall.WaitStatus
	wpid, err := syscall.Wait4(pid, &wstatus, 0, nil)

	if err != nil {
		log.Error("error waiting pid, ", pid, " , err ", err)
		log.Error("skip mysql waiting")
		return err
	}

	log.Info("success waiting pid ", wpid, " exits.")
	return nil
}

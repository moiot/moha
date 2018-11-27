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
	_ "net/http/pprof"
	"os"
	"syscall"

	"git.mobike.io/database/mysql-agent/agent"
	"git.mobike.io/database/mysql-agent/pkg/log"
)

func main() {
	cfg := agent.NewConfig()
	if err := cfg.Parse(os.Args[1:]); err != nil {
		log.Fatalf("verifying flags error, %v. See '%s --help'", err, agent.DefaultName)
	}
	log.Info("service-boot init logger")
	err := agent.InitLogger(cfg)
	if err != nil {
		log.Error("error when init logger. error: ", err)
	}

	procAttr := &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
	}
	filename, args := cfg.ForkProcessFile, cfg.ForkProcessArgs
	cpid, err := syscall.ForkExec(filename, args, procAttr)
	if err != nil {
		log.Error("error while ForkExec , error is ", err)
	}
	log.Info("service-boot forked child pid is ", cpid)
	log.Infof("service-boot process with pid %d exits", syscall.Getpid())
	os.Exit(0)

}

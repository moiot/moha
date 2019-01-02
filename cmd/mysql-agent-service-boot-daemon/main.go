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

	"github.com/moiot/moha/pkg/log"
)

func main() {

	procAttr := &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
	}
	fileNameAndArgs := os.Args[1:]
	if len(fileNameAndArgs) < 2 {
		log.Errorf("args should be of length at least 2. service-boot-daemon exit(1)")
		os.Exit(1)
	}
	log.Info("fileNameAndArgs are ", fileNameAndArgs)
	filename, args := fileNameAndArgs[0], fileNameAndArgs
	cpid, err := syscall.ForkExec(filename, args, procAttr)
	if err != nil {
		log.Error("error while ForkExec , error is ", err)
	}
	log.Info("service-boot-daemon forked child pid is ", cpid)
	log.Infof("service-boot-daemon process with pid %d daemon", syscall.Getpid())

	log.Info("begin to wait4 all cpid")
	var wstatus syscall.WaitStatus
	wpid, err := syscall.Wait4(-1, &wstatus, 0, nil)
	if err != nil {
		log.Error("wait4 all cpid has error.", err)
	} else {
		log.Infof("wait4 all cpid return cpid %d, wstatus %+v .",
			wpid, wstatus)
	}

	log.Info("wait4 done. daemon blocks forever")
	neverReceive := make(chan int)
	<-neverReceive

}

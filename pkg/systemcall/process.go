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

package systemcall

import (
	"fmt"
	"os"
	"runtime"
	"syscall"

	"git.mobike.io/database/mysql-agent/pkg/log"
	"github.com/juju/errors"
)

// DoubleForkAndExecute fork current process twice and runs syscall.Exec
func DoubleForkAndExecute(cfgFile string) error {
	procAttr := &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
	}
	cpid, err := syscall.ForkExec("/agent/mysql-agent-service-boot",
		[]string{"/agent/mysql-agent-service-boot",
			fmt.Sprintf("-config=%s", cfgFile)},
		procAttr)
	wstatus := new(syscall.WaitStatus)
	wpid, err := waitpid(cpid, wstatus, 0)
	log.Infof("Agent process waitpid, pid=%d, waitstatus is %d ,error is %v", wpid, *wstatus, err)
	return errors.Trace(err)
}

// isChildProcess returns true if current process is child else false
// On Darwin:
//  r1 = child pid in both parent and child.
//  r2 = 0 in parent, 1 in child.
// Normal Unix:
//  r1 = 0 in child.
func isChildProcess(r1, r2 uintptr) bool {
	return (runtime.GOOS == "darwin" && r2 == 1) || r1 == 0
}

func waitpid(pid int, wstatus *syscall.WaitStatus, options int) (wpid int, err error) {
	return syscall.Wait4(pid, wstatus, options, nil)
}

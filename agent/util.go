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

package agent

import (
	"os"

	"github.com/juju/errors"

	"net"
	"strings"
	"time"

	"github.com/moiot/moha/pkg/log"
	"github.com/coreos/etcd/clientv3"
)

// checkFileExist checks whether a file is exists and is a file.
func checkFileExist(fpath string) (string, error) {
	fi, err := os.Stat(fpath)
	if err != nil {
		return "", errors.Trace(err)
	}
	if fi.IsDir() {
		return "", errors.Errorf("path: %s, is a directory, not a file", fpath)
	}
	return fpath, nil
}

func parseHost(address string) (h, p string, err error) {

	addr := strings.Split(address, ":")
	if len(addr) > 2 {
		return "", "", errors.NotValidf("leader host is not a valid address %s")
	}
	if len(addr) == 1 {
		h = addr[0]
		p = "3306"
	} else {
		h = addr[0]
		p = addr[1]
	}
	return h, p, nil
}

// isPortAlive returns true if the given port is alive else false
func isPortAlive(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 100*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func waitUtilPortAlive(host, port string, stopSig <-chan interface{}) <-chan interface{} {
	r := make(chan interface{})
	go func() {
		defer close(r)
		for {
			select {
			case <-stopSig:
				return
			default:
				if !isPortAlive(host, port) {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return
			}
		}
	}()
	return r
}

// DoWithRetry will execute f once, followed by `retryTimes` retry if returned err is not nil
// so given DoWithRetry(f, 2), f will be executed at most 3 times, 1 execution and 2 retries, with the last time error returned.
func DoWithRetry(f func() error, funcName string, retryTimes int, retryInterval time.Duration) error {
	var err error
	for i := 0; i <= retryTimes; i++ {
		if err = f(); err == nil {
			return nil
		}
		log.Warn("function has error, retry ", funcName, err)
		time.Sleep(retryInterval)
	}
	return err
}

// Concatenate creates a new array and concatenates all passed-in arrays together
func Concatenate(arrays ...[]clientv3.Op) []clientv3.Op {
	if len(arrays) == 0 {
		return []clientv3.Op{}
	}
	first := arrays[0]
	r := make([]clientv3.Op, len(first))
	copy(r, first)
	for i, element := range arrays {
		if i == 0 {
			continue
		}
		r = append(r, element...)
	}
	return r

}

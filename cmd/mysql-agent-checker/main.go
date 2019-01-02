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
	"os/signal"
	"syscall"

	"github.com/juju/errors"
	"github.com/moiot/moha/checker"
	log "github.com/sirupsen/logrus"
)

func main() {
	cfg := checker.NewConfig()
	if err := cfg.Parse(os.Args[1:]); err != nil {
		log.Fatalf("verifying flags error, %v. See 'checker --help'", err)
	}

	svr, err := checker.NewServer(cfg)
	if err != nil {
		log.Fatalf("create checker daemon error. %v", err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	go func() {
		sig := <-sc
		log.Infof("got signal [%d] to exit.", sig)
		// svr.Close()
		os.Exit(0)
	}()

	if err := svr.Start(); err != nil {
		log.Errorf("checker daemon error, %v", errors.Trace(err))
		os.Exit(2)
	}
}

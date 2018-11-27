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

	"net/http"
	_ "net/http/pprof"

	"git.mobike.io/database/mysql-agent/agent"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"git.mobike.io/database/mysql-agent/pkg/log"
)

func main() {
	cfg := agent.NewConfig()
	if err := cfg.Parse(os.Args[1:]); err != nil {
		log.Fatalf("verifying flags error, %v. See '%s --help'", err, agent.DefaultName)
	}

	agent.InitLogger(cfg)
	agent.PrintVersionInfo()

	// init prometheus
	http.Handle("/metrics", promhttp.Handler())

	svr, err := agent.NewServer(cfg)
	if err != nil {
		log.Errorf("create %s daemon error. %v", agent.DefaultName, err)
		os.Exit(3)
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
		svr.Close()
		log.Info("agent server close done, exit with status 0")
		os.Exit(0)
	}()

	if err := svr.Start(); err != nil {
		log.Errorf("%s daemon error, %v", agent.DefaultName, err)
		os.Exit(2)
	}
}

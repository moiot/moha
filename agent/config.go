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
	"flag"
	"fmt"
	"os"
	"time"

	"git.mobike.io/database/mysql-agent/pkg/mysql"
	"github.com/BurntSushi/toml"
	"github.com/juju/errors"
)

const (
	// DefaultName is the default name of the command line.
	DefaultName = "mysql-agent"

	defaultListenAddr = "127.0.0.1:9527"

	defaultEtcdURLs         = "http://127.0.0.1:2379"
	defaultEtcdDialTimeout  = 5 * time.Second
	defaultRefreshtInterval = 2
)

// Config holds the configuration of agent
type Config struct {
	*flag.FlagSet

	// ClusterName distinguishes one Master-Slaves cluster from another
	ClusterName string `toml:"cluster-name" json:"cluster-name"`

	// LeaderLeaseTTL is the leader lease time to live, measured by second
	LeaderLeaseTTL int64 `toml:"leader-lease-ttl" json:"leader-lease-ttl"`
	// ShutdownThreshold is the time that costs when agent shutdown, measured by second
	ShutdownThreshold int64 `toml:"shutdown-threshold" json:"shutdown-threshold"`

	DataDir    string `toml:"data-dir" json:"data-dir"`
	NodeID     string `toml:"node-id" json:"node-id"`
	ListenAddr string `toml:"addr" json:"addr"`

	// OnlyFollow decides whether current node join in leader campaign
	// if onlyFollow is true, current node will NOT campaign leader
	// else, aka onlyFollow is false, node will participate in leader campaign
	OnlyFollow bool `toml:"only-follow" json:"only-follow"`

	EtcdURLs        string `toml:"etcd-urls" json:"etcd-urls"`
	EtcdDialTimeout time.Duration
	RefreshInterval int    `toml:"refresh-interval" json:"refresh-interval"`
	RegisterTTL     int    `toml:"register-ttl" json:"register-ttl"`
	EtcdRootPath    string `toml:"etcd-root-path" json:"etcd-root-path"`
	EtcdUsername    string `toml:"etcd-username" json:"etcd-username"`
	EtcdPassword    string `toml:"etcd-password" json:"etcd-password"`

	LogLevel    string `toml:"log-level" json:"log-level"`
	LogFile     string `toml:"log-file" json:"log-file"`
	ErrorLog    string `toml:"error-log" json:"error-log"`
	LogMaxSize  int    `toml:"log-max-size" json:"log-max-size"`
	LogMaxDays  int    `toml:"log-max-days" json:"log-max-days"`
	LogCompress bool   `toml:"log-compress" json:"log-compress"`

	DBConfig mysql.DBConfig `toml:"db-config" json:"db-config"`

	InternalServiceHost string `toml:"internal-service-host" json:"internal-service-host"`
	ExternalServiceHost string `toml:"external-service-host" json:"external-service-host"`

	ForkProcessFile       string   `toml:"fork-process-file" json:"fork-process-file"`
	ForkProcessArgs       []string `toml:"fork-process-args" json:"fork-process-args"`
	ForkProcessWaitSecond int      `toml:"fork-process-wait-second" json:"fork-process-wait-second"`

	CampaignWaitTime time.Duration

	fd int

	configFile   string
	printVersion bool
}

// NewConfig return an instance of configuration
func NewConfig() *Config {
	cfg := &Config{
		EtcdDialTimeout:  defaultEtcdDialTimeout,
		CampaignWaitTime: 2 * time.Second,
	}

	cfg.FlagSet = flag.NewFlagSet(DefaultName, flag.ContinueOnError)
	fs := cfg.FlagSet
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", DefaultName)
		fs.PrintDefaults()
	}

	// server related configuration.
	fs.StringVar(&cfg.DataDir, "data-dir", "", "the path to store node id")
	fs.StringVar(&cfg.NodeID, "node-id", "", "the ID of agent node; if not specify, we will generate one from hostname and the listening port")
	fs.StringVar(&cfg.ListenAddr, "addr", defaultListenAddr, "addr(i.e. 'host:port') to listen on the specific port")
	// etcd related configuration.
	fs.StringVar(&cfg.EtcdURLs, "etcd-urls", defaultEtcdURLs, "a comma separated list of the etcd endpoints")
	fs.IntVar(&cfg.RefreshInterval, "refresh-interval", defaultRefreshtInterval, "interval of seconds of refreshing info to etcd ticks")
	fs.IntVar(&cfg.RegisterTTL, "heartbeat-interval", 30, "seconds that registry info is available in etcd")
	// log related configuration.
	fs.StringVar(&cfg.LogLevel, "L", "info", "log level: debug, info, warn, error, fatal")
	fs.StringVar(&cfg.LogFile, "log-file", "", "log file path")
	// misc related configuration.
	fs.StringVar(&cfg.configFile, "config", "", fmt.Sprintf("path to the %s configuration file", DefaultName))
	fs.BoolVar(&cfg.printVersion, "V", false, "print version info")

	fs.IntVar(&cfg.fd, "fd", 1, "file descriptor to write, default stdout")

	return cfg
}

// Parse parse all config from command-line flags, or configuration file.
func (cfg *Config) Parse(arguments []string) error {
	// Parse first to get config file
	perr := cfg.FlagSet.Parse(arguments)
	switch perr {
	case nil:
	case flag.ErrHelp:
		os.Exit(0)
	default:
		os.Exit(2)
	}

	if cfg.printVersion {
		displayVersionInfo()
		os.Exit(0)
	}

	// Load config file if specified.
	if cfg.configFile != "" {
		if err := cfg.configFromFile(cfg.configFile); err != nil {
			return errors.Trace(err)
		}
	}

	// Parse again to replace with command line options.
	cfg.FlagSet.Parse(arguments)
	if len(cfg.FlagSet.Args()) > 0 {
		return errors.Errorf("'%s' is not a valid flag", cfg.FlagSet.Arg(0))
	}

	return nil
}

// configFromFile loads configuration from toml file.
func (cfg *Config) configFromFile(path string) error {
	_, err := toml.DecodeFile(path, cfg)
	return errors.Trace(err)
}

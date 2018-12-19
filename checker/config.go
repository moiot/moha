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

package checker

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
	DefaultName            = "mysql-agent-checker"
	defaultEtcdURLs        = "http://127.0.0.1:2379"
	defaultEtcdDialTimeout = 2 * time.Second
)

// Config is the config of checker
type Config struct {
	*flag.FlagSet

	ClusterName string `toml:"cluster-name" json:"cluster-name"`

	EtcdURLs        string `toml:"etcd-urls" json:"etcd-urls"`
	EtcdDialTimeout time.Duration
	EtcdRootPath    string `toml:"etcd-root-path" json:"etcd-root-path"`
	EtcdUsername    string `toml:"etcd-username" json:"etcd-username"`
	EtcdPassword    string `toml:"etcd-password" json:"etcd-password"`

	MaxCounter int `toml:"max-counter" json:"max-counter"`

	LogLevel  string `toml:"log-level" json:"log-level"`
	LogFile   string `toml:"log-file" json:"log-file"`
	LogRotate string `toml:"log-rotate" json:"log-rotate"`

	DBConfig     mysql.DBConfig `toml:"db-config" json:"db-config"`
	RootUser     string         `toml:"root-user" json:"user"`
	RootPassword string         `toml:"root-password" json:"password"`

	IDContainerMapping map[string]string `toml:"id-container-mapping" json:"id-container-mapping"`
	ContainerAZMapping map[string]string `toml:"container-az-mapping" json:"container-az-mapping"`

	PartitionTemplate string `toml:"partition-template" json:"partition-template"`
	PartitionType     string `toml:"partition-type" json:"partition-type"`

	ChaosJob string

	configFile   string
	printVersion bool
}

// NewConfig return an instance of configuration
func NewConfig() *Config {
	cfg := &Config{
		EtcdDialTimeout: defaultEtcdDialTimeout,
	}

	cfg.FlagSet = flag.NewFlagSet(DefaultName, flag.ContinueOnError)
	fs := cfg.FlagSet
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", DefaultName)
		fs.PrintDefaults()
	}

	// server related configuration.
	// etcd related configuration.
	fs.StringVar(&cfg.EtcdURLs, "etcd-urls", defaultEtcdURLs, "a comma separated list of the etcd endpoints")
	// log related configuration.
	fs.StringVar(&cfg.LogLevel, "L", "info", "log level: debug, info, warn, error, fatal")
	fs.StringVar(&cfg.LogFile, "log-file", "", "log file path")
	fs.StringVar(&cfg.LogRotate, "log-rotate", "", "log file rotate type, hour/day")
	// misc related configuration.
	fs.StringVar(&cfg.ChaosJob, "chaos", "change_master", fmt.Sprintf("the name of the chaos job checker runs"))
	fs.StringVar(&cfg.configFile, "config", "", fmt.Sprintf("path to the %s configuration file", DefaultName))
	fs.BoolVar(&cfg.printVersion, "V", false, "print version info")

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

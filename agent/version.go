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
	"fmt"
	"runtime"

	"github.com/moiot/moha/pkg/log"
)

var (
	// Version defines the version of agent
	Version = "1.0.0+git"

	// GitHash will be set during make
	GitHash = "Not provided (use make build instead of go build)"
	// BuildTS will be set during make
	BuildTS = "Not provided (use make build instead of go build)"
)

// PrintVersionInfo show version info to log
func PrintVersionInfo() {
	log.Infof("%s Version: %s", DefaultName, Version)
	log.Infof("Git Commit Hash: %s", GitHash)
	log.Infof("Build TS: %s", BuildTS)
	log.Infof("Go Version: %s", runtime.Version())
	log.Infof("Go OS/Arch: %s%s", runtime.GOOS, runtime.GOARCH)
}

// DisplayVersionInfo show version info to stdout
func displayVersionInfo() {
	fmt.Printf("%s Version: %s\n", DefaultName, Version)
	fmt.Printf("Git Commit Hash: %s\n", GitHash)
	fmt.Printf("Build TS: %s\n", BuildTS)
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Printf("Go OS/Arch: %s%s\n", runtime.GOOS, runtime.GOARCH)
}

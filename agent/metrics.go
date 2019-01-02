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
	"runtime/debug"

	"github.com/moiot/moha/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
)

var agentHeartbeat = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "moha",
	Subsystem: "agent",
	Name:      "agent_heartbeat",
	Help:      "agent heartbeat, to detect agent is alive, hang or dead",
}, []string{"cluster_name"})

var agentMasterSwitch = prometheus.NewCounterVec(prometheus.CounterOpts{
	Namespace: "moha",
	Subsystem: "agent",
	Name:      "agent_master_switch",
	Help:      "agent master switch, to indicate a master switch happens",
}, []string{"cluster_name"})

var agentIsLeader = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "moha",
	Subsystem: "agent",
	Name:      "agent_is_leader",
	Help:      "agent is leader, to indicate whether current node is the leader",
}, []string{"cluster_name"})

var agentSlaveStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: "moha",
	Subsystem: "agent",
	Name:      "agent_slave_status",
	Help:      "agent slave status",
}, []string{"cluster_name", "type"})

func initMetrics() {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("prometheus register panic. error: %s, stack: %s", err, debug.Stack())
		}
	}()
	prometheus.MustRegister(agentHeartbeat)
	prometheus.MustRegister(agentMasterSwitch)
	prometheus.MustRegister(agentIsLeader)
	prometheus.MustRegister(agentSlaveStatus)
}

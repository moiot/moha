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
	"context"
	"io/ioutil"
	"os"
	"strings"
	"time"

	. "gopkg.in/check.v1"
)

var _ = Suite(&testNodeSuite{})

type testNodeSuite struct{}

func (t *testNodeSuite) TestNodeCreate(c *C) {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "node_test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpDir)

	etcdClient := testServerCluster.RandClient()
	listenAddr := "http://127.0.0.1:39527"

	// test agent node
	cfg := &Config{
		DataDir:         tmpDir,
		EtcdURLs:        strings.Join(etcdClient.Endpoints(), ","),
		EtcdDialTimeout: defaultEtcdDialTimeout,
		ListenAddr:      listenAddr,
		RefreshInterval: 1,
		RegisterTTL:     30,
	}

	node, err := NewAgentNode(cfg)
	c.Assert(node, NotNil)
	c.Assert(err, IsNil)

	an := node.(*agentNode)
	an.getAgentStatus = func() *agentStatus {
		return &agentStatus{Master: true}
	}

	ns := &NodeStatus{
		NodeID:       an.id,
		InternalHost: an.internalHost,
		ExternalHost: an.externalHost,
		IsAlive:      true,
		IsMaster:     true,
	}

	// check register.
	err = node.Register(context.Background())
	c.Assert(err, IsNil)
	mustEqualStatus(c, node.(*agentNode), an.id, ns)

	// check unregister.
	ctx, cancel := context.WithCancel(context.Background())
	err = node.Unregister(ctx)
	c.Assert(err, IsNil)

	// cancel context.
	cancel()
}

func (t *testNodeSuite) TestHeartbeat(c *C) {
	tmpDir, err := ioutil.TempDir(os.TempDir(), "node_test")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpDir)

	etcdClient := testServerCluster.RandClient()
	listenAddr := "http://127.0.0.1:39527"

	// test agent node
	cfg := &Config{
		DataDir:         tmpDir,
		EtcdURLs:        strings.Join(etcdClient.Endpoints(), ","),
		EtcdDialTimeout: defaultEtcdDialTimeout,
		ListenAddr:      listenAddr,
		RefreshInterval: 1,
		RegisterTTL:     30,
	}

	node, err := NewAgentNode(cfg)
	c.Assert(node, NotNil)
	c.Assert(err, IsNil)

	ctx, cancel := context.WithCancel(context.Background())

	// check Heartbeat.
	errCh := node.Heartbeat(ctx)
	c.Assert(errCh, NotNil)

	// cancel context.
	time.Sleep(2 * time.Second)
	cancel()
}

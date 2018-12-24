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
	"time"

	"git.mobike.io/database/mysql-agent/pkg/etcd"
	"git.mobike.io/database/mysql-agent/pkg/log"
	"github.com/juju/errors"
	. "gopkg.in/check.v1"
)

var _ = Suite(&testRegistrySuite{})

type testRegistrySuite struct{}

type RegistryTestClient interface {
	Node(ctx context.Context, nodeID string) (*NodeStatus, error)
}

func (t *testRegistrySuite) TestRegisterNode(c *C) {
	etcdClient := etcd.NewClient(testServerCluster.RandClient(), "mock_cluster_nodes")
	r := NewEtcdRegistry(etcdClient, time.Duration(5)*time.Second)
	// make sure etcd registry is created successfully.
	c.Assert(r, NotNil)

	ns := &NodeStatus{
		NodeID:       "register_node_test",
		InternalHost: "test",
		ExternalHost: "test",
		IsAlive:      true,
	}

	// validate register logic.
	err := r.RegisterNode(context.Background(), ns.NodeID, ns.ExternalHost, ns.InternalHost, 30)
	c.Assert(err, IsNil)
	// check the node status of the registered node works as expected.
	mustEqualStatus(c, r, ns.NodeID, ns)

	// validate unregister logic.
	err = r.UnregisterNode(context.Background(), ns.NodeID)
	c.Assert(err, IsNil)
	_, err = r.Node(context.Background(), ns.NodeID)
	log.Info("TestRegisterNode return error ", err)
	c.Assert(errors.IsNotFound(err), Equals, true)
}

func (t *testRegistrySuite) TestRefreshNode(c *C) {
	etcdClient := etcd.NewClient(testServerCluster.RandClient(), "mock_cluster_nodes")
	r := NewEtcdRegistry(etcdClient, time.Duration(5)*time.Second)

	ns := &NodeStatus{
		NodeID:       "test",
		InternalHost: "test",
		ExternalHost: "test",
	}

	// register node.
	err := r.RegisterNode(context.Background(), ns.NodeID, ns.InternalHost, ns.ExternalHost, 30)
	c.Assert(err, IsNil)

	// refresh node 'alive' child.
	err = r.RefreshNode(context.Background(), ns.NodeID)
	c.Assert(err, IsNil)

	ns.IsAlive = true
	mustEqualStatus(c, r, ns.NodeID, ns)

	// unregister node.
	err = r.UnregisterNode(context.Background(), ns.NodeID)
	c.Assert(err, IsNil)
}

// mustEqualStatus asserts NodeStatus in etcd is equal to status parameter.
func mustEqualStatus(c *C, r RegistryTestClient, nodeID string, status *NodeStatus) {
	nstat, err := r.Node(context.Background(), nodeID)

	c.Assert(nstat, NotNil)
	c.Assert(status, NotNil)

	// ignore LatestPos
	status.LatestPos = nstat.LatestPos
	c.Assert(err, IsNil)
	c.Assert(nstat, DeepEquals, status)
}

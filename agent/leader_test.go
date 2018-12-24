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
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/juju/errors"
	. "gopkg.in/check.v1"
)

var _ = Suite(&testLeaderSuite{})

type testLeaderSuite struct{}

func (t *testAgentServerSuite) TestGetLeader(c *C) {

	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
	}

	_, err := mockClient.Put(ctx, fullLeaderPath, leaderValue)
	c.Assert(err, IsNil)

	leader, _, err := testedServer.getLeaderAndMyTerm()
	c.Assert(err, IsNil)
	c.Assert(leader.name, Equals, leaderValue)

	// mock no leader in etcd
	mockClient.Delete(ctx, fullLeaderPath)

	leader, _, err = testedServer.getLeaderAndMyTerm()
	c.Assert(err, IsNil)
	c.Assert(leader.name, Equals, "")

}

func (t *testAgentServerSuite) TestDeleteLeaderKey(c *C) {

	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
	}

	_, err := mockClient.Put(ctx, fullLeaderPath, leaderValue)
	c.Assert(err, IsNil)

	err = testedServer.deleteLeaderKey()
	c.Assert(err, IsNil)

	// check leader key is actually deleted
	v, _, err := etcdClient.Get(ctx, leaderPath)
	c.Assert(errors.IsNotFound(err), Equals, true)
	c.Assert(v, IsNil)

	// mock leader is not current node
	_, err = mockClient.Put(ctx, fullLeaderPath, "another_unique_id")
	c.Assert(err, IsNil)

	err = testedServer.deleteLeaderKey()
	c.Assert(err, NotNil)

	// mock leader is not set yet
	mockClient.Delete(ctx, fullLeaderPath)
	err = testedServer.deleteLeaderKey()
	c.Assert(err, NotNil)

}

func (t *testAgentServerSuite) TestKeepLeaderAlive(c *C) {
	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
		leaseExpireTS:  time.Now().Add(1 * time.Minute).Unix(),
		db:             mockDB,
	}

	lr, err := mockClient.Grant(ctx, 10)
	c.Assert(err, IsNil)
	_, err = mockClient.Put(ctx, fullLeaderPath, leaderValue, clientv3.WithLease(lr.ID))
	c.Assert(err, IsNil)

	testedServer.leaderStopCh = make(chan interface{})
	go func() {
		testedServer.keepLeaderAliveLoop(int64(lr.ID))
	}()
	time.Sleep(1 * time.Second)
	close(testedServer.leaderStopCh)

}

func (t *testAgentServerSuite) TestResumeLeader(c *C) {

	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
	}
	lr, err := mockClient.Grant(ctx, 10)
	c.Assert(err, IsNil)
	_, err = mockClient.Put(ctx, fullLeaderPath, leaderValue, clientv3.WithLease(lr.ID))
	c.Assert(err, IsNil)

	r, _, err := testedServer.resumeLeader()
	c.Assert(err, IsNil)
	c.Assert(r, Equals, true)

	lr, err = mockClient.Grant(ctx, 10)
	c.Assert(err, IsNil)
	_, err = mockClient.Put(ctx, fullLeaderPath, "not"+leaderValue, clientv3.WithLease(lr.ID))
	c.Assert(err, IsNil)

	r, _, err = testedServer.resumeLeader()
	c.Assert(err, IsNil)
	c.Assert(r, Equals, false)

}

func (t *testAgentServerSuite) TestWatchLeader(c *C) {
	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
		shutdownCh:     make(chan interface{}),
	}
	lr, err := mockClient.Grant(ctx, 10)
	c.Assert(err, IsNil)
	_, err = mockClient.Put(ctx, fullLeaderPath, leaderValue, clientv3.WithLease(lr.ID))
	c.Assert(err, IsNil)

	go func() {
		err := testedServer.watchLeader(0)
		c.Assert(err, IsNil)
	}()

	time.Sleep(1 * time.Second)
	testedServer.Close()

}

func (t *testAgentServerSuite) TestCampaignLeader(c *C) {

	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
	}
	// mock there is no leader
	// leave etcd operation blank
	isLeader, lr, err := testedServer.campaignLeader()
	c.Assert(err, IsNil)
	c.Assert(lr, NotNil)
	c.Assert(isLeader, Equals, true)

	// mock the leader is current node
	mockClient.Delete(ctx, fullLeaderPath)
	mockClient.Put(ctx, fullLeaderPath, leaderValue)
	isLeader, lr, err = testedServer.campaignLeader()
	c.Assert(err, IsNil)
	c.Assert(lr, IsNil)
	c.Assert(isLeader, Equals, false)

	// mock the leader is not current node
	mockClient.Delete(ctx, fullLeaderPath)
	mockClient.Put(ctx, fullLeaderPath, "some random string")
	isLeader, lr, err = testedServer.campaignLeader()
	c.Assert(err, IsNil)
	c.Assert(lr, IsNil)
	c.Assert(isLeader, Equals, false)

	// mock the leader lease is out of date
	mockClient.Delete(ctx, fullLeaderPath)
	isLeader, lr, err = testedServer.campaignLeader()
	c.Assert(err, IsNil)
	c.Assert(lr, NotNil)
	c.Assert(isLeader, Equals, true)

}

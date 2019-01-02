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
	"time"

	"github.com/juju/errors"
	. "gopkg.in/check.v1"
)

func (t *testAgentServerSuite) TestCampaignLeaderAsSinglePoint(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.term = 5

	s.node.RawClient().Delete(ctx, leaderPath, true)

	success, err := s.campaignLeaderAsSinglePoint()
	c.Assert(success, Equals, true)
	c.Assert(err, IsNil)

	leader, _, err := s.getLeaderAndMyTerm()
	c.Assert(leader.meta.term, Equals, uint64(6))
	c.Assert(leader.spm, Equals, s.node.ID())
	c.Assert(leader.name, Equals, s.node.ID())
}

func (t *testAgentServerSuite) TestResumeLeaderAsSinglePoint(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.term = 5

	s.node.RawClient().Delete(ctx, leaderPath, true)
	success, err := s.resumeLeaderAsSinglePoint()
	c.Assert(success, Equals, false)
	c.Assert(err, IsNil)

	s.node.RawClient().Put(ctx, leaderPath, s.node.ID())

	success, err = s.resumeLeaderAsSinglePoint()
	c.Assert(success, Equals, true)
	c.Assert(err, IsNil)
	leader, _, err := s.getLeaderAndMyTerm()
	c.Assert(leader.meta.term, Equals, uint64(5))
	c.Assert(leader.spm, Equals, s.node.ID())
	c.Assert(leader.name, Equals, s.node.ID())
}

func (t *testAgentServerSuite) TestBecomeSinglePointMaster(c *C) {
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)

	s.term = 5

	go s.becomeSinglePointMaster(false)
	time.Sleep(1 * time.Second)

	leader, _, err := s.getLeaderAndMyTerm()
	c.Assert(err, IsNil)
	c.Assert(leader.meta.term, Equals, uint64(6))
	c.Assert(leader.spm, Equals, s.node.ID())
	c.Assert(leader.name, Equals, s.node.ID())

	s.node.Register(ctx)
	// add another node on /slave
	s.node.RawClient().Put(ctx,
		join("slave", "another_id"), "place_holder")

	time.Sleep(2 * time.Duration(mockCfg.RegisterTTL) * time.Second)

	_, _, err = s.node.RawClient().Get(ctx, spmPath)
	c.Assert(errors.IsNotFound(err), Equals, true)

}

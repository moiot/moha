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

	. "gopkg.in/check.v1"
)

var _ = Suite(&testCleanerSuite{})

type testCleanerSuite struct{}

func (t *testCleanerSuite) TestStartClean(c *C) {

	cfg := *mockCfg
	cfg.DataDir = funcName(1)
	cfg.EtcdRootPath = funcName(1)
	s, err := NewServer(&cfg)
	defer os.RemoveAll(cfg.DataDir)
	c.Assert(err, IsNil)
	s.node.RawClient().Put(s.ctx, leaderPath, s.node.ID())

	err = StartClean(&cfg)
	c.Assert(err, IsNil)
	_, _, err = s.node.RawClient().Get(s.ctx, leaderPath)
	c.Assert(err, NotNil)

}

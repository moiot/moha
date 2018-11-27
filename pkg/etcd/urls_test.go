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

package etcd

import (
	"strings"

	. "gopkg.in/check.v1"
)

var _ = Suite(&testURLsSuite{})

type testURLsSuite struct{}

func (t *testURLsSuite) TestURLs(c *C) {
	etcdURLsStrs := []string{
		"http://etcd01:20080",
		"http://192.168.0.1:20080",
		"http://etcd01.mobike.io:20080",
	}
	expectedSetLen := len(etcdURLsStrs)

	uv, err := NewURLsValue(strings.Join(etcdURLsStrs, ","))
	c.Assert(err, IsNil)
	c.Assert(uv, NotNil)

	etcdURLsValueStr := uv.String()
	c.Assert(strings.Split(etcdURLsValueStr, ","), HasLen, expectedSetLen)

	etcdHostStr := uv.HostString()
	c.Assert(strings.Split(etcdHostStr, ","), HasLen, expectedSetLen)

	c.Assert(uv.StringSlice(), HasLen, expectedSetLen)

	c.Assert(uv.URLSlice(), HasLen, expectedSetLen)
}

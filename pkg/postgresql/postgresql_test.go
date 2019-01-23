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

package postgresql_test

import (
	"testing"

	. "gopkg.in/check.v1"
)

type testPostgreSQLSuite struct{}

var _ = Suite(&testPostgreSQLSuite{})

// Hock into "go test" runner.
func TestMySQL(t *testing.T) {
	TestingT(t)
}

func (s *testPostgreSQLSuite) Test_Xxx(c *C) {
}

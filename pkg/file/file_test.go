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

package file

import (
	"io/ioutil"
	"os"
	"testing"

	. "gopkg.in/check.v1"
)

func TestFile(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&testFileSuite{})

type testFileSuite struct{}

func (t *testFileSuite) TestLockAndUnlock(c *C) {
	// create test file
	f, err := ioutil.TempFile("", "lock")
	c.Assert(err, IsNil)
	f.Close()
	defer func() {
		err = os.Remove(f.Name())
		c.Assert(err, IsNil)
	}()

	// try lock the unlocked file
	_, err = TryLockFile(f.Name(), os.O_WRONLY, PrivateFileMode)
	c.Assert(err, IsNil)

	// try to lock the locked file
	_, err = TryLockFile(f.Name(), os.O_WRONLY, PrivateFileMode)
	c.Assert(err, NotNil)

	// try to lock an non-exist file
	_, err = TryLockFile(f.Name()+"nonexist", os.O_WRONLY, PrivateFileMode)
	c.Assert(err, NotNil)

}

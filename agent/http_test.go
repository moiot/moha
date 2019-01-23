package agent

import (
	"net/http"
	"os"

	. "gopkg.in/check.v1"
)

func (t *testAgentServerSuite) TestChangeMaster(c *C) {
	w := NewMockResponseWriter()
	s := newMockServer()
	defer os.RemoveAll(s.cfg.DataDir)
	s.isLeader = 1
	s.leaderStopCh = make(chan interface{})
	s.ChangeMaster(w, nil)
	c.Assert(string(w.content), Equals, "current node is leader and has given up leader successfully\n")

	s.isLeader = 0
	s.ChangeMaster(w, nil)
	c.Assert(string(w.content), Equals, "current node is not leader, change master should be applied on leader\n")
}

func (t *testAgentServerSuite) TestSetReadOnly(c *C) {
	w := NewMockResponseWriter()

	s := newMockServer()
	s.isLeader = 1
	s.leaderStopCh = make(chan interface{})
	defer os.RemoveAll(s.cfg.DataDir)
	s.SetReadOnly(w, nil)
	c.Assert(string(w.content), Equals, "set current node readonly success\n")

	s.isLeader = 0

	s.SetReadOnly(w, nil)
	c.Assert(string(w.content), Equals, "current node is not leader, so no need to set readonly\n")
}

func (t *testAgentServerSuite) TestSetReadWrite(c *C) {
	w := NewMockResponseWriter()
	s := newMockServer()
	s.isLeader = 1
	s.leaderStopCh = make(chan interface{})
	defer os.RemoveAll(s.cfg.DataDir)
	s.SetReadWrite(w, nil)
	c.Assert(string(w.content), Equals, "set current node readwrite success\n")

	s.isLeader = 0

	s.SetReadWrite(w, nil)
	c.Assert(string(w.content), Equals, "current node is not leader, so no need to set readwrite\n")
}

func (t *testAgentServerSuite) TestMasterCheck(c *C) {
	w := NewMockResponseWriter()
	s := newMockServer()
	s.isLeader = 1
	defer os.RemoveAll(s.cfg.DataDir)
	s.MasterCheck(w, nil)
	c.Assert(w.statusCode, Equals, 200)

	s.isLeader = 0
	s.MasterCheck(w, nil)
	c.Assert(w.statusCode, Equals, 418)

}

func (t *testAgentServerSuite) TestSlaveCheck(c *C) {
	s := newMockServer()
	s.isLeader = 1
	defer os.RemoveAll(s.cfg.DataDir)

	w := NewMockResponseWriter()
	s.SlaveCheck(w, nil)
	c.Assert(w.statusCode, Equals, 418)

	w = NewMockResponseWriter()
	s.isLeader = 0
	s.isSinglePointMaster = 0
	s.SlaveCheck(w, nil)
	c.Assert(w.statusCode, Equals, 200)

	w = NewMockResponseWriter()
	s.isLeader = 1
	s.isSinglePointMaster = 1
	s.SlaveCheck(w, nil)
	c.Assert(w.statusCode, Equals, 200)

}

func NewMockResponseWriter() *MockResponseWriter {
	r := &MockResponseWriter{}
	r.header = make(map[string][]string)
	return r
}

type MockResponseWriter struct {
	content    []byte
	header     http.Header
	statusCode int
}

func (m *MockResponseWriter) Header() http.Header {
	return m.header
}
func (m *MockResponseWriter) Write(b []byte) (int, error) {
	m.content = b
	return len(b), nil
}
func (m *MockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

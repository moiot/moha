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
	"encoding/json"

	. "gopkg.in/check.v1"
)

func (t *testAgentServerSuite) TestUploadLogForElectionAsSlave(c *C) {
	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
		term:           5,
	}

	err := testedServer.uploadLogForElectionAsSlave()
	c.Assert(err, IsNil)

	value, _, err := mockNode.RawClient().
		Get(ctx, join(electionPath, "nodes", mockNode.ID()))
	c.Assert(err, IsNil)
	var uploadedLog LogForElection
	err = json.Unmarshal(value, &uploadedLog)
	c.Assert(err, IsNil)
	c.Assert(uploadedLog.Term, Equals, uint64(6))
	c.Assert(uploadedLog.LastUUID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2")
	c.Assert(uploadedLog.LastGTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")

}

func (t *testAgentServerSuite) TestUploadLogForElectionAsFormerMaster(c *C) {

	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
		term:           5,
	}

	err := testedServer.uploadLogForElectionAsFormerMaster()
	c.Assert(err, IsNil)

	value, _, err := mockNode.RawClient().
		Get(ctx, join(electionPath, "nodes", mockNode.ID()))
	c.Assert(err, IsNil)
	var uploadedLog LogForElection
	err = json.Unmarshal(value, &uploadedLog)
	c.Assert(err, IsNil)
	c.Assert(uploadedLog.Term, Equals, uint64(6))
	c.Assert(uploadedLog.LastUUID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2")
	c.Assert(uploadedLog.LastGTID, Equals, "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46")

}

func (t *testAgentServerSuite) TestGetAllLogsForElection(c *C) {
	testedServer := Server{
		ctx:            ctx,
		cfg:            mockCfg,
		node:           mockNode,
		serviceManager: mockServiceManager,
	}

	l1 := LogForElection{
		Term:     2,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
	}
	l1JSON, err := json.Marshal(l1)
	c.Assert(err, IsNil)
	mockNode.RawClient().Put(ctx, join(electionPath, "nodes", "1"), string(l1JSON))

	l2 := LogForElection{
		Term:     3,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
	}
	l2JSON, err := json.Marshal(l2)
	c.Assert(err, IsNil)
	mockNode.RawClient().Put(ctx, join(electionPath, "nodes", "2"), string(l2JSON))

	l3 := LogForElection{
		Term:     5,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-46",
	}
	l3JSON, err := json.Marshal(l3)
	c.Assert(err, IsNil)
	mockNode.RawClient().Put(ctx, join(electionPath, "nodes", "3"), string(l3JSON))

	m, err := testedServer.getAllLogsForElection()
	c.Assert(err, IsNil)
	c.Assert(len(m), Equals, 3)
	assertLogForElectionEqual(m["1"], l1, c)
	assertLogForElectionEqual(m["2"], l2, c)
	assertLogForElectionEqual(m["3"], l3, c)
}

func (t *testAgentServerSuite) TestIsLatestLog(c *C) {
	m := make(map[string]LogForElection)
	m["1"] = LogForElection{
		Term:     5,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-48",
	}

	m["2"] = LogForElection{
		Term:     3,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d3",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d3:1-55",
	}

	m["3"] = LogForElection{
		Term:     5,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-51",
	}

	testedServer := Server{
		node: mockNode,
	}

	m[testedServer.node.ID()] = LogForElection{
		Term:     5,
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-51",
	}
	isLatest, isSingleNode := testedServer.isLatestLog(m)
	c.Assert(isLatest, Equals, true)
	c.Assert(isSingleNode, Equals, false)

	m[testedServer.node.ID()] = LogForElection{
		Term:     5,
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2:1-44",
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d2",
	}
	isLatest, isSingleNode = testedServer.isLatestLog(m)
	c.Assert(isLatest, Equals, false)
	c.Assert(isSingleNode, Equals, false)

	m[testedServer.node.ID()] = LogForElection{
		Term:     3,
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d3:1-55",
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d3",
	}
	isLatest, isSingleNode = testedServer.isLatestLog(m)
	c.Assert(isLatest, Equals, false)
	c.Assert(isSingleNode, Equals, false)

	m[testedServer.node.ID()] = LogForElection{
		Term:     6,
		LastGTID: "85ab69d1-b21f-11e6-9c5e-64006a8978d3:1-55",
		LastUUID: "85ab69d1-b21f-11e6-9c5e-64006a8978d3",
	}
	isLatest, isSingleNode = testedServer.isLatestLog(m)
	c.Assert(isLatest, Equals, true)
	c.Assert(isSingleNode, Equals, true)

}

func assertLogForElectionEqual(obtained, expected LogForElection, c *C) {
	c.Assert(obtained.Term, Equals, expected.Term)
	c.Assert(obtained.LastUUID, Equals, expected.LastUUID)
	c.Assert(obtained.LastGTID, Equals, expected.LastGTID)
}

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
	"sync"

	"github.com/coreos/etcd/clientv3/concurrency"
	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/log"
)

// NewDistributedLockGenerator creates DistributedLockGenerator
func NewDistributedLockGenerator(client *Client, lockPrefix string, lockTTL int) DistributedLockGenerator {

	return DistributedLockGenerator{
		client:     client,
		lockPrefix: client.KeyWithRootPath(lockPrefix),
		lockTTL:    lockTTL,
	}
}

// DistributedLockGenerator implements LockGenerator interface
type DistributedLockGenerator struct {
	client     *Client
	lockPrefix string
	lockTTL    int
}

// NewLocker creates a new distributed locker
func (g *DistributedLockGenerator) NewLocker() (sync.Locker, error) {

	session, err := concurrency.NewSession(g.client.client, concurrency.WithTTL(g.lockTTL))
	if err != nil {
		log.Error("has error when NewSession ", err)
		return nil, errors.Trace(err)
	}
	return concurrency.NewLocker(session, g.lockPrefix), nil
}

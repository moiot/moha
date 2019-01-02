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
	"encoding/json"
	"path"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/etcd"
	"github.com/moiot/moha/pkg/log"
)

const (
	registryPath = "slave"
	refreshPath  = "binlog"
)

// EtcdRegistry wraps the reactions with etcd.
type EtcdRegistry struct {
	client         *etcd.Client
	reqTimeout     time.Duration
	registerStopCh chan interface{}
}

// NewEtcdRegistry returns an EtcdRegistry client.
func NewEtcdRegistry(cli *etcd.Client, reqTimeout time.Duration) *EtcdRegistry {
	return &EtcdRegistry{
		client:         cli,
		reqTimeout:     reqTimeout,
		registerStopCh: make(chan interface{}),
	}
}

// Close closes the etcd client.
func (r *EtcdRegistry) Close() error {
	err := r.client.Close()
	return errors.Trace(err)
}

// RegisterNode registers node in the etcd.
func (r *EtcdRegistry) RegisterNode(pctx context.Context, nodeID, internalHost, externalHost string, ttl int64) error {

	obj := &NodeStatus{
		NodeID:       nodeID,
		InternalHost: internalHost,
		ExternalHost: externalHost,
	}
	objstr, err := json.Marshal(obj)
	if err != nil {
		return errors.Annotatef(err, "error marshal NodeStatus(%v)", obj)
	}

	key := join(registryPath, nodeID)
	err = r.register(pctx, key, string(objstr), ttl)
	if err != nil {
		return errors.Trace(err)
	}
	return nil

}

func (r *EtcdRegistry) register(pctx context.Context, key, value string, ttl int64) (err error) {
	defer func() {
		if err == nil {
			log.Info("register ", key, " to etcd with ttl ", ttl)
		} else {
			log.Error("register ", key, " to etcd with ttl ", ttl, " fail ", err)
		}
	}()

	ctx, cancel := context.WithTimeout(pctx, r.reqTimeout)
	defer cancel()
	lessor := r.client.NewLease()
	lr, err := lessor.Grant(ctx, ttl)
	if err != nil {
		log.Error("error grant register lease. err: ", err)
		return err
	}
	err = r.client.Put(ctx, key, value, clientv3.WithLease(lr.ID))
	if err != nil {
		log.Error("error put register info. err: ", err)
		return err
	}
	return nil
}

// Node returns node status in etcd by nodeID.
func (r *EtcdRegistry) Node(pctx context.Context, nodeID string) (*NodeStatus, error) {
	ctx, cancel := context.WithTimeout(pctx, r.reqTimeout)
	defer cancel()

	reg, ref := join(registryPath, nodeID), join(refreshPath, nodeID)
	resp, err := r.client.MultiGet(ctx, reg, ref)
	if err != nil {
		return nil, errors.Trace(err)
	}

	status := &NodeStatus{}
	lastPos := &Position{}
	var isFound bool
	for key, n := range resp {
		switch key {
		case reg:
			if err := json.Unmarshal(n, &status); err != nil {
				return nil, errors.Annotatef(err, "error unmarshal NodeStatus with nodeID(%s)", nodeID)
			}
			isFound = true
		}
	}
	if !isFound {
		return nil, errors.NotFoundf("cannot found node %s from etcd", nodeID)
	}
	status.IsAlive = true
	status.LatestPos = *lastPos

	return status, nil
}

// UnregisterNode unregisters node from etcd.
func (r *EtcdRegistry) UnregisterNode(pctx context.Context, nodeID string) error {
	ctx, cancel := context.WithTimeout(pctx, r.reqTimeout)
	defer cancel()

	close(r.registerStopCh)
	err := r.client.Delete(ctx, join(registryPath, nodeID), true)
	return errors.Trace(err)
}

// RefreshNode keeps heartbeats with etcd.
func (r *EtcdRegistry) RefreshNode(pctx context.Context, nodeID string) error {
	ctx, cancel := context.WithTimeout(pctx, r.reqTimeout)
	defer cancel()

	refreshKey := join(refreshPath, nodeID)
	latestPosJSONBytes, err := json.Marshal(latestPos)
	if err != nil {
		return errors.Trace(err)
	}

	// try to touch alive state of node, update ttl
	err = r.client.Put(ctx, refreshKey, string(latestPosJSONBytes))
	return errors.Trace(err)
}

// prefixed joins paths.
func join(p ...string) string {
	return path.Join(p...)
}

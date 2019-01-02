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
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/etcd"
	"github.com/moiot/moha/pkg/file"
	"github.com/moiot/moha/pkg/log"
)

const (
	nodeIDFile = ".node"
	lockFile   = ".lock"
)

// Node defines actions with etcd.
type Node interface {
	// ID returns node ID
	ID() string
	// Register registers node to etcd.
	Register(ctx context.Context) error
	// Unregister unregisters node from etcd.
	Unregister(ctx context.Context) error
	// Heartbeat refreshes node state in etcd.
	// key 'root/nodes/<nodeID/alive' will disappear after TTL time passed.
	Heartbeat(ctx context.Context) <-chan error
	// RowClient returns raw etcd client.
	RawClient() *etcd.Client
	// NodeStatus return one node status
	NodeStatus(ctx context.Context, nodeID string) (*NodeStatus, error)
}

// Position describes the position of MySQL binlog.
type Position struct {
	File string
	Pos  string
	GTID string

	UUID string

	EndTxnID uint64 `json:",omitempty"`

	SlaveIORunning      bool `json:",omitempty"`
	SlaveSQLRunning     bool `json:",omitempty"`
	SecondsBehindMaster int  `json:",omitempty"`
}

// NodeStatus describes the status information of a node in etcd.
type NodeStatus struct {
	NodeID       string
	InternalHost string
	ExternalHost string
	IsAlive      bool     `json:",omitempty"`
	LatestPos    Position `json:",omitempty"`
}

type agentNode struct {
	*EtcdRegistry
	id              string
	internalHost    string
	externalHost    string
	registerTTL     int64
	refreshInterval time.Duration
}

// NewAgentNode returns a MySQL agentNode that monitor MySQL server.
func NewAgentNode(cfg *Config) (Node, error) {
	if err := checkExclusive(cfg.DataDir); err != nil {
		return nil, errors.Trace(err)
	}

	uv, err := etcd.NewURLsValue(cfg.EtcdURLs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cli, err := etcd.NewClientFromCfg(uv.StringSlice(), cfg.EtcdDialTimeout,
		cfg.EtcdRootPath, cfg.ClusterName, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// retrieve nodeID.
	nodeID, err := readLocalNodeID(cfg.DataDir)
	if err != nil {
		if cfg.NodeID != "" {
			nodeID = cfg.NodeID
		} else if errors.IsNotFound(err) {
			nodeID, err = generateLocalNodeID(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else {
			return nil, errors.Trace(err)
		}
	} else if cfg.NodeID != "" {
		log.Warning()
	}

	node := &agentNode{
		EtcdRegistry:    NewEtcdRegistry(cli, cfg.EtcdDialTimeout),
		id:              nodeID,
		internalHost:    cfg.InternalServiceHost,
		externalHost:    cfg.ExternalServiceHost,
		refreshInterval: time.Duration(cfg.RefreshInterval) * time.Second,
		registerTTL:     int64(cfg.RegisterTTL),
	}
	return node, nil
}

func (n *agentNode) Register(ctx context.Context) error {
	err := n.RegisterNode(ctx, n.id, n.internalHost, n.externalHost, n.registerTTL)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (n *agentNode) Unregister(ctx context.Context) error {
	return errors.Trace(n.UnregisterNode(ctx, n.id))
}

func (n *agentNode) Heartbeat(ctx context.Context) <-chan error {
	errc := make(chan error, 1)

	go func() {
		defer func() {
			if err := n.Close(); err != nil {
				errc <- errors.Trace(err)
			}
			close(errc)
			log.Info("heartbeat goroutine exited")
		}()

		ticker := time.NewTicker(time.Duration(n.registerTTL/3+1) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := n.Register(ctx); err != nil {
					errc <- errors.Trace(err)
				}
			case <-n.registerStopCh:
				log.Info("receive from registerStopCh so register node loop stops")
				return
			case <-ctx.Done():
				log.Info("pctx is done so Heartbeat node loop stops")
				return
			}
		}
	}()

	return errc
}

func (n *agentNode) RawClient() *etcd.Client {
	return n.client
}

func (n *agentNode) ID() string {
	return n.id
}

func (n *agentNode) NodeStatus(ctx context.Context, nodeID string) (*NodeStatus, error) {
	return n.Node(ctx, nodeID)
}

// checkExclusive tries to get file lock of dataDir in exclusive mode
// if get lock fails, other MySQL agent may run.
func checkExclusive(dataDir string) error {
	err := os.MkdirAll(dataDir, file.PrivateDirMode)
	if err != nil {
		return errors.Trace(err)
	}
	lockPath := filepath.Join(dataDir, lockFile)
	// when the process exits, the lock file will be closed by system
	// and automatically release the lock
	_, err = file.TryLockFile(lockPath, os.O_WRONLY|os.O_CREATE, file.PrivateFileMode)
	return errors.Trace(err)
}

// readLocalNodeID reads nodeID from a local file
// returns a NotFound error if the nodeID file does not exist.
func readLocalNodeID(dataDir string) (string, error) {
	nodeIDPath := filepath.Join(dataDir, nodeIDFile)
	if _, err := checkFileExist(nodeIDPath); err != nil {
		return "", errors.NewNotFound(err, "local nodeID file not exist")
	}
	data, err := ioutil.ReadFile(nodeIDPath)
	if err != nil {
		return "", errors.Annotate(err, "local nodeID file is wrong")
	}

	return string(data), nil
}

func generateLocalNodeID(cfg *Config) (string, error) {
	dataDir := cfg.DataDir
	if err := os.MkdirAll(dataDir, file.PrivateDirMode); err != nil {
		return "", errors.Trace(err)
	}

	id := cfg.ExternalServiceHost
	nodeIDPath := filepath.Join(dataDir, nodeIDFile)
	if err := ioutil.WriteFile(nodeIDPath, []byte(id), file.PrivateFileMode); err != nil {
		return "", errors.Trace(err)
	}
	return id, nil
}

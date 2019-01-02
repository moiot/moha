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

	"github.com/moiot/moha/pkg/etcd"
	"github.com/moiot/moha/pkg/log"
	"github.com/coreos/etcd/clientv3"
	"github.com/juju/errors"
)

// StartClean should be invoked by another process when agent exits with status code 3,
// is used to clean the data that should have been cleaned by agent gracefully shutdown.
func StartClean(cfg *Config) error {

	// init etcd client
	uv, err := etcd.NewURLsValue(cfg.EtcdURLs)
	if err != nil {
		log.Error("init etcd client err; ", err)
		return errors.Trace(err)
	}
	cli, err := etcd.NewClientFromCfg(uv.StringSlice(), cfg.EtcdDialTimeout,
		cfg.EtcdRootPath, cfg.ClusterName, cfg.EtcdUsername, cfg.EtcdPassword)
	if err != nil {
		log.Error("init etcd client err; ", err)
		return errors.Trace(err)
	}
	defer cli.Close()

	// delete leader key if exists
	nodeID, err := readLocalNodeID(cfg.DataDir)
	if err != nil {
		log.Error("fail to get local node id. err; ", err)
		return errors.Trace(err)
	}

	ctx := context.TODO()
	resp, err := cli.Txn(ctx).
		If(clientv3.Compare(
			clientv3.Value(cli.KeyWithRootPath(leaderPath)),
			"=",
			nodeID)).
		Then(clientv3.OpDelete(cli.KeyWithRootPath(leaderPath))).
		Commit()
	if err != nil {
		return errors.Trace(err)
	}
	if !resp.Succeeded {
		return errors.New("resign leader failed, we are not leader already")
	}
	return nil

}

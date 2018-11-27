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
	"context"
	"testing"
	"time"

	"fmt"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/integration"
	"github.com/juju/errors"
	. "gopkg.in/check.v1"
)

var (
	etcdMockCluster *integration.ClusterV3
	etcdCli         *Client
	ctx             context.Context
)

// Hock into "go test" runner.
func TestEtcd(t *testing.T) {
	ctx, etcdCli, etcdMockCluster = testSetup(t)
	defer etcdMockCluster.Terminate(t)

	TestingT(t)
}

var _ = Suite(&testEtcdSuite{})

type testEtcdSuite struct{}

func (t *testEtcdSuite) TestCreate(c *C) {
	// mock another etcd client
	etcdClient := etcdMockCluster.RandClient()
	kv := clientv3.NewKV(etcdClient)

	key := "mysql_root/new_key"
	obj := "test_val"

	// verify the kv pair is empty before set
	getResp, err := kv.Get(ctx, key)
	c.Assert(err, IsNil)
	c.Assert(getResp.Kvs, HasLen, 0)

	// create a new kv pair
	err = etcdCli.Put(ctx, key, obj)
	c.Assert(err, IsNil)

	// verify the kv pair is exists
	getResp, err = kv.Get(ctx, etcdCli.KeyWithRootPath(key))
	c.Assert(err, IsNil)
	c.Assert(getResp.Kvs, HasLen, 1)
}

func (t *testEtcdSuite) TestCreateWithTTL(c *C) {
	key := "mysql_root/ttl_key"
	obj := "ttl_value"

	// create a new lease
	lcr, err := etcdCli.client.Grant(ctx, 1)
	c.Assert(err, IsNil)
	opts := []clientv3.OpOption{clientv3.WithLease(clientv3.LeaseID(lcr.ID))}

	// create a new kv pair with lease
	err = etcdCli.Put(ctx, key, obj, opts...)
	c.Assert(err, IsNil)

	time.Sleep(2 * time.Second)

	// verify the kv pair is gone if lease expired
	_, _, err = etcdCli.Get(ctx, key)
	c.Assert(errors.IsNotFound(err), Equals, true)
}

func (t *testEtcdSuite) TestMultiGet(c *C) {

	key1 := "mysql_root/multi_get_1"
	key2 := "mysql_root/multi_get_2"
	key3 := "mysql_root/multi_get_3"
	key4 := "mysql_root/multi_get_4"
	key5 := "mysql_root/multi_get_5"
	obj := "test_exist"

	var err error
	err = etcdCli.Put(ctx, key1, obj)
	err = etcdCli.Put(ctx, key2, obj)
	err = etcdCli.Put(ctx, key3, obj)
	err = etcdCli.Put(ctx, key4, obj)
	err = etcdCli.Put(ctx, key5, obj)
	c.Assert(err, IsNil)

	values, err := etcdCli.MultiGet(ctx, key1, key2, key3, key4, key5)
	c.Assert(err, IsNil)
	c.Assert(len(values), Equals, 5)
	for k, v := range values {
		c.Log(k, ":", v)
	}

	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("mysql_root/multi_get_%d", i)
		c.Assert(string(values[key]), Equals, obj)
	}
}

func (t *testEtcdSuite) TestCreateWithKeyExist(c *C) {
	// mock another etcd client
	etcdClient := etcdMockCluster.RandClient()
	kv := clientv3.NewKV(etcdClient)

	key := "mysql_root/exist"
	obj := "test_exist"

	// create a new kv pair
	_, err := kv.Put(ctx, key, obj, nil...)
	c.Assert(err, IsNil)

	// verify the kv pair is already exists
	err = etcdCli.Put(ctx, key, obj)
	c.Assert(err, IsNil)
}

func (t *testEtcdSuite) TestUpdate(c *C) {
	key := "mysql_root/update"
	obj1 := "update_1"
	obj2 := "update_2"

	// create a new lease
	lcr, err := etcdCli.client.Grant(ctx, 2)
	c.Assert(err, IsNil)
	opts := []clientv3.OpOption{clientv3.WithLease(lcr.ID)}

	// create a kv pair with lease
	err = etcdCli.Put(ctx, key, obj1, opts...)
	c.Assert(err, IsNil)

	time.Sleep(time.Second)

	// update the kv pair and renew the lease at the same time
	err = etcdCli.Put(ctx, key, obj2)
	c.Assert(err, IsNil)

	time.Sleep(3 * time.Second)

	// verify kv pair exists
	res, _, err := etcdCli.Get(ctx, key)
	c.Assert(err, IsNil)
	c.Assert(string(res), Equals, obj2)
}

func (t *testEtcdSuite) TestUpdateOrCreate(c *C) {
	key := "mysql_root/update_key"
	obj := "update_or_create"

	err := etcdCli.Put(ctx, key, obj)
	c.Assert(err, IsNil)
}

func (t *testEtcdSuite) TestDelete(c *C) {
	key := "mysql_root/test_key"
	err := etcdCli.Put(ctx, key, key)
	c.Assert(err, IsNil)

	err = etcdCli.Delete(ctx, key, true)
	c.Assert(err, IsNil)
}

func (t *testEtcdSuite) TestList(c *C) {
	key := "mysql_root/test_key"

	k1 := key + "/level1"
	k2 := key + "/level2"
	k3 := key + "/level3"
	k11 := k1 + "/level11"

	for _, k := range []string{k1, k2, k3, k11} {
		err := etcdCli.Put(ctx, k, k)
		c.Assert(err, IsNil)
	}

	root, err := etcdCli.List(ctx, key)
	c.Assert(err, IsNil)
	c.Assert(string(root.Children["level1"].Value), Equals, k1)
	c.Assert(string(root.Children["level1"].Children["level11"].Value), Equals, k11)
	c.Assert(string(root.Children["level2"].Value), Equals, k2)
	c.Assert(string(root.Children["level3"].Value), Equals, k3)
}

// mock cluster and client of etcd.
func testSetup(t *testing.T) (context.Context, *Client, *integration.ClusterV3) {
	cluster := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	etcd := NewClient(cluster.RandClient(), "etcd_test")
	return context.Background(), etcd, cluster
}

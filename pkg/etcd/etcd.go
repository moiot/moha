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
	"path"
	"strings"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/mvcc/mvccpb"
	"github.com/juju/errors"
)

const (
	// DefaultRootPath is the root path of the keys stored in etcd.
	DefaultRootPath = "mysql/mysql-servers"
)

// Node contains etcd query result as a Trie tree.
type Node struct {
	Value    []byte
	Children map[string]*Node
}

// Client is a wrapper of etcd client that support some simple method.
type Client struct {
	client   *clientv3.Client
	rootPath string
}

// NewClient returns a wrapped etcd client.
func NewClient(cli *clientv3.Client, root string) *Client {
	return &Client{
		client:   cli,
		rootPath: root,
	}
}

// NewClientFromCfg returns a customized wrapped etcd client.
func NewClientFromCfg(endpoints []string,
	dialTimeout time.Duration,
	root string, clusterName, username, password string) (*Client, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:            endpoints,
		DialTimeout:          dialTimeout,
		DialKeepAliveTime:    1 * time.Second,
		DialKeepAliveTimeout: 1 * time.Second,
		Username:             username,
		Password:             password,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &Client{
		client:   cli,
		rootPath: keyWithPrefix(root, clusterName),
	}, nil
}

// Close shutdown the connection to etcd.
func (e *Client) Close() error {
	if err := e.client.Close(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Put guarantees to set a `key = value` with options.
func (e *Client) Put(ctx context.Context, k string, val string, opts ...clientv3.OpOption) error {
	key := e.KeyWithRootPath(k)

	kv := clientv3.NewKV(e.client)
	_, err := kv.Put(ctx, key, val, opts...)
	return errors.Trace(err)
}

// Get returns a key/value matches the given key.
// returns the value, leaderID and error respectively
func (e *Client) Get(ctx context.Context, key string) ([]byte, *mvccpb.KeyValue, error) {
	key = e.KeyWithRootPath(key)

	kv := clientv3.NewKV(e.client)

	resp, err := kv.Get(ctx, key)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	if len(resp.Kvs) == 0 {
		return nil, nil, errors.NotFoundf("key %s in etcd", key)
	}

	return resp.Kvs[0].Value, resp.Kvs[0], nil
}

// PrefixGet returns the map struct that contains key/value with same prefix.
func (e *Client) PrefixGet(ctx context.Context, prefix string) (map[string][]byte, error) {
	key := e.KeyWithRootPath(prefix)
	kv := clientv3.NewKV(e.client)
	resp, err := kv.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := make(map[string][]byte)
	for _, kv := range resp.Kvs {
		prefix := e.rootPath
		if !strings.HasSuffix(prefix, "/") {
			prefix = prefix + "/"
		}
		k := strings.TrimPrefix(string(kv.Key), prefix)
		r[k] = kv.Value
	}
	return r, nil
}

// MultiGet returns the map struct that contains key/value within the given keys.
func (e *Client) MultiGet(ctx context.Context, keys ...string) (map[string][]byte, error) {

	txn := e.client.Txn(ctx)
	ops := make([]clientv3.Op, len(keys))
	for i, k := range keys {
		ops[i] = clientv3.OpGet(e.KeyWithRootPath(k))
	}
	txn.Then(ops...)
	resp, err := txn.Commit()
	if err != nil {
		return nil, errors.Trace(err)
	}
	values := make(map[string][]byte, len(resp.Responses))
	for _, r := range resp.Responses {
		if len(r.GetResponseRange().Kvs) == 0 {
			continue
		}
		k := string(r.GetResponseRange().Kvs[0].Key)
		prefix := e.rootPath
		if !strings.HasSuffix(prefix, "/") {
			prefix = prefix + "/"
		}
		k = strings.TrimPrefix(k, prefix)
		values[k] = r.GetResponseRange().Kvs[0].Value
	}
	return values, nil
}

// Delete deletes the `key/values` with matching prefix or key.
func (e *Client) Delete(ctx context.Context, key string, withPrefixMatch bool) error {
	key = e.KeyWithRootPath(key)

	var opts []clientv3.OpOption
	if withPrefixMatch {
		opts = []clientv3.OpOption{clientv3.WithPrefix()}
	}

	kv := clientv3.NewKV(e.client)

	_, err := kv.Delete(ctx, key, opts...)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// List returns the trie struct that contains key/value with same prefix.
func (e *Client) List(ctx context.Context, key string) (*Node, error) {
	// ensure key has '/' suffix
	key = e.KeyWithRootPath(key)
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}

	kv := clientv3.NewKV(e.client)

	resp, err := kv.Get(ctx, key, clientv3.WithPrefix())
	if err != nil {
		return nil, errors.Trace(err)
	}

	root := new(Node)
	length := len(key)
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		// ignore bad key.
		if len(key) <= length {
			continue
		}

		keySuffix := key[length:]
		leafNode := parseToDirTree(root, keySuffix)
		leafNode.Value = kv.Value
	}

	return root, nil
}

func parseToDirTree(root *Node, path string) *Node {
	pathDirs := strings.Split(path, "/")

	current := root
	var next *Node
	var ok bool

	for _, dir := range pathDirs {
		if current.Children == nil {
			current.Children = make(map[string]*Node)
		}

		next, ok = current.Children[dir]
		if !ok {
			current.Children[dir] = new(Node)
			next = current.Children[dir]
		}

		current = next
	}

	return current
}

// Txn returns an etcd transaction
func (e *Client) Txn(ctx context.Context) clientv3.Txn {
	return e.client.Txn(ctx)
}

// NewWatcher creates a new watcher
func (e *Client) NewWatcher() clientv3.Watcher {
	return clientv3.NewWatcher(e.client)
}

// NewLease creates a new lessor
func (e *Client) NewLease() clientv3.Lease {
	return clientv3.NewLease(e.client)
}

// KeyWithRootPath returns the full path of the key on etcd.
// when clientv3 api is used, all keys must be wrapped with this method
// for example, clientv3.Watch(ctx, client.KeyWithFullPath(key))
func (e *Client) KeyWithRootPath(key string) string {
	return keyWithPrefix(e.rootPath, key)
}

func keyWithPrefix(prefix, key string) string {
	return path.Join(prefix, key)
}

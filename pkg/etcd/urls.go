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
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/moiot/moha/pkg/types"
)

// URLsValue is an alias of URLs.
type URLsValue types.URLs

// NewURLsValue returns a URLsValue from a string of URL list(separated by comma).
// e.g. http://192.168.0.2:2380, http://192.168.0.3:2380
func NewURLsValue(s string) (*URLsValue, error) {
	v := &URLsValue{}
	err := v.Set(s)
	return v, err
}

// Set parses a string line of URL
func (uv *URLsValue) Set(s string) error {
	us := strings.Split(s, ",")
	nus, err := types.NewURLs(us)
	if err != nil {
		return errors.Trace(err)
	}

	*uv = URLsValue(nus)
	return nil
}

// String returns a string of etcd URLs separated by comma.
func (uv *URLsValue) String() string {
	rs := make([]string, len(*uv))
	for i, u := range *uv {
		rs[i] = u.String()
	}
	return strings.Join(rs, ",")
}

// HostString returns a string of 'host:port' list separated by comma.
func (uv *URLsValue) HostString() string {
	rs := make([]string, len(*uv))
	for i, u := range *uv {
		rs[i] = u.Host
	}
	return strings.Join(rs, ",")
}

// StringSlice returns a slice of string with formatted URL.
func (uv *URLsValue) StringSlice() []string {
	rs := make([]string, len(*uv))
	for i, u := range *uv {
		rs[i] = u.String()
	}
	return rs
}

// URLSlice returns a slice of URLs.
func (uv *URLsValue) URLSlice() []url.URL {
	rs := []url.URL(*uv)
	return rs
}

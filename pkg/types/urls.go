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

package types

import (
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/juju/errors"
)

// URLs defines a wrapper slice of URLs.
type URLs []url.URL

// NewURLs returns a URLs using a slice of formatted URL string.
func NewURLs(strs []string) (URLs, error) {
	all := make([]url.URL, len(strs))
	if len(all) == 0 {
		return nil, errors.New("invalid URLs")
	}
	for i, str := range strs {
		str = strings.TrimSpace(str)
		u, err := url.Parse(str)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "unix" && u.Scheme != "unixs" {
			return nil, errors.Errorf("URL scheme must be http, https, unix, unixs: %s", str)
		}
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			return nil, errors.Errorf("URL address does not use form 'host:port': %s", str)
		}
		if u.Path != "" {
			return nil, errors.Errorf("URL can not contain a path: %s", str)
		}
		all[i] = *u
	}
	us := URLs(all)
	us.Sort()

	return us, nil
}

// String returns a string of URLs separated by comma.
func (us URLs) String() string {
	return strings.Join(us.StringSlice(), ",")
}

// Sort sorts URLs.
func (us *URLs) Sort() {
	sort.Sort(us)
}

// Len returns the length of URLs.
func (us URLs) Len() int { return len(us) }

// Less compares two URL and return the less one.
func (us URLs) Less(i, j int) bool { return us[i].String() < us[j].String() }

// Swap swaps two URLs item.
func (us URLs) Swap(i, j int) { us[i], us[j] = us[j], us[i] }

// StringSlice returns a slice of formatted URL string.
func (us URLs) StringSlice() []string {
	out := make([]string, len(us))
	for i, v := range us {
		out[i] = v.String()
	}
	return out
}

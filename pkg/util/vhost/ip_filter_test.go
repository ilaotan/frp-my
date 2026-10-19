// Copyright 2026 The frp Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vhost

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewIPAllowList(t *testing.T) {
	t.Run("empty list allows everything", func(t *testing.T) {
		list, err := NewIPAllowList(nil)
		require.NoError(t, err)
		require.True(t, list.Empty())
		require.True(t, list.Contains(netip.MustParseAddr("1.2.3.4")))
		require.True(t, list.ContainsRemoteAddr("1.2.3.4:5678"))
	})

	t.Run("blank entries are ignored", func(t *testing.T) {
		list, err := NewIPAllowList([]string{"  ", ""})
		require.NoError(t, err)
		require.True(t, list.Empty())
	})

	t.Run("rejects malformed entries", func(t *testing.T) {
		for _, entry := range []string{"1.2.3", "not-an-ip", "10.0.0.0/33", "2001:db8::/129"} {
			_, err := NewIPAllowList([]string{entry})
			require.Error(t, err, entry)
		}
	})
}

func TestIPAllowListContains(t *testing.T) {
	list, err := NewIPAllowList([]string{"192.168.1.10", "10.0.0.0/8", "2001:db8::/32"})
	require.NoError(t, err)
	require.Equal(t, 3, list.Len())

	cases := []struct {
		addr  string
		allow bool
	}{
		{"192.168.1.10", true},
		{"192.168.1.11", false},
		{"10.1.2.3", true},
		{"11.1.2.3", false},
		{"2001:db8::1", true},
		{"2001:db9::1", false},
		{"::ffff:192.168.1.10", true},
	}
	for _, tc := range cases {
		require.Equal(t, tc.allow, list.Contains(netip.MustParseAddr(tc.addr)), tc.addr)
	}
}

func TestIPAllowListContainsRemoteAddr(t *testing.T) {
	list, err := NewIPAllowList([]string{"10.0.0.0/8", "::1"})
	require.NoError(t, err)

	require.True(t, list.ContainsRemoteAddr("10.1.2.3:5000"))
	require.True(t, list.ContainsRemoteAddr("[::1]:5000"))
	require.False(t, list.ContainsRemoteAddr("8.8.8.8:5000"))

	// Fail closed when the remote address can't be parsed.
	require.False(t, list.ContainsRemoteAddr(""))
	require.False(t, list.ContainsRemoteAddr("garbage"))
}

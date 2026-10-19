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
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// IPAllowList is an immutable matcher for a set of IP addresses and CIDR prefixes.
//
// A nil or empty list means "no restriction", which keeps the behavior of
// proxies that don't configure an allow list.
type IPAllowList struct {
	prefixes []netip.Prefix
}

// NewIPAllowList parses entries such as "192.168.1.10", "10.0.0.0/8" or "2001:db8::/32".
// It returns a nil list when no entry is configured.
func NewIPAllowList(entries []string) (*IPAllowList, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	list := &IPAllowList{prefixes: make([]netip.Prefix, 0, len(entries))}
	for _, entry := range entries {
		raw := strings.TrimSpace(entry)
		if raw == "" {
			continue
		}
		prefix, err := parseIPPrefix(raw)
		if err != nil {
			return nil, err
		}
		list.prefixes = append(list.prefixes, prefix)
	}
	if len(list.prefixes) == 0 {
		return nil, nil
	}
	return list, nil
}

func parseIPPrefix(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("invalid CIDR %q: %w", raw, err)
		}
		return prefix.Masked(), nil
	}

	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid IP %q: %w", raw, err)
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// Empty reports whether the list imposes no restriction.
func (l *IPAllowList) Empty() bool {
	return l == nil || len(l.prefixes) == 0
}

// Len returns the number of configured entries.
func (l *IPAllowList) Len() int {
	if l == nil {
		return 0
	}
	return len(l.prefixes)
}

// Contains reports whether ip is allowed by the list.
func (l *IPAllowList) Contains(ip netip.Addr) bool {
	if l.Empty() {
		return true
	}
	if !ip.IsValid() {
		return false
	}

	ip = ip.Unmap()
	for _, prefix := range l.prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// ContainsRemoteAddr reports whether the host part of a "host:port" address is allowed.
// Unparsable addresses are rejected once a list is configured, so a malformed
// remote address can never bypass the restriction.
func (l *IPAllowList) ContainsRemoteAddr(remoteAddr string) bool {
	if l.Empty() {
		return true
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}
	return l.Contains(ip)
}

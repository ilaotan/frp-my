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

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func TestValidateDomainConfigForServerRejectsSubdomainHostCaseInsensitively(t *testing.T) {
	tests := []struct {
		name          string
		subDomainHost string
		customDomain  string
		wantErr       bool
	}{
		{
			name:          "lowercase subdomain",
			subDomainHost: "frp.example.com",
			customDomain:  "victim.frp.example.com",
			wantErr:       true,
		},
		{
			name:          "mixed case custom domain",
			subDomainHost: "frp.example.com",
			customDomain:  "victim.FRP.example.com",
			wantErr:       true,
		},
		{
			name:          "mixed case wildcard domain",
			subDomainHost: "frp.example.com",
			customDomain:  "*.FRP.example.com",
			wantErr:       true,
		},
		{
			name:          "mixed case subdomain host",
			subDomainHost: "FRP.Example.Com",
			customDomain:  "victim.frp.example.com",
			wantErr:       true,
		},
		{
			name:          "external domain",
			subDomainHost: "frp.example.com",
			customDomain:  "victim.example.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDomainConfigForServer(
				&v1.DomainConfig{CustomDomains: []string{tt.customDomain}},
				&v1.ServerConfig{SubDomainHost: tt.subDomainHost},
			)
			if tt.wantErr {
				require.ErrorContains(t, err, "should not belong to subdomain host")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateHTTPAuthPagePath(t *testing.T) {
	tests := []struct {
		name     string
		authHdr  string
		pagePath string
		wantErr  bool
	}{
		{name: "empty path is fine", authHdr: "X-FRP-Auth"},
		{name: "custom path", authHdr: "X-FRP-Auth", pagePath: "/_s3cr3t/entry"},
		{name: "trailing slash", authHdr: "X-FRP-Auth", pagePath: "/_s3cr3t/entry/"},
		{name: "path without auth header", pagePath: "/_s3cr3t/entry", wantErr: true},
		{name: "relative path", authHdr: "X-FRP-Auth", pagePath: "entry/login", wantErr: true},
		{name: "root path", authHdr: "X-FRP-Auth", pagePath: "/", wantErr: true},
		{name: "query string", authHdr: "X-FRP-Auth", pagePath: "/entry?x=1", wantErr: true},
		{name: "dot segments", authHdr: "X-FRP-Auth", pagePath: "/entry/../admin", wantErr: true},
		{name: "double slash", authHdr: "X-FRP-Auth", pagePath: "/entry//login", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTTPAuthPagePath(&v1.HTTPProxyConfig{
				HTTPAuthHeader:   tc.authHdr,
				HTTPAuthPagePath: tc.pagePath,
			})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateHTTPProxyAuthFields(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *v1.HTTPProxyConfig
		wantErr bool
	}{
		{
			name: "no auth fields",
			cfg:  &v1.HTTPProxyConfig{},
		},
		{
			name: "header with credentials",
			cfg: &v1.HTTPProxyConfig{
				HTTPUser:       "alice",
				HTTPPassword:   "secret",
				HTTPAuthHeader: "X-FRP-Auth",
			},
		},
		{
			name:    "header without credentials",
			cfg:     &v1.HTTPProxyConfig{HTTPAuthHeader: "X-FRP-Auth"},
			wantErr: true,
		},
		{
			name: "invalid header name",
			cfg: &v1.HTTPProxyConfig{
				HTTPUser:       "alice",
				HTTPPassword:   "secret",
				HTTPAuthHeader: "X FRP Auth",
			},
			wantErr: true,
		},
		{
			name: "ip and cidr entries",
			cfg: &v1.HTTPProxyConfig{
				AllowIPs: []string{"10.0.0.0/8", "192.168.1.10", "2001:db8::/32", "::1"},
			},
		},
		{
			name:    "invalid cidr",
			cfg:     &v1.HTTPProxyConfig{AllowIPs: []string{"10.0.0.0/33"}},
			wantErr: true,
		},
		{
			name:    "invalid ip",
			cfg:     &v1.HTTPProxyConfig{AllowIPs: []string{"10.0.0"}},
			wantErr: true,
		},
		{
			name:    "empty entry",
			cfg:     &v1.HTTPProxyConfig{AllowIPs: []string{" "}},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// A domain is required by the same validator, so keep it valid to
			// isolate the auth and allowIPs checks.
			tc.cfg.CustomDomains = []string{"www.example.com"}

			err := validateHTTPProxyConfigForClient(tc.cfg)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

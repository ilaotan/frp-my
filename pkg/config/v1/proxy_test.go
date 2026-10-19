// Copyright 2023 The frp Authors
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

package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/fatedier/frp/pkg/msg"
)

func TestUnmarshalTypedProxyConfig(t *testing.T) {
	require := require.New(t)
	proxyConfigs := struct {
		Proxies []TypedProxyConfig `json:"proxies,omitempty"`
	}{}

	strs := `{
		"proxies": [
			{
				"type": "tcp",
				"localPort": 22,
				"remotePort": 6000
			},
			{
				"type": "http",
				"localPort": 80,
				"customDomains": ["www.example.com"]
			}
		]
	}`
	err := json.Unmarshal([]byte(strs), &proxyConfigs)
	require.NoError(err)

	require.IsType(&TCPProxyConfig{}, proxyConfigs.Proxies[0].ProxyConfigurer)
	require.IsType(&HTTPProxyConfig{}, proxyConfigs.Proxies[1].ProxyConfigurer)
}

func TestHTTPProxyConfigAuthHeaderAndAllowIPs(t *testing.T) {
	require := require.New(t)

	cfg := &HTTPProxyConfig{}
	err := json.Unmarshal([]byte(`{
		"type": "http",
		"customDomains": ["www.example.com"],
		"httpUser": "alice",
		"httpPassword": "secret",
		"httpAuthHeader": "X-FRP-Auth",
		"httpAuthPagePath": "/_s3cr3t/entry",
		"allowIPs": ["10.0.0.0/8", "192.168.1.10"]
	}`), cfg)
	require.NoError(err)
	require.Equal("X-FRP-Auth", cfg.HTTPAuthHeader)
	require.Equal("/_s3cr3t/entry", cfg.HTTPAuthPagePath)
	require.Equal([]string{"10.0.0.0/8", "192.168.1.10"}, cfg.AllowIPs)

	// The fields must survive the frpc -> frps control message round trip.
	var m msg.NewProxy
	cfg.MarshalToMsg(&m)
	require.Equal("X-FRP-Auth", m.HTTPAuthHeader)
	require.Equal("/_s3cr3t/entry", m.HTTPAuthPagePath)
	require.Equal([]string{"10.0.0.0/8", "192.168.1.10"}, m.AllowIPs)

	restored := &HTTPProxyConfig{}
	restored.UnmarshalFromMsg(&m)
	require.Equal("X-FRP-Auth", restored.HTTPAuthHeader)
	require.Equal("/_s3cr3t/entry", restored.HTTPAuthPagePath)
	require.Equal([]string{"10.0.0.0/8", "192.168.1.10"}, restored.AllowIPs)

	cloned, ok := cfg.Clone().(*HTTPProxyConfig)
	require.True(ok)
	require.Equal([]string{"10.0.0.0/8", "192.168.1.10"}, cloned.AllowIPs)
}

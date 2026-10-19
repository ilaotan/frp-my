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

package source

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func TestNewConfigSource(t *testing.T) {
	require := require.New(t)

	src := NewConfigSource()
	require.NotNil(src)
}

func TestConfigSource_ReplaceAll(t *testing.T) {
	require := require.New(t)

	src := NewConfigSource()

	err := src.ReplaceAll(
		[]v1.ProxyConfigurer{mockProxy("proxy1"), mockProxy("proxy2")},
	)
	require.NoError(err)

	proxies, err := src.Load()
	require.NoError(err)
	require.Len(proxies, 2)

	// ReplaceAll again should replace everything
	err = src.ReplaceAll(
		[]v1.ProxyConfigurer{mockProxy("proxy3")},
	)
	require.NoError(err)

	proxies, err = src.Load()
	require.NoError(err)
	require.Len(proxies, 1)
	require.Equal("proxy3", proxies[0].GetBaseConfig().Name)

	// ReplaceAll with nil proxy should fail
	err = src.ReplaceAll([]v1.ProxyConfigurer{nil})
	require.Error(err)

	// ReplaceAll with empty name proxy should fail
	err = src.ReplaceAll([]v1.ProxyConfigurer{&v1.TCPProxyConfig{}})
	require.Error(err)
}

func TestConfigSource_Load(t *testing.T) {
	require := require.New(t)

	src := NewConfigSource()

	err := src.ReplaceAll(
		[]v1.ProxyConfigurer{mockProxy("proxy1"), mockProxy("proxy2")},
	)
	require.NoError(err)

	proxies, err := src.Load()
	require.NoError(err)
	require.Len(proxies, 2)
}

// TestConfigSource_Load_FiltersDisabled verifies that Load() filters out
// proxies and visitors with Enabled explicitly set to false.
func TestConfigSource_Load_FiltersDisabled(t *testing.T) {
	require := require.New(t)

	src := NewConfigSource()

	disabled := false
	enabled := true

	// Create enabled proxy (nil Enabled = enabled by default)
	enabledProxy := mockProxy("enabled-proxy")

	// Create disabled proxy
	disabledProxy := &v1.TCPProxyConfig{}
	disabledProxy.Name = "disabled-proxy"
	disabledProxy.Type = "tcp"
	disabledProxy.Enabled = &disabled

	// Create explicitly enabled proxy
	explicitEnabledProxy := &v1.TCPProxyConfig{}
	explicitEnabledProxy.Name = "explicit-enabled-proxy"
	explicitEnabledProxy.Type = "tcp"
	explicitEnabledProxy.Enabled = &enabled

	err := src.ReplaceAll(
		[]v1.ProxyConfigurer{enabledProxy, disabledProxy, explicitEnabledProxy},
	)
	require.NoError(err)

	// Load should filter out disabled configs
	proxies, err := src.Load()
	require.NoError(err)
	require.Len(proxies, 2, "Should have 2 enabled proxies")

	// Verify the correct proxies are returned
	proxyNames := make([]string, 0, len(proxies))
	for _, p := range proxies {
		proxyNames = append(proxyNames, p.GetBaseConfig().Name)
	}
	require.Contains(proxyNames, "enabled-proxy")
	require.Contains(proxyNames, "explicit-enabled-proxy")
	require.NotContains(proxyNames, "disabled-proxy")
}

func TestConfigSource_ReplaceAll_DoesNotApplyRuntimeDefaults(t *testing.T) {
	require := require.New(t)

	src := NewConfigSource()

	proxyCfg := &v1.TCPProxyConfig{}
	proxyCfg.Name = "proxy1"
	proxyCfg.Type = "tcp"
	proxyCfg.LocalPort = 10080

	err := src.ReplaceAll([]v1.ProxyConfigurer{proxyCfg})
	require.NoError(err)

	proxies, err := src.Load()
	require.NoError(err)
	require.Len(proxies, 1)
	require.Empty(proxies[0].GetBaseConfig().LocalIP)
}

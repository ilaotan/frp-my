package v1

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProxyCloneDeepCopy(t *testing.T) {
	require := require.New(t)

	enabled := true
	pluginHTTP2 := true
	cfg := &TCPProxyConfig{
		ProxyBaseConfig: ProxyBaseConfig{
			Name:        "p1",
			Type:        "tcp",
			Enabled:     &enabled,
			Annotations: map[string]string{"a": "1"},
			Metadatas:   map[string]string{"m": "1"},
			HealthCheck: HealthCheckConfig{
				Type: "http",
				HTTPHeaders: []HTTPHeader{
					{Name: "X-Test", Value: "v1"},
				},
			},
			ProxyBackend: ProxyBackend{
				Plugin: TypedClientPluginOptions{
					Type: PluginHTTPS2HTTP,
					ClientPluginOptions: &HTTPS2HTTPPluginOptions{
						Type:           PluginHTTPS2HTTP,
						EnableHTTP2:    &pluginHTTP2,
						RequestHeaders: HeaderOperations{Set: map[string]string{"k": "v"}},
					},
				},
			},
		},
		RemotePort: 6000,
	}

	cloned := cfg.Clone().(*TCPProxyConfig)

	*cloned.Enabled = false
	cloned.Annotations["a"] = "changed"
	cloned.Metadatas["m"] = "changed"
	cloned.HealthCheck.HTTPHeaders[0].Value = "changed"
	cloned.RemotePort = 7000
	clientPlugin := cloned.Plugin.ClientPluginOptions.(*HTTPS2HTTPPluginOptions)
	*clientPlugin.EnableHTTP2 = false
	clientPlugin.RequestHeaders.Set["k"] = "changed"

	require.True(*cfg.Enabled)
	require.Equal("1", cfg.Annotations["a"])
	require.Equal("1", cfg.Metadatas["m"])
	require.Equal("v1", cfg.HealthCheck.HTTPHeaders[0].Value)
	require.Equal(6000, cfg.RemotePort)

	origPlugin := cfg.Plugin.ClientPluginOptions.(*HTTPS2HTTPPluginOptions)
	require.True(*origPlugin.EnableHTTP2)
	require.Equal("v", origPlugin.RequestHeaders.Set["k"])
}

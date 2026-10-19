package source

import (
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

func TestBaseSourceLoadReturnsClonedConfigurers(t *testing.T) {
	require := require.New(t)

	src := NewConfigSource()

	proxyCfg := &v1.TCPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name: "proxy1",
			Type: "tcp",
		},
	}

	err := src.ReplaceAll([]v1.ProxyConfigurer{proxyCfg})
	require.NoError(err)

	firstProxies, err := src.Load()
	require.NoError(err)
	require.Len(firstProxies, 1)

	// Mutate loaded objects as runtime completion would do.
	firstProxies[0].Complete()

	secondProxies, err := src.Load()
	require.NoError(err)
	require.Len(secondProxies, 1)

	require.Empty(secondProxies[0].GetBaseConfig().LocalIP)
}

package basic

import (
	"fmt"
	"strings"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/pkg/port"
	"github.com/onsi/ginkgo/v2"
)

var _ = ginkgo.Describe("[Feature: Basic]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("TCP", func() {
		ginkgo.It("Expose a tcp echo server", func() {
			serverConf := consts.DefaultServerConfig
			var clientConf strings.Builder
			clientConf.WriteString(consts.DefaultClientConfig)

			getProxyConf := func(proxyName string, portName string, extra string) string {
				return fmt.Sprintf(`
			[[proxies]]
			name = "%s"
			type = "tcp"
			localPort = {{ .%s }}
			remotePort = {{ .%s }}
			`+extra, proxyName, framework.TCPEchoServerPort, portName)
			}

			tests := []struct {
				proxyName   string
				portName    string
				extraConfig string
			}{
				{
					proxyName: "normal",
					portName:  port.GenName("Normal"),
				},
				{
					proxyName:   "with-encryption",
					portName:    port.GenName("WithEncryption"),
					extraConfig: "transport.useEncryption = true",
				},
				{
					proxyName:   "with-compression",
					portName:    port.GenName("WithCompression"),
					extraConfig: "transport.useCompression = true",
				},
				{
					proxyName: "with-encryption-and-compression",
					portName:  port.GenName("WithEncryptionAndCompression"),
					extraConfig: `
					transport.useEncryption = true
					transport.useCompression = true
					`,
				},
			}

			// build all client config
			for _, test := range tests {
				clientConf.WriteString(getProxyConf(test.proxyName, test.portName, test.extraConfig) + "\n")
			}
			// run frps and frpc
			f.RunProcesses(serverConf, []string{clientConf.String()})

			for _, test := range tests {
				framework.NewRequestExpect(f).
					PortName(test.portName).
					Explain(test.proxyName).
					Ensure()
			}
		})
	})
})

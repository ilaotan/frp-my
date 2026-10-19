package plugin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/mock/server/httpserver"
	"github.com/fatedier/frp/test/e2e/pkg/port"
	"github.com/fatedier/frp/test/e2e/pkg/request"
	"github.com/onsi/ginkgo/v2"
)

var _ = ginkgo.Describe("[Feature: Client-Plugins]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("UnixDomainSocket", func() {
		ginkgo.It("Expose a unix domain socket echo server", func() {
			serverConf := consts.DefaultServerConfig
			var clientConf strings.Builder
			clientConf.WriteString(consts.DefaultClientConfig)

			getProxyConf := func(proxyName string, portName string, extra string) string {
				return fmt.Sprintf(`
				[[proxies]]
				name = "%s"
				type = "tcp"
				remotePort = {{ .%s }}
				`+extra, proxyName, portName) + fmt.Sprintf(`
				[proxies.plugin]
				type = "unix_domain_socket"
				unixPath = "{{ .%s }}"
				`, framework.UDSEchoServerAddr)
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
				framework.NewRequestExpect(f).Port(f.PortByName(test.portName)).Ensure()
			}
		})
	})

	ginkgo.It("http_proxy", func() {
		serverConf := consts.DefaultServerConfig
		clientConf := consts.DefaultClientConfig

		remotePort := f.AllocPort()
		clientConf += fmt.Sprintf(`
		[[proxies]]
		name = "tcp"
		type = "tcp"
		remotePort = %d
		[proxies.plugin]
		type = "http_proxy"
		httpUser = "abc"
		httpPassword = "123"
		`, remotePort)

		f.RunProcesses(serverConf, []string{clientConf})

		// http proxy, no auth info
		framework.NewRequestExpect(f).PortName(framework.HTTPSimpleServerPort).RequestModify(func(r *request.Request) {
			r.HTTP().Proxy("http://127.0.0.1:" + strconv.Itoa(remotePort))
		}).Ensure(framework.ExpectResponseCode(407))

		// http proxy, correct auth
		framework.NewRequestExpect(f).PortName(framework.HTTPSimpleServerPort).RequestModify(func(r *request.Request) {
			r.HTTP().Proxy("http://abc:123@127.0.0.1:" + strconv.Itoa(remotePort))
		}).Ensure()

		// connect TCP server by CONNECT method
		framework.NewRequestExpect(f).PortName(framework.TCPEchoServerPort).RequestModify(func(r *request.Request) {
			r.TCP().Proxy("http://abc:123@127.0.0.1:" + strconv.Itoa(remotePort))
		})
	})

	ginkgo.It("socks5 proxy", func() {
		serverConf := consts.DefaultServerConfig
		clientConf := consts.DefaultClientConfig

		remotePort := f.AllocPort()
		clientConf += fmt.Sprintf(`
		[[proxies]]
		name = "tcp"
		type = "tcp"
		remotePort = %d
		[proxies.plugin]
		type = "socks5"
		username = "abc"
		password = "123"
		`, remotePort)

		f.RunProcesses(serverConf, []string{clientConf})

		// http proxy, no auth info
		framework.NewRequestExpect(f).PortName(framework.TCPEchoServerPort).RequestModify(func(r *request.Request) {
			r.TCP().Proxy("socks5://127.0.0.1:" + strconv.Itoa(remotePort))
		}).ExpectError(true).Ensure()

		// http proxy, correct auth
		framework.NewRequestExpect(f).PortName(framework.TCPEchoServerPort).RequestModify(func(r *request.Request) {
			r.TCP().Proxy("socks5://abc:123@127.0.0.1:" + strconv.Itoa(remotePort))
		}).Ensure()
	})

	ginkgo.Describe("http2http", func() {
		ginkgo.It("host header rewrite", func() {
			serverConf := consts.DefaultServerConfig

			localPort := f.AllocPort()
			remotePort := f.AllocPort()
			clientConf := consts.DefaultClientConfig + fmt.Sprintf(`
			[[proxies]]
			name = "http2http"
			type = "tcp"
			remotePort = %d
			[proxies.plugin]
			type = "http2http"
			localAddr = "127.0.0.1:%d"
			hostHeaderRewrite = "rewrite.test.com"
			`, remotePort, localPort)

			f.RunProcesses(serverConf, []string{clientConf})

			localServer := httpserver.New(
				httpserver.WithBindPort(localPort),
				httpserver.WithHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = w.Write([]byte(req.Host))
				})),
			)
			f.RunServer("", localServer)

			framework.NewRequestExpect(f).
				Port(remotePort).
				RequestModify(func(r *request.Request) {
					r.HTTP().HTTPHost("example.com")
				}).
				ExpectResp([]byte("rewrite.test.com")).
				Ensure()
		})

		ginkgo.It("set request header", func() {
			serverConf := consts.DefaultServerConfig

			localPort := f.AllocPort()
			remotePort := f.AllocPort()
			clientConf := consts.DefaultClientConfig + fmt.Sprintf(`
			[[proxies]]
			name = "http2http"
			type = "tcp"
			remotePort = %d
			[proxies.plugin]
			type = "http2http"
			localAddr = "127.0.0.1:%d"
			requestHeaders.set.x-from-where = "frp"
			`, remotePort, localPort)

			f.RunProcesses(serverConf, []string{clientConf})

			localServer := httpserver.New(
				httpserver.WithBindPort(localPort),
				httpserver.WithHandler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = w.Write([]byte(req.Header.Get("x-from-where")))
				})),
			)
			f.RunServer("", localServer)

			framework.NewRequestExpect(f).
				Port(remotePort).
				RequestModify(func(r *request.Request) {
					r.HTTP().HTTPHost("example.com")
				}).
				ExpectResp([]byte("frp")).
				Ensure()
		})
	})

})

package features

import (
	"bufio"
	"fmt"
	"net"

	"github.com/fatedier/frp/pkg/util/log"
	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/mock/server/streamserver"
	"github.com/fatedier/frp/test/e2e/pkg/request"
	"github.com/fatedier/frp/test/e2e/pkg/rpc"
	"github.com/onsi/ginkgo/v2"
	pp "github.com/pires/go-proxyproto"
)

var _ = ginkgo.Describe("[Feature: Real IP]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("Proxy Protocol", func() {
		ginkgo.It("TCP", func() {
			serverConf := consts.LegacyDefaultServerConfig
			clientConf := consts.LegacyDefaultClientConfig

			localPort := f.AllocPort()
			localServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(localPort),
				streamserver.WithCustomHandler(func(c net.Conn) {
					defer c.Close()
					rd := bufio.NewReader(c)
					ppHeader, err := pp.Read(rd)
					if err != nil {
						log.Errorf("read proxy protocol error: %v", err)
						return
					}

					for {
						if _, err := rpc.ReadBytes(rd); err != nil {
							return
						}

						buf := []byte(ppHeader.SourceAddr.String())
						_, _ = rpc.WriteBytes(c, buf)
					}
				}))
			f.RunServer("", localServer)

			remotePort := f.AllocPort()
			clientConf += fmt.Sprintf(`
			[tcp]
			type = tcp
			local_port = %d
			remote_port = %d
			proxy_protocol_version = v2
			`, localPort, remotePort)

			f.RunProcesses(serverConf, []string{clientConf})

			framework.NewRequestExpect(f).Port(remotePort).Ensure(func(resp *request.Response) bool {
				log.Tracef("proxy protocol get SourceAddr: %s", string(resp.Content))
				addr, err := net.ResolveTCPAddr("tcp", string(resp.Content))
				if err != nil {
					return false
				}
				if addr.IP.String() != "127.0.0.1" {
					return false
				}
				return true
			})
		})

	})
})

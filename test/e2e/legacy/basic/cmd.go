package basic

import (
	"strconv"
	"strings"

	"github.com/onsi/ginkgo/v2"

	"github.com/fatedier/frp/test/e2e/framework"
)

const (
	ConfigValidStr = "syntax is ok"
)

var _ = ginkgo.Describe("[Feature: Cmd]", func() {
	f := framework.NewDefaultFramework()

	ginkgo.Describe("Verify", func() {
		ginkgo.It("frps valid", func() {
			path := f.GenerateConfigFile(`
			[common]
			bind_addr = 0.0.0.0
			bind_port = 7000
			`)
			_, output, err := f.RunFrps("verify", "-c", path)
			framework.ExpectNoError(err)
			framework.ExpectTrue(strings.Contains(output, ConfigValidStr), "output: %s", output)
		})
		ginkgo.It("frps invalid", func() {
			path := f.GenerateConfigFile(`
			[common]
			bind_addr = 0.0.0.0
			bind_port = 70000
			`)
			_, output, err := f.RunFrps("verify", "-c", path)
			framework.ExpectNoError(err)
			framework.ExpectTrue(!strings.Contains(output, ConfigValidStr), "output: %s", output)
		})
		ginkgo.It("frpc valid", func() {
			path := f.GenerateConfigFile(`
			[common]
			server_addr = 0.0.0.0
			server_port = 7000
			`)
			_, output, err := f.RunFrpc("verify", "-c", path)
			framework.ExpectNoError(err)
			framework.ExpectTrue(strings.Contains(output, ConfigValidStr), "output: %s", output)
		})
		ginkgo.It("frpc invalid", func() {
			path := f.GenerateConfigFile(`
			[common]
			server_addr = 0.0.0.0
			server_port = 7000
			protocol = invalid
			`)
			_, output, err := f.RunFrpc("verify", "-c", path)
			framework.ExpectNoError(err)
			framework.ExpectTrue(!strings.Contains(output, ConfigValidStr), "output: %s", output)
		})
	})

	ginkgo.Describe("Single proxy", func() {
		ginkgo.It("TCP", func() {
			serverPort := f.AllocPort()
			_, _, err := f.RunFrps("-t", "123", "-p", strconv.Itoa(serverPort))
			framework.ExpectNoError(err)

			localPort := f.PortByName(framework.TCPEchoServerPort)
			remotePort := f.AllocPort()
			_, _, err = f.RunFrpc("tcp", "-s", "127.0.0.1", "-P", strconv.Itoa(serverPort), "-t", "123", "-u", "test",
				"-l", strconv.Itoa(localPort), "-r", strconv.Itoa(remotePort), "-n", "tcp_test")
			framework.ExpectNoError(err)

			framework.NewRequestExpect(f).Port(remotePort).Ensure()
		})

	})
})

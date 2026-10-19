package features

import (
	"fmt"
	"strconv"
	"time"

	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/mock/server/streamserver"
	"github.com/fatedier/frp/test/e2e/pkg/request"
	"github.com/onsi/ginkgo/v2"
)

var _ = ginkgo.Describe("[Feature: Group]", func() {
	f := framework.NewDefaultFramework()

	validateFooBarResponse := func(resp *request.Response) bool {
		if string(resp.Content) == "foo" || string(resp.Content) == "bar" {
			return true
		}
		return false
	}

	ginkgo.Describe("Load Balancing", func() {
		ginkgo.It("TCP", func() {
			serverConf := consts.LegacyDefaultServerConfig
			clientConf := consts.LegacyDefaultClientConfig

			fooPort := f.AllocPort()
			fooServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(fooPort), streamserver.WithRespContent([]byte("foo")))
			f.RunServer("", fooServer)

			barPort := f.AllocPort()
			barServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(barPort), streamserver.WithRespContent([]byte("bar")))
			f.RunServer("", barServer)

			remotePort := f.AllocPort()
			clientConf += fmt.Sprintf(`
			[foo]
			type = tcp
			local_port = %d
			remote_port = %d
			group = test
			group_key = 123

			[bar]
			type = tcp
			local_port = %d
			remote_port = %d
			group = test
			group_key = 123
			`, fooPort, remotePort, barPort, remotePort)

			f.RunProcesses(serverConf, []string{clientConf})

			fooCount := 0
			barCount := 0
			for i := range 10 {
				framework.NewRequestExpect(f).Explain("times " + strconv.Itoa(i)).Port(remotePort).Ensure(func(resp *request.Response) bool {
					switch string(resp.Content) {
					case "foo":
						fooCount++
					case "bar":
						barCount++
					default:
						return false
					}
					return true
				})
			}

			framework.ExpectTrue(fooCount > 1 && barCount > 1, "fooCount: %d, barCount: %d", fooCount, barCount)
		})
	})

	ginkgo.Describe("Health Check", func() {
		ginkgo.It("TCP", func() {
			serverConf := consts.LegacyDefaultServerConfig
			clientConf := consts.LegacyDefaultClientConfig

			fooPort := f.AllocPort()
			fooServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(fooPort), streamserver.WithRespContent([]byte("foo")))
			f.RunServer("", fooServer)

			barPort := f.AllocPort()
			barServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(barPort), streamserver.WithRespContent([]byte("bar")))
			f.RunServer("", barServer)

			remotePort := f.AllocPort()
			clientConf += fmt.Sprintf(`
			[foo]
			type = tcp
			local_port = %d
			remote_port = %d
			group = test
			group_key = 123
			health_check_type = tcp
			health_check_interval_s = 1

			[bar]
			type = tcp
			local_port = %d
			remote_port = %d
			group = test
			group_key = 123
			health_check_type = tcp
			health_check_interval_s = 1
			`, fooPort, remotePort, barPort, remotePort)

			f.RunProcesses(serverConf, []string{clientConf})

			// check foo and bar is ok
			results := []string{}
			for range 10 {
				framework.NewRequestExpect(f).Port(remotePort).Ensure(validateFooBarResponse, func(resp *request.Response) bool {
					results = append(results, string(resp.Content))
					return true
				})
			}
			framework.ExpectContainElements(results, []string{"foo", "bar"})

			// close bar server, check foo is ok
			barServer.Close()
			time.Sleep(2 * time.Second)
			for range 10 {
				framework.NewRequestExpect(f).Port(remotePort).ExpectResp([]byte("foo")).Ensure()
			}

			// resume bar server, check foo and bar is ok
			f.RunServer("", barServer)
			time.Sleep(2 * time.Second)
			results = []string{}
			for range 10 {
				framework.NewRequestExpect(f).Port(remotePort).Ensure(validateFooBarResponse, func(resp *request.Response) bool {
					results = append(results, string(resp.Content))
					return true
				})
			}
			framework.ExpectContainElements(results, []string{"foo", "bar"})
		})

	})
})

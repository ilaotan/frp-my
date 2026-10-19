package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/fatedier/frp/pkg/sdk/client"
	"github.com/fatedier/frp/test/e2e/framework"
	"github.com/fatedier/frp/test/e2e/framework/consts"
	"github.com/fatedier/frp/test/e2e/mock/server/streamserver"
	"github.com/fatedier/frp/test/e2e/pkg/request"
	"github.com/onsi/ginkgo/v2"
)

func waitForProxyStatus(proxyClient *client.Client, proxyName, want string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	var lastStatus string
	var lastErr error
	for {
		status, err := proxyClient.GetProxyStatus(ctx, proxyName)
		if err == nil {
			lastErr = nil
			lastStatus = status.Status
			if status.Status == want {
				return nil
			}
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("timeout waiting for proxy %q status %q: last error: %w", proxyName, want, lastErr)
			}
			return fmt.Errorf("timeout waiting for proxy %q status %q: last status %q", proxyName, want, lastStatus)
		case <-ticker.C:
		}
	}
}

func waitForSignal(signal <-chan struct{}) error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	select {
	case <-signal:
		return nil
	case <-timer.C:
		return fmt.Errorf("timeout waiting for backend health-check signal")
	}
}

// Keep this longer than client/proxy.statusCheckInterval (3s). The wrapper's
// health notification is deliberately non-blocking, so the E2E assertion must
// also cover the fallback poll plus scheduling margin while recovery is gated.
const proxyFallbackObservationWindow = 4 * time.Second

func waitForServerProxyStatus(port int, proxyName, want string, timeout time.Duration) error {
	return waitForLifecycleCondition(timeout, func() error {
		body, err := getLifecycleEndpoint(port, "/api/proxies/"+url.PathEscape(proxyName))
		if err != nil {
			return err
		}
		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(body), &status); err != nil {
			return err
		}
		if status.Status != want {
			return fmt.Errorf("frps proxy %q status %q, want %q", proxyName, status.Status, want)
		}
		return nil
	})
}

type httpHealthStage struct {
	healthy bool
	signal  chan<- struct{}
	release <-chan struct{}
}

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
			serverConf := consts.DefaultServerConfig
			clientConf := consts.DefaultClientConfig

			fooPort := f.AllocPort()
			fooServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(fooPort), streamserver.WithRespContent([]byte("foo")))
			f.RunServer("", fooServer)

			barPort := f.AllocPort()
			barServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(barPort), streamserver.WithRespContent([]byte("bar")))
			f.RunServer("", barServer)

			remotePort := f.AllocPort()
			clientConf += fmt.Sprintf(`
			[[proxies]]
			name = "foo"
			type = "tcp"
			localPort = %d
			remotePort = %d
			loadBalancer.group = "test"
			loadBalancer.groupKey = "123"

			[[proxies]]
			name = "bar"
			type = "tcp"
			localPort = %d
			remotePort = %d
			loadBalancer.group = "test"
			loadBalancer.groupKey = "123"
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
			dashboardPort := f.AllocPort()
			serverConf := consts.DefaultServerConfig + fmt.Sprintf(`
			webServer.port = %d
			`, dashboardPort)
			clientConf := consts.DefaultClientConfig

			fooPort := f.AllocPort()
			fooServer := streamserver.New(streamserver.TCP, streamserver.WithBindPort(fooPort), streamserver.WithRespContent([]byte("foo")))
			f.RunServer("", fooServer)

			barPort := f.AllocPort()
			newBarServer := func() *streamserver.Server {
				return streamserver.New(streamserver.TCP, streamserver.WithBindPort(barPort), streamserver.WithRespContent([]byte("bar")))
			}
			barServer := newBarServer()
			f.RunServer("", barServer)

			remotePort := f.AllocPort()
			adminPort := f.AllocPort()
			clientConf += fmt.Sprintf(`
			webServer.port = %d

			[[proxies]]
			name = "foo"
			type = "tcp"
			localPort = %d
			remotePort = %d
			loadBalancer.group = "test"
			loadBalancer.groupKey = "123"
			healthCheck.type = "tcp"
			healthCheck.intervalSeconds = 1

			[[proxies]]
			name = "bar"
			type = "tcp"
			localPort = %d
			remotePort = %d
			loadBalancer.group = "test"
			loadBalancer.groupKey = "123"
			healthCheck.type = "tcp"
			healthCheck.intervalSeconds = 1
			healthCheck.maxFailed = 3
			`, adminPort, fooPort, remotePort, barPort, remotePort)

			f.RunProcesses(serverConf, []string{clientConf})
			proxyClient := f.APIClientForFrpc(adminPort)
			framework.ExpectNoError(waitForProxyStatus(proxyClient, "foo", "running"))
			framework.ExpectNoError(waitForProxyStatus(proxyClient, "bar", "running"))

			// Both requests traverse frps, the load-balancing group, frpc, and the
			// corresponding local backend.
			results := []string{}
			for range 10 {
				framework.NewRequestExpect(f).Port(remotePort).Ensure(validateFooBarResponse, func(resp *request.Response) bool {
					results = append(results, string(resp.Content))
					return true
				})
			}
			framework.ExpectContainElements(results, []string{"foo", "bar"})

			// frps removes the group listener before deleting the proxy from
			// its manager. Its offline status is the removal barrier; frpc's
			// local check-failed status alone does not acknowledge that work.
			for range 2 {
				framework.ExpectNoError(barServer.Close())
				framework.ExpectNoError(waitForProxyStatus(proxyClient, "bar", "check failed"))
				framework.ExpectNoError(waitForServerProxyStatus(dashboardPort, "bar", "offline", 5*time.Second))
				for range 10 {
					framework.NewRequestExpect(f).Port(remotePort).ExpectResp([]byte("foo")).Ensure()
				}

				barServer = newBarServer()
				f.RunServer("", barServer)
				framework.ExpectNoError(waitForProxyStatus(proxyClient, "bar", "running"))
				results = []string{}
				for range 10 {
					framework.NewRequestExpect(f).Port(remotePort).Ensure(validateFooBarResponse, func(resp *request.Response) bool {
						results = append(results, string(resp.Content))
						return true
					})
				}
				framework.ExpectContainElements(results, []string{"foo", "bar"})
			}
		})

	})
})

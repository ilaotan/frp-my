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
	"github.com/samber/lo"

	"github.com/fatedier/frp/pkg/config/types"
	"github.com/fatedier/frp/pkg/util/util"
)

type ServerConfig struct {
	APIMetadata

	Auth AuthServerConfig `json:"auth,omitempty"`
	// BindAddr specifies the address that the server binds to. By default,
	// this value is "0.0.0.0".
	BindAddr string `json:"bindAddr,omitempty"`
	// BindPort specifies the port that the server listens on. By default, this
	// value is 7000.
	BindPort int `json:"bindPort,omitempty"`
	// ProxyBindAddr specifies the address that the proxy binds to. This value
	// may be the same as BindAddr.
	ProxyBindAddr string `json:"proxyBindAddr,omitempty"`

	WebServer WebServerConfig `json:"webServer,omitempty"`

	Log LogConfig `json:"log,omitempty"`

	Transport ServerTransportConfig `json:"transport,omitempty"`

	// DetailedErrorsToClient defines whether to send the specific error (with
	// debug info) to frpc. By default, this value is true.
	DetailedErrorsToClient *bool `json:"detailedErrorsToClient,omitempty"`
	// MaxPortsPerClient specifies the maximum number of ports a single client
	// may proxy to. If this value is 0, no limit will be applied.
	MaxPortsPerClient int64 `json:"maxPortsPerClient,omitempty"`
	// UserConnTimeout specifies the maximum time to wait for a work
	// connection. By default, this value is 10.
	UserConnTimeout int64 `json:"userConnTimeout,omitempty"`

	AllowPorts []types.PortsRange `json:"allowPorts,omitempty"`

	HTTPPlugins []HTTPPluginOptions `json:"httpPlugins,omitempty"`
}

func (c *ServerConfig) Complete() error {
	if err := c.Auth.Complete(); err != nil {
		return err
	}
	c.Log.Complete()
	c.Transport.Complete()
	c.WebServer.Complete()

	c.BindAddr = util.EmptyOr(c.BindAddr, "0.0.0.0")
	c.BindPort = util.EmptyOr(c.BindPort, 7000)
	if c.ProxyBindAddr == "" {
		c.ProxyBindAddr = c.BindAddr
	}

	if c.WebServer.Port > 0 {
		c.WebServer.Addr = util.EmptyOr(c.WebServer.Addr, "0.0.0.0")
	}

	c.DetailedErrorsToClient = util.EmptyOr(c.DetailedErrorsToClient, new(true))
	c.UserConnTimeout = util.EmptyOr(c.UserConnTimeout, 10)
	return nil
}

type AuthServerConfig struct {
	Method           AuthMethod   `json:"method,omitempty"`
	AdditionalScopes []AuthScope  `json:"additionalScopes,omitempty"`
	Token            string       `json:"token,omitempty"`
	TokenSource      *ValueSource `json:"tokenSource,omitempty"`
}

func (c *AuthServerConfig) Complete() error {
	c.Method = util.EmptyOr(c.Method, "token")
	return nil
}

type ServerTransportConfig struct {
	// TCPMux toggles TCP stream multiplexing. This allows multiple requests
	// from a client to share a single TCP connection. By default, this value
	// is true.
	// $HideFromDoc
	TCPMux *bool `json:"tcpMux,omitempty"`
	// TCPMuxKeepaliveInterval specifies the keep alive interval for TCP stream multiplier.
	// If TCPMux is true, heartbeat of application layer is unnecessary because it can only rely on heartbeat in TCPMux.
	TCPMuxKeepaliveInterval int64 `json:"tcpMuxKeepaliveInterval,omitempty"`
	// TCPKeepAlive specifies the interval between keep-alive probes for an active network connection between frpc and frps.
	// If negative, keep-alive probes are disabled.
	TCPKeepAlive int64 `json:"tcpKeepalive,omitempty"`
	// MaxPoolCount specifies the maximum pool size for each proxy. By default,
	// this value is 5. Negative values are invalid.
	MaxPoolCount int64 `json:"maxPoolCount,omitempty"`
	// HeartBeatTimeout specifies the maximum time to wait for a heartbeat
	// before terminating the connection. It is not recommended to change this
	// value. By default, this value is 90. Set negative value to disable it.
	HeartbeatTimeout int64 `json:"heartbeatTimeout,omitempty"`
	// TLS specifies TLS settings for the connection from the client.
	TLS TLSServerConfig `json:"tls,omitempty"`
}

func (c *ServerTransportConfig) Complete() {
	c.TCPMux = util.EmptyOr(c.TCPMux, new(true))
	c.TCPMuxKeepaliveInterval = util.EmptyOr(c.TCPMuxKeepaliveInterval, 30)
	c.TCPKeepAlive = util.EmptyOr(c.TCPKeepAlive, 7200)
	c.MaxPoolCount = util.EmptyOr(c.MaxPoolCount, 5)
	if lo.FromPtr(c.TCPMux) {
		// If TCPMux is enabled, heartbeat of application layer is unnecessary because we can rely on heartbeat in tcpmux.
		c.HeartbeatTimeout = util.EmptyOr(c.HeartbeatTimeout, -1)
	} else {
		c.HeartbeatTimeout = util.EmptyOr(c.HeartbeatTimeout, 90)
	}
	if c.TLS.TrustedCaFile != "" {
		c.TLS.Force = true
	}
}

type TLSServerConfig struct {
	// Force specifies whether to only accept TLS-encrypted connections.
	Force bool `json:"force,omitempty"`

	TLSConfig
}

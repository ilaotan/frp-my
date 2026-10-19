// Copyright 2016 fatedier, fatedier@gmail.com
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

package msg

import ()

const (
	TypeLogin         byte = 'o'
	TypeLoginResp     byte = '1'
	TypeNewProxy      byte = 'p'
	TypeNewProxyResp  byte = '2'
	TypeCloseProxy    byte = 'c'
	TypeNewWorkConn   byte = 'w'
	TypeReqWorkConn   byte = 'r'
	TypeStartWorkConn byte = 's'
	TypePing          byte = 'h'
	TypePong          byte = '4'
)

var msgTypeMap = map[byte]any{
	TypeLogin:         Login{},
	TypeLoginResp:     LoginResp{},
	TypeNewProxy:      NewProxy{},
	TypeNewProxyResp:  NewProxyResp{},
	TypeCloseProxy:    CloseProxy{},
	TypeNewWorkConn:   NewWorkConn{},
	TypeReqWorkConn:   ReqWorkConn{},
	TypeStartWorkConn: StartWorkConn{},
	TypePing:          Ping{},
	TypePong:          Pong{},
}

// When frpc start, client send this message to login to server.
type Login struct {
	Version      string            `json:"version,omitempty"`
	Hostname     string            `json:"hostname,omitempty"`
	Os           string            `json:"os,omitempty"`
	Arch         string            `json:"arch,omitempty"`
	User         string            `json:"user,omitempty"`
	PrivilegeKey string            `json:"privilege_key,omitempty"`
	Timestamp    int64             `json:"timestamp,omitempty"`
	RunID        string            `json:"run_id,omitempty"`
	ClientID     string            `json:"client_id,omitempty"`
	Metas        map[string]string `json:"metas,omitempty"`

	// Some global configures.
	PoolCount int `json:"pool_count,omitempty"`
}

type LoginResp struct {
	Version string `json:"version,omitempty"`
	RunID   string `json:"run_id,omitempty"`
	Error   string `json:"error,omitempty"`
}

// When frpc login success, send this message to frps for running a new proxy.
type NewProxy struct {
	ProxyName          string            `json:"proxy_name,omitempty"`
	ProxyType          string            `json:"proxy_type,omitempty"`
	UseEncryption      bool              `json:"use_encryption,omitempty"`
	UseCompression     bool              `json:"use_compression,omitempty"`
	BandwidthLimit     string            `json:"bandwidth_limit,omitempty"`
	BandwidthLimitMode string            `json:"bandwidth_limit_mode,omitempty"`
	Group              string            `json:"group,omitempty"`
	GroupKey           string            `json:"group_key,omitempty"`
	Metas              map[string]string `json:"metas,omitempty"`
	Annotations        map[string]string `json:"annotations,omitempty"`

	// tcp only
	RemotePort int `json:"remote_port,omitempty"`
}

type NewProxyResp struct {
	ProxyName  string `json:"proxy_name,omitempty"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	Error      string `json:"error,omitempty"`
}

type CloseProxy struct {
	ProxyName string `json:"proxy_name,omitempty"`
}

type NewWorkConn struct {
	RunID        string `json:"run_id,omitempty"`
	PrivilegeKey string `json:"privilege_key,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type ReqWorkConn struct{}

type StartWorkConn struct {
	ProxyName string `json:"proxy_name,omitempty"`
	SrcAddr   string `json:"src_addr,omitempty"`
	DstAddr   string `json:"dst_addr,omitempty"`
	SrcPort   uint16 `json:"src_port,omitempty"`
	DstPort   uint16 `json:"dst_port,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Ping struct {
	PrivilegeKey string `json:"privilege_key,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type Pong struct {
	Error string `json:"error,omitempty"`
}

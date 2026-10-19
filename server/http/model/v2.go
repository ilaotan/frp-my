// Copyright 2026 The frp Authors
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

package model

type V2PageResp[T any] struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Items    []T `json:"items"`
}

type V2SystemInfoResp struct {
	Version string                 `json:"version"`
	Config  V2SystemInfoConfigResp `json:"config"`
	Status  V2SystemInfoStatusResp `json:"status"`
}

type V2SystemInfoConfigResp struct {
	BindPort          int    `json:"bindPort"`
	MaxPoolCount      int64  `json:"maxPoolCount"`
	MaxPortsPerClient int64  `json:"maxPortsPerClient"`
	HeartbeatTimeout  int64  `json:"heartbeatTimeout"`
	AllowPortsStr     string `json:"allowPortsStr"`
	TLSForce          bool   `json:"tlsForce"`
}

type V2SystemInfoStatusResp struct {
	TotalTrafficIn  int64            `json:"totalTrafficIn"`
	TotalTrafficOut int64            `json:"totalTrafficOut"`
	CurConns        int64            `json:"curConns"`
	ClientCounts    int64            `json:"clientCounts"`
	ProxyTypeCounts map[string]int64 `json:"proxyTypeCount"`
}

type V2SystemPruneResp struct {
	Type    string `json:"type"`
	Cleared int    `json:"cleared"`
	Total   int    `json:"total"`
}

type V2UserResp struct {
	User        string `json:"user"`
	ClientCount int    `json:"clientCount"`
	ProxyCount  int    `json:"proxyCount"`
}

type V2ClientDetailResp struct {
	ClientInfoResp
	Status V2ClientStatusResp `json:"status"`
}

type V2ClientStatusResp struct {
	State      string `json:"phase"`
	CurConns   int64  `json:"curConns"`
	ProxyCount int64  `json:"proxyCount"`
}

type V2ProxyResp struct {
	Name     string            `json:"name"`
	User     string            `json:"user"`
	ClientID string            `json:"clientID"`
	Spec     V2ProxySpec       `json:"spec"`
	Status   V2ProxyStatusResp `json:"status"`
}

type V2ProxySpec struct {
	Type string `json:"type"`

	TCP *V2TCPProxySpec `json:"tcp,omitempty"`
}

type V2ProxyBaseSpec struct {
	Annotations  map[string]string        `json:"annotations,omitempty"`
	Metadatas    map[string]string        `json:"metadatas,omitempty"`
	Transport    *V2ProxyTransportSpec    `json:"transport,omitempty"`
	LoadBalancer *V2ProxyLoadBalancerSpec `json:"loadBalancer,omitempty"`
}

type V2ProxyTransportSpec struct {
	UseEncryption      bool   `json:"useEncryption"`
	UseCompression     bool   `json:"useCompression"`
	BandwidthLimit     string `json:"bandwidthLimit"`
	BandwidthLimitMode string `json:"bandwidthLimitMode"`
}

type V2ProxyLoadBalancerSpec struct {
	Group string `json:"group"`
}

type V2TCPProxySpec struct {
	V2ProxyBaseSpec
	RemotePort *int `json:"remotePort,omitempty"`
}

type V2ProxyStatusResp struct {
	State           string `json:"phase"`
	TodayTrafficIn  int64  `json:"todayTrafficIn"`
	TodayTrafficOut int64  `json:"todayTrafficOut"`
	CurConns        int64  `json:"curConns"`
	LastStartAt     int64  `json:"lastStartAt,omitempty"`
	LastCloseAt     int64  `json:"lastCloseAt,omitempty"`
}

type V2ProxyTrafficResp struct {
	Name        string                    `json:"name"`
	Unit        string                    `json:"unit"`
	Granularity string                    `json:"granularity"`
	History     []V2ProxyTrafficPointResp `json:"history"`
}

type V2ProxyTrafficPointResp struct {
	Date       string `json:"date"`
	TrafficIn  int64  `json:"trafficIn"`
	TrafficOut int64  `json:"trafficOut"`
}

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

package http

import (
	"encoding/json"
	"strings"
	"testing"

	configtypes "github.com/fatedier/frp/pkg/config/types"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/metrics/mem"
	"github.com/fatedier/frp/server/http/model"
)

func TestBuildV2ProxySpecTCPAndRedaction(t *testing.T) {
	cfg := &v1.TCPProxyConfig{
		ProxyBaseConfig: newV2ProxyTestBaseConfig(t, "tcp"),
		RemotePort:      6000,
	}

	raw := mustMarshalJSON(t, buildV2ProxySpec("tcp", cfg))

	var specObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &specObject); err != nil {
		t.Fatalf("unmarshal spec failed: %v", err)
	}
	assertRawJSONKeys(t, specObject, "tcp", "type")

	var gotType string
	if err := json.Unmarshal(specObject["type"], &gotType); err != nil {
		t.Fatalf("unmarshal spec type failed: %v", err)
	}
	if gotType != "tcp" {
		t.Fatalf("spec type mismatch, want tcp got %q", gotType)
	}

	var block map[string]json.RawMessage
	if err := json.Unmarshal(specObject["tcp"], &block); err != nil {
		t.Fatalf("unmarshal active block failed: %v", err)
	}
	assertRawJSONKeys(t, block, "annotations", "loadBalancer", "metadatas", "remotePort", "transport")
	assertV2ProxyCommonSpec(t, block)
	assertV2ProxyTypeFields(t, "tcp", specObject["tcp"])
	assertNoV2ProxySensitiveFields(t, block)

	content := string(raw)
	for _, secret := range []string{
		"secret-proxy-name",
		"secret-group-key",
		"secret-local-host",
		"secret-plugin-user",
		"secret-plugin-password",
		"secret-health-path",
		"secret-http-user",
		"secret-http-password",
		"secret-request-header",
		"secret-response-header",
	} {
		if strings.Contains(content, secret) {
			t.Fatalf("sensitive value %q leaked in spec: %s", secret, content)
		}
	}
}

func assertV2ProxyTypeFields(t *testing.T, proxyType string, raw json.RawMessage) {
	t.Helper()

	switch proxyType {
	case "tcp":
		var block model.V2TCPProxySpec
		if err := json.Unmarshal(raw, &block); err != nil {
			t.Fatalf("unmarshal tcp block failed: %v", err)
		}
		if block.RemotePort == nil || *block.RemotePort != 6000 {
			t.Fatalf("tcp remote port mismatch: %#v", block.RemotePort)
		}
	}
}

func TestBuildV2ProxyRespOfflineTypedShells(t *testing.T) {
	for _, proxyType := range apiV2ProxyTypes {
		t.Run(proxyType, func(t *testing.T) {
			resp := (&Controller{}).buildV2ProxyResp(&mem.ProxyStats{
				Name: "offline-" + proxyType,
				Type: proxyType,
			})
			if resp.Status.State != "offline" {
				t.Fatalf("offline phase mismatch: %#v", resp.Status)
			}

			var specObject map[string]json.RawMessage
			if err := json.Unmarshal(mustMarshalJSON(t, resp.Spec), &specObject); err != nil {
				t.Fatalf("unmarshal offline spec failed: %v", err)
			}
			assertRawJSONKeys(t, specObject, proxyType, "type")
			assertRawJSONKeysFromMessage(t, specObject[proxyType])
		})
	}
}

func TestBuildV2ProxySpecDoesNotPopulateMismatchedBlock(t *testing.T) {
	spec := buildV2ProxySpec("tcp", nil)

	var specObject map[string]json.RawMessage
	if err := json.Unmarshal(mustMarshalJSON(t, spec), &specObject); err != nil {
		t.Fatalf("unmarshal mismatched spec failed: %v", err)
	}
	assertRawJSONKeys(t, specObject, "tcp", "type")
	assertRawJSONKeysFromMessage(t, specObject["tcp"])
}

func newV2ProxyTestBaseConfig(t *testing.T, proxyType string) v1.ProxyBaseConfig {
	t.Helper()

	bandwidthLimit, err := configtypes.NewBandwidthQuantity("10MB")
	if err != nil {
		t.Fatalf("create bandwidth limit failed: %v", err)
	}
	enabled := false
	return v1.ProxyBaseConfig{
		Name:        "secret-proxy-name",
		Type:        proxyType,
		Enabled:     &enabled,
		Annotations: map[string]string{"annotation-key": "annotation-value"},
		Metadatas:   map[string]string{"metadata-key": "metadata-value"},
		Transport: v1.ProxyTransport{
			UseEncryption:        true,
			UseCompression:       true,
			BandwidthLimit:       bandwidthLimit,
			BandwidthLimitMode:   configtypes.BandwidthLimitModeServer,
			ProxyProtocolVersion: "v2",
		},
		LoadBalancer: v1.LoadBalancerConfig{
			Group:    "public-group",
			GroupKey: "secret-group-key",
		},
		HealthCheck: v1.HealthCheckConfig{
			Type: "http",
			Path: "secret-health-path",
		},
		ProxyBackend: v1.ProxyBackend{
			LocalIP:   "secret-local-host",
			LocalPort: 8080,
			Plugin: v1.TypedClientPluginOptions{
				Type: v1.PluginHTTPProxy,
				ClientPluginOptions: &v1.HTTPProxyPluginOptions{
					Type:         v1.PluginHTTPProxy,
					HTTPUser:     "secret-plugin-user",
					HTTPPassword: "secret-plugin-password",
				},
			},
		},
	}
}

func assertV2ProxyCommonSpec(t *testing.T, block map[string]json.RawMessage) {
	t.Helper()

	var annotations map[string]string
	if err := json.Unmarshal(block["annotations"], &annotations); err != nil {
		t.Fatalf("unmarshal annotations failed: %v", err)
	}
	if annotations["annotation-key"] != "annotation-value" {
		t.Fatalf("annotations mismatch: %#v", annotations)
	}

	var metadatas map[string]string
	if err := json.Unmarshal(block["metadatas"], &metadatas); err != nil {
		t.Fatalf("unmarshal metadatas failed: %v", err)
	}
	if metadatas["metadata-key"] != "metadata-value" {
		t.Fatalf("metadatas mismatch: %#v", metadatas)
	}

	assertRawJSONKeysFromMessage(t, block["transport"],
		"bandwidthLimit",
		"bandwidthLimitMode",
		"useCompression",
		"useEncryption",
	)
	var transport model.V2ProxyTransportSpec
	if err := json.Unmarshal(block["transport"], &transport); err != nil {
		t.Fatalf("unmarshal transport failed: %v", err)
	}
	if !transport.UseEncryption || !transport.UseCompression ||
		transport.BandwidthLimit != "10MB" || transport.BandwidthLimitMode != "server" {
		t.Fatalf("transport mismatch: %#v", transport)
	}

	assertRawJSONKeysFromMessage(t, block["loadBalancer"], "group")
	var loadBalancer model.V2ProxyLoadBalancerSpec
	if err := json.Unmarshal(block["loadBalancer"], &loadBalancer); err != nil {
		t.Fatalf("unmarshal load balancer failed: %v", err)
	}
	if loadBalancer.Group != "public-group" {
		t.Fatalf("load balancer mismatch: %#v", loadBalancer)
	}
}

func assertNoV2ProxySensitiveFields(t *testing.T, value any) {
	t.Helper()

	forbidden := map[string]struct{}{
		"allowUsers":           {},
		"enabled":              {},
		"groupKey":             {},
		"healthCheck":          {},
		"httpPassword":         {},
		"httpUser":             {},
		"localIP":              {},
		"localPort":            {},
		"name":                 {},
		"natTraversal":         {},
		"plugin":               {},
		"proxyProtocolVersion": {},
		"requestHeaders":       {},
		"responseHeaders":      {},
		"secretKey":            {},
		"type":                 {},
	}

	var walk func(any)
	walk = func(current any) {
		switch current := current.(type) {
		case map[string]any:
			for key, nested := range current {
				if _, ok := forbidden[key]; ok {
					t.Fatalf("sensitive field %q leaked in active block", key)
				}
				walk(nested)
			}
		case []any:
			for _, nested := range current {
				walk(nested)
			}
		}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal active block failed: %v", err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode active block failed: %v", err)
	}
	walk(decoded)
}

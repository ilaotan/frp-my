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

package source

import (
	"sync"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

// baseSource provides shared state and behavior for Source implementations.
// It manages proxy storage.
// Concrete types (ConfigSource, StoreSource) embed this struct.
type baseSource struct {
	mu sync.RWMutex

	proxies map[string]v1.ProxyConfigurer
}

func newBaseSource() baseSource {
	return baseSource{
		proxies: make(map[string]v1.ProxyConfigurer),
	}
}

// Load returns all enabled proxy configurations.
// Configurations with Enabled explicitly set to false are filtered out.
func (s *baseSource) Load() ([]v1.ProxyConfigurer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	proxies := make([]v1.ProxyConfigurer, 0, len(s.proxies))
	for _, p := range s.proxies {
		// Filter out disabled proxies (nil or true means enabled)
		if enabled := p.GetBaseConfig().Enabled; enabled != nil && !*enabled {
			continue
		}
		proxies = append(proxies, p)
	}

	return cloneConfigurers(proxies)
}

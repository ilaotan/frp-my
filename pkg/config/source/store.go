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
	"errors"
	"fmt"
	"os"
	"path/filepath"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/util/jsonx"
)

type StoreSourceConfig struct {
	Path string `json:"path"`
}

type storeData struct {
	Proxies []v1.TypedProxyConfig `json:"proxies,omitempty"`
}

type StoreSource struct {
	baseSource
	config StoreSourceConfig
}

var (
	ErrAlreadyExists = errors.New("already exists")
	ErrNotFound      = errors.New("not found")
)

const storeKindProxy = "proxy"

func NewStoreSource(cfg StoreSourceConfig) (*StoreSource, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("path is required")
	}

	s := &StoreSource{
		baseSource: newBaseSource(),
		config:     cfg,
	}

	if err := s.loadFromFile(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load existing data: %w", err)
		}
	}

	return s, nil
}

func (s *StoreSource) loadFromFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadFromFileUnlocked()
}

func (s *StoreSource) loadFromFileUnlocked() error {
	data, err := os.ReadFile(s.config.Path)
	if err != nil {
		return err
	}

	type rawStoreData struct {
		Proxies []jsonx.RawMessage `json:"proxies,omitempty"`
	}
	stored := rawStoreData{}
	if err := jsonx.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	s.proxies = make(map[string]v1.ProxyConfigurer)

	for i, proxyData := range stored.Proxies {
		proxyCfg, err := v1.DecodeProxyConfigurerJSON(proxyData, v1.DecodeOptions{
			DisallowUnknownFields: false,
		})
		if err != nil {
			return fmt.Errorf("failed to decode proxy at index %d: %w", i, err)
		}
		name := proxyCfg.GetBaseConfig().Name
		if name == "" {
			return fmt.Errorf("proxy name cannot be empty")
		}
		s.proxies[name] = proxyCfg
	}

	return nil
}

func (s *StoreSource) saveToFileUnlocked() error {
	stored := storeData{
		Proxies: make([]v1.TypedProxyConfig, 0, len(s.proxies)),
	}

	for _, p := range s.proxies {
		stored.Proxies = append(stored.Proxies, v1.TypedProxyConfig{ProxyConfigurer: p})
	}

	data, err := jsonx.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	dir := filepath.Dir(s.config.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpPath := s.config.Path + ".tmp"

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.config.Path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (s *StoreSource) persistOrRollbackUnlocked(rollback func()) error {
	if err := s.saveToFileUnlocked(); err != nil {
		rollback()
		return fmt.Errorf("failed to persist: %w", err)
	}
	return nil
}

func (s *StoreSource) AddProxy(proxy v1.ProxyConfigurer) error {
	name, err := validateProxyName(proxy)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.proxies[name]; exists {
		return fmt.Errorf("%w: %s %q", ErrAlreadyExists, storeKindProxy, name)
	}

	s.proxies[name] = proxy
	return s.persistOrRollbackUnlocked(func() {
		delete(s.proxies, name)
	})
}

func (s *StoreSource) UpdateProxy(proxy v1.ProxyConfigurer) error {
	name, err := validateProxyName(proxy)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.proxies[name]
	if !exists {
		return fmt.Errorf("%w: %s %q", ErrNotFound, storeKindProxy, name)
	}

	s.proxies[name] = proxy
	return s.persistOrRollbackUnlocked(func() {
		s.proxies[name] = old
	})
}

func (s *StoreSource) RemoveProxy(name string) error {
	if name == "" {
		return fmt.Errorf("%s name cannot be empty", storeKindProxy)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.proxies[name]
	if !exists {
		return fmt.Errorf("%w: %s %q", ErrNotFound, storeKindProxy, name)
	}

	delete(s.proxies, name)
	return s.persistOrRollbackUnlocked(func() {
		s.proxies[name] = old
	})
}

func (s *StoreSource) GetProxy(name string) v1.ProxyConfigurer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, exists := s.proxies[name]
	if !exists {
		return nil
	}
	return p
}

func (s *StoreSource) GetAllProxies() ([]v1.ProxyConfigurer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]v1.ProxyConfigurer, 0, len(s.proxies))
	for _, p := range s.proxies {
		result = append(result, p)
	}
	return result, nil
}

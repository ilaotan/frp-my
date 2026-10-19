package model

import (
	"fmt"
	"strings"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

type ProxyDefinition struct {
	Name string `json:"name"`
	Type string `json:"type"`

	TCP *v1.TCPProxyConfig `json:"tcp,omitempty"`
}

func (p *ProxyDefinition) Validate(pathName string, isUpdate bool) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("proxy name is required")
	}
	if !IsProxyType(p.Type) {
		return fmt.Errorf("invalid proxy type: %s", p.Type)
	}
	if isUpdate && pathName != "" && pathName != p.Name {
		return fmt.Errorf("proxy name in URL must match name in body")
	}

	_, blockType, blockCount := p.activeBlock()
	if blockCount != 1 {
		return fmt.Errorf("exactly one proxy type block is required")
	}
	if blockType != p.Type {
		return fmt.Errorf("proxy type block %q does not match type %q", blockType, p.Type)
	}
	return nil
}

func (p *ProxyDefinition) ToConfigurer() (v1.ProxyConfigurer, error) {
	block, _, _ := p.activeBlock()
	if block == nil {
		return nil, fmt.Errorf("exactly one proxy type block is required")
	}

	cfg := block
	cfg.GetBaseConfig().Name = p.Name
	cfg.GetBaseConfig().Type = p.Type
	return cfg, nil
}

func ProxyDefinitionFromConfigurer(cfg v1.ProxyConfigurer) (ProxyDefinition, error) {
	if cfg == nil {
		return ProxyDefinition{}, fmt.Errorf("proxy config is nil")
	}

	base := cfg.GetBaseConfig()
	payload := ProxyDefinition{
		Name: base.Name,
		Type: base.Type,
	}

	switch c := cfg.(type) {
	case *v1.TCPProxyConfig:
		payload.TCP = c
	default:
		return ProxyDefinition{}, fmt.Errorf("unsupported proxy configurer type %T", cfg)
	}

	return payload, nil
}

func (p *ProxyDefinition) activeBlock() (v1.ProxyConfigurer, string, int) {
	count := 0
	var block v1.ProxyConfigurer
	var blockType string

	if p.TCP != nil {
		count++
		block = p.TCP
		blockType = "tcp"
	}

	return block, blockType, count
}

func IsProxyType(typ string) bool {
	return typ == "tcp"
}

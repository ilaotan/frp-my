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

package sub

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/fatedier/frp/pkg/config"
	"github.com/fatedier/frp/pkg/config/source"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/v1/validation"
	"github.com/fatedier/frp/pkg/policy/security"
)

var proxyTypes = []v1.ProxyType{
	v1.ProxyTypeTCP,
}

func init() {
	for _, typ := range proxyTypes {
		c := v1.NewProxyConfigurerByType(typ)
		if c == nil {
			panic("proxy type: " + typ + " not support")
		}
		clientCfg := v1.ClientCommonConfig{}
		cmd := NewProxyCommand(string(typ), c, &clientCfg)
		config.RegisterClientCommonConfigFlags(cmd, &clientCfg)
		config.RegisterProxyFlags(cmd, c)
		rootCmd.AddCommand(cmd)
	}
}

func NewProxyCommand(name string, c v1.ProxyConfigurer, clientCfg *v1.ClientCommonConfig) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Run frpc with a single %s proxy", name),
		Run: func(cmd *cobra.Command, args []string) {
			if err := clientCfg.Complete(); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			unsafeFeatures := security.NewUnsafeFeatures(allowUnsafe)
			validator := validation.NewConfigValidator(unsafeFeatures)
			if _, err := validator.ValidateClientCommonConfig(clientCfg); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}

			c.GetBaseConfig().Type = name
			c.Complete()
			proxyCfg := c
			if err := validation.ValidateProxyConfigurerForClient(proxyCfg); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			err := startService(clientCfg, []v1.ProxyConfigurer{proxyCfg}, unsafeFeatures, "")
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}
}

func startService(
	cfg *v1.ClientCommonConfig,
	proxyCfgs []v1.ProxyConfigurer,
	unsafeFeatures *security.UnsafeFeatures,
	cfgFile string,
) error {
	configSource := source.NewConfigSource()
	if err := configSource.ReplaceAll(proxyCfgs); err != nil {
		return fmt.Errorf("failed to set config source: %w", err)
	}
	aggregator := source.NewAggregator(configSource)
	return startServiceWithAggregator(cfg, aggregator, unsafeFeatures, cfgFile)
}

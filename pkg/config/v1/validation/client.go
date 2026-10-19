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

package validation

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/samber/lo"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/policy/security"
)

func (v *ConfigValidator) ValidateClientCommonConfig(c *v1.ClientCommonConfig) (Warning, error) {
	var (
		warnings Warning
		errs     error
	)

	validators := []func() (Warning, error){
		func() (Warning, error) { return v.validateAuthConfig(&c.Auth) },
		func() (Warning, error) { return nil, validateLogConfig(&c.Log) },
		func() (Warning, error) { return nil, validateWebServerConfig(&c.WebServer) },
		func() (Warning, error) { return validateTransportConfig(&c.Transport) },
		func() (Warning, error) { return validateIncludeFiles(c.IncludeConfigFiles) },
	}

	for _, validator := range validators {
		w, err := validator()
		warnings = AppendError(warnings, w)
		errs = AppendError(errs, err)
	}
	return warnings, errs
}

func (v *ConfigValidator) validateAuthConfig(c *v1.AuthClientConfig) (Warning, error) {
	var errs error
	if !slices.Contains(SupportedAuthMethods, c.Method) {
		errs = AppendError(errs, fmt.Errorf("invalid auth method, optional values are %v", SupportedAuthMethods))
	}
	if !lo.Every(SupportedAuthAdditionalScopes, c.AdditionalScopes) {
		errs = AppendError(errs, fmt.Errorf("invalid auth additional scopes, optional values are %v", SupportedAuthAdditionalScopes))
	}

	errs = AppendError(errs, v.validateAuthTokenSource(c.Token, c.TokenSource))
	return nil, errs
}

func validateTransportConfig(c *v1.ClientTransportConfig) (Warning, error) {
	var (
		warnings Warning
		errs     error
	)

	if c.HeartbeatTimeout > 0 && c.HeartbeatInterval > 0 {
		if c.HeartbeatTimeout < c.HeartbeatInterval {
			errs = AppendError(errs, fmt.Errorf("invalid transport.heartbeatTimeout, heartbeat timeout should not less than heartbeat interval"))
		}
	}

	if !lo.FromPtr(c.TLS.Enable) {
		checkTLSConfig := func(name string, value string) Warning {
			if value != "" {
				return fmt.Errorf("%s is invalid when transport.tls.enable is false", name)
			}
			return nil
		}

		warnings = AppendError(warnings, checkTLSConfig("transport.tls.certFile", c.TLS.CertFile))
		warnings = AppendError(warnings, checkTLSConfig("transport.tls.keyFile", c.TLS.KeyFile))
		warnings = AppendError(warnings, checkTLSConfig("transport.tls.trustedCaFile", c.TLS.TrustedCaFile))
		// wss keeps TLS on regardless of transport.tls.enable.
		if c.TLS.UseSystemCa && c.Protocol != "wss" {
			errs = AppendError(errs, fmt.Errorf("transport.tls.useSystemCa requires transport.tls.enable to be true unless transport.protocol is wss"))
		}
	}

	if !slices.Contains(SupportedTransportProtocols, c.Protocol) {
		errs = AppendError(errs, fmt.Errorf("invalid transport.protocol, optional values are %v", SupportedTransportProtocols))
	}
	if !slices.Contains(SupportedWireProtocols, c.WireProtocol) {
		errs = AppendError(errs, fmt.Errorf("invalid transport.wireProtocol, optional values are %v", SupportedWireProtocols))
	}
	return warnings, errs
}

func validateIncludeFiles(files []string) (Warning, error) {
	var errs error
	for _, f := range files {
		absDir, err := filepath.Abs(filepath.Dir(f))
		if err != nil {
			errs = AppendError(errs, fmt.Errorf("include: parse directory of %s failed: %v", f, err))
			continue
		}
		if _, err := os.Stat(absDir); os.IsNotExist(err) {
			errs = AppendError(errs, fmt.Errorf("include: directory of %s not exist", f))
		}
	}
	return nil, errs
}

func ValidateAllClientConfig(
	c *v1.ClientCommonConfig,
	proxyCfgs []v1.ProxyConfigurer,
	unsafeFeatures *security.UnsafeFeatures,
) (Warning, error) {
	validator := NewConfigValidator(unsafeFeatures)
	var warnings Warning
	if c != nil {
		warning, err := validator.ValidateClientCommonConfig(c)
		warnings = AppendError(warnings, warning)
		if err != nil {
			return warnings, err
		}
	}

	for _, c := range proxyCfgs {
		if err := ValidateProxyConfigurerForClient(c); err != nil {
			return warnings, fmt.Errorf("proxy %s: %v", c.GetBaseConfig().Name, err)
		}
	}
	return warnings, nil
}

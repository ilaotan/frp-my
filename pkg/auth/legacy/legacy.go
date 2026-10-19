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

package legacy

type BaseConfig struct {
	// AuthenticationMethod specifies what authentication method to use to
	// authenticate frpc with frps. If "token" is specified - token will be
	// read into login message. By default, this value is "token".
	AuthenticationMethod string `ini:"authentication_method" json:"authentication_method"`
	// AuthenticateHeartBeats specifies whether to include authentication token in
	// heartbeats sent to frps. By default, this value is false.
	AuthenticateHeartBeats bool `ini:"authenticate_heartbeats" json:"authenticate_heartbeats"`
	// AuthenticateNewWorkConns specifies whether to include authentication token in
	// new work connections sent to frps. By default, this value is false.
	AuthenticateNewWorkConns bool `ini:"authenticate_new_work_conns" json:"authenticate_new_work_conns"`
}

func getDefaultBaseConf() BaseConfig {
	return BaseConfig{
		AuthenticationMethod:     "token",
		AuthenticateHeartBeats:   false,
		AuthenticateNewWorkConns: false,
	}
}

type ClientConfig struct {
	BaseConfig  `ini:",extends"`
	TokenConfig `ini:",extends"`
}

func GetDefaultClientConf() ClientConfig {
	return ClientConfig{
		BaseConfig:  getDefaultBaseConf(),
		TokenConfig: getDefaultTokenConf(),
	}
}

type ServerConfig struct {
	BaseConfig  `ini:",extends"`
	TokenConfig `ini:",extends"`
}

func GetDefaultServerConf() ServerConfig {
	return ServerConfig{
		BaseConfig:  getDefaultBaseConf(),
		TokenConfig: getDefaultTokenConf(),
	}
}

type TokenConfig struct {
	// Token specifies the authorization token used to create keys to be sent
	// to the server. The server must have a matching token for authorization
	// to succeed.  By default, this value is "".
	Token string `ini:"token" json:"token"`
}

func getDefaultTokenConf() TokenConfig {
	return TokenConfig{
		Token: "",
	}
}

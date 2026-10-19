// Copyright 2019 fatedier, fatedier@gmail.com
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

package controller

import (
	plugin "github.com/fatedier/frp/pkg/plugin/server"
	"github.com/fatedier/frp/server/group"
	"github.com/fatedier/frp/server/ports"
)

// All resource managers and controllers
type ResourceController struct {
	// TCP Group Controller
	TCPGroupCtl *group.TCPGroupCtl

	// Manage all TCP ports
	TCPPortManager *ports.Manager

	// All server manager plugin
	PluginManager *plugin.Manager
}

func (rc *ResourceController) Close() error {
	return nil
}

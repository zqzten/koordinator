//go:build !github
// +build !github

/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package noderesource

import (
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/plugins/batchresource"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/plugins/cpunormalization"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/plugins/dynamicprodresource"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/plugins/midresource"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/plugins/vk"
)

func init() {
	// set default plugins
	addPluginOption(&midresource.Plugin{}, true)
	addPluginOption(&batchresource.Plugin{}, true)
	addPluginOption(&cpunormalization.Plugin{}, true)
	addPluginOption(&dynamicprodresource.Plugin{}, false) // disable by default
	addPluginOption(&vk.Plugin{}, true)
}

func addPlugins(filter framework.FilterFn) {
	// NOTE: plugins run in order of the registration.
	framework.RegisterSetupExtender(filter, setupPlugins...)
	framework.RegisterNodePrepareExtender(filter, nodePreparePlugins...)
	framework.RegisterNodeSyncExtender(filter, nodeSyncPlugins...)
	framework.RegisterNodeMetaSyncExtender(filter, nodeMetaSyncPlugins...)
	framework.RegisterResourceCalculateExtender(filter, resourceCalculatePlugins...)
}

var (
	// SetupPlugin
	setupPlugins = []framework.SetupPlugin{
		&cpunormalization.Plugin{}, // should be first
		&batchresource.Plugin{},
		&dynamicprodresource.Plugin{},
	}
	// NodePreparePlugin implements node resource preparing for the calculated results.
	nodePreparePlugins = []framework.NodePreparePlugin{
		&cpunormalization.Plugin{},
		&midresource.Plugin{},
		&batchresource.Plugin{},
		&dynamicprodresource.Plugin{},
	}
	// NodeSyncPlugin implements the check of resource updating.
	nodeSyncPlugins = []framework.NodeSyncPlugin{
		&midresource.Plugin{},
		&batchresource.Plugin{},
	}
	// NodeMetaSyncPlugin implements the check of node meta updating.
	nodeMetaSyncPlugins = []framework.NodeMetaSyncPlugin{
		&cpunormalization.Plugin{},
		&dynamicprodresource.Plugin{},
	}
	// ResourceCalculatePlugin implements resource counting and overcommitment algorithms.
	resourceCalculatePlugins = []framework.ResourceCalculatePlugin{
		&cpunormalization.Plugin{},
		&midresource.Plugin{},
		&batchresource.Plugin{},
		&dynamicprodresource.Plugin{},
		&vk.Plugin{}, // should be at the ending
	}
)

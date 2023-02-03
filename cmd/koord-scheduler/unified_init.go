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

package main

import (
	_ "github.com/koordinator-sh/koordinator/apis/extension/ack"
	_ "github.com/koordinator-sh/koordinator/apis/extension/unified"
	_ "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare/unified"
	_ "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/gpumodel"
	_ "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/unified"
	_ "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/volumebinding/metrics"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/unified"
	unifiedasiquota "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/asiquotaadaptor"
	unifiedcpuset "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/cpusetallocator"
	unifiedcustomaffinity "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/custompodaffinity"
	unifiedeci "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/eci"
	unifiedelasticquotatree "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/elasticquotatree"
	unifiedinterpodaffinity "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/interpodaffinity"
	unifiednodeaffinity "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/nodeaffinity"
	unifiednodeports "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/nodeports"
	unifiedoverquota "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/overquota"
	unifiedpodconstraint "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint"
	unifiedresourcepolicy "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/resourcepolicy"
	unifiedscheduleresult "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/scheduleresult"
	unifiedtainttoleration "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/tainttoleration"
	unifiedvolumebinding "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/volumebinding"
)

func init() {
	schedulingHooks = append(schedulingHooks, unified.NewHook())

	koordinatorPlugins[unifiedcpuset.Name] = unifiedcpuset.New
	koordinatorPlugins[unifiedeci.Name] = unifiedeci.New
	koordinatorPlugins[unifiedoverquota.Name] = unifiedoverquota.New
	koordinatorPlugins[unifiedpodconstraint.Name] = unifiedpodconstraint.New
	koordinatorPlugins[unifiedscheduleresult.Name] = unifiedscheduleresult.New
	koordinatorPlugins[unifiedcustomaffinity.Name] = unifiedcustomaffinity.New
	koordinatorPlugins[unifiedvolumebinding.Name] = unifiedvolumebinding.New
	koordinatorPlugins[unifiedasiquota.Name] = unifiedasiquota.New
	koordinatorPlugins[unifiedelasticquotatree.Name] = unifiedelasticquotatree.New
	koordinatorPlugins[unifiedresourcepolicy.Name] = unifiedresourcepolicy.New
	koordinatorPlugins[unifiedtainttoleration.Name] = unifiedtainttoleration.New
	koordinatorPlugins[unifiednodeports.Name] = unifiednodeports.New
	koordinatorPlugins[unifiednodeaffinity.Name] = unifiednodeaffinity.New
	koordinatorPlugins[unifiedinterpodaffinity.Name] = unifiedinterpodaffinity.New
}

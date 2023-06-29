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

package utils

import "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"

const (
	DrainNodeKey         = "drain.descheduler.koordinator.sh/drain-node"
	NodeNameKey          = "drain.descheduler.koordinator.sh/node-name"
	PlanningKey          = "drain.descheduler.koordinator.sh/node-planning"
	GroupKey             = "drain.descheduler.koordinator.sh/node-group"
	PodNamespaceKey      = "drain.descheduler.koordinator.sh/pod-namespace"
	PodNameKey           = "drain.descheduler.koordinator.sh/pod-name"
	AbortKey             = "drain.descheduler.koordinator.sh/abort"
	CleanKey             = "drain.descheduler.koordinator.sh/clean-taint"
	MigrationPolicy      = "drain.descheduler.koordinator.sh/migration-policy"
	PausedAfterConfirmed = "drain.descheduler.koordinator.sh/paused-after-confirmed"
)

var DrainNodeGroupKind = v1alpha1.SchemeGroupVersion.WithKind("DrainNodeGroup")

var DrainNodeKind = v1alpha1.SchemeGroupVersion.WithKind("DrainNode")

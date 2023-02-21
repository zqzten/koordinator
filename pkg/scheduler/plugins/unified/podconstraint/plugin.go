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

package podconstraint

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/podconstraint/cache"
)

const (
	Name = "UnifiedPodConstraint"
)

var (
	_ framework.PreFilterPlugin     = &Plugin{}
	_ framework.FilterPlugin        = &Plugin{}
	_ framework.PreFilterExtensions = &Plugin{}
	_ framework.PreScorePlugin      = &Plugin{}
	_ framework.ScorePlugin         = &Plugin{}
	_ framework.ScoreExtensions     = &Plugin{}
)

type Plugin struct {
	handle             framework.Handle
	pluginArgs         *schedulingconfig.UnifiedPodConstraintArgs
	podConstraintCache *cache.PodConstraintCache
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	pluginArgs, ok := args.(*schedulingconfig.UnifiedPodConstraintArgs)
	if !ok {
		return nil, fmt.Errorf("want args to be of type UnifiedPodConstraintArgs, got %T", args)
	}
	podConstraintCache, err := cache.NewPodConstraintCache(handle, *pluginArgs.EnableDefaultPodConstraint)
	if err != nil {
		return nil, err
	}
	return &Plugin{
		handle:             handle,
		pluginArgs:         pluginArgs,
		podConstraintCache: podConstraintCache,
	}, nil
}

func (p *Plugin) Name() string {
	return Name
}

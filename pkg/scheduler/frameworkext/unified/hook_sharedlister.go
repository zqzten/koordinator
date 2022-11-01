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

package unified

import (
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

var (
	_ framework.SharedLister           = &hookSharedLister{}
	_ framework.NodeInfoLister         = &hookSharedLister{}
	_ frameworkext.SharedListerAdapter = NewHookSharedLister
)

type hookSharedLister struct {
	framework.SharedLister
}

func NewHookSharedLister(sharedLister framework.SharedLister) framework.SharedLister {
	return &hookSharedLister{
		SharedLister: sharedLister,
	}
}

func (s *hookSharedLister) NodeInfos() framework.NodeInfoLister {
	return s
}

func (s *hookSharedLister) List() ([]*framework.NodeInfo, error) {
	nodeInfos, err := s.SharedLister.NodeInfos().List()
	if err != nil {
		return nil, err
	}
	hookedNodeInfos := HookNodeInfos(nodeInfos)
	return hookedNodeInfos, nil
}

func (s *hookSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	nodeInfos, err := s.SharedLister.NodeInfos().HavePodsWithAffinityList()
	if err != nil {
		return nil, err
	}
	hookedNodeInfos := HookNodeInfos(nodeInfos)
	return hookedNodeInfos, nil
}

func (s *hookSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	nodeInfos, err := s.SharedLister.NodeInfos().HavePodsWithRequiredAntiAffinityList()
	if err != nil {
		return nil, err
	}
	hookedNodeInfos := HookNodeInfos(nodeInfos)
	return hookedNodeInfos, nil
}

func (s *hookSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	nodeInfo, err := s.SharedLister.NodeInfos().Get(nodeName)
	if err != nil {
		return nil, err
	}
	hookedNodeInfo := HookNodeInfo(nodeInfo)
	return hookedNodeInfo, nil
}

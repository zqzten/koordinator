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
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/overquota"
)

var (
	_ framework.SharedLister           = &overQuotaSharedLister{}
	_ framework.NodeInfoLister         = &overQuotaSharedLister{}
	_ frameworkext.SharedListerAdapter = NewOverQuotaSharedLister
)

type overQuotaSharedLister struct {
	framework.SharedLister
}

func NewOverQuotaSharedLister(sharedLister framework.SharedLister) framework.SharedLister {
	return &overQuotaSharedLister{
		SharedLister: sharedLister,
	}
}

func (s *overQuotaSharedLister) NodeInfos() framework.NodeInfoLister {
	return s
}

func (s *overQuotaSharedLister) List() ([]*framework.NodeInfo, error) {
	nodeInfos, err := s.SharedLister.NodeInfos().List()
	if err != nil {
		return nil, err
	}
	overQuotaNodeInfos, _ := overquota.HookNodeInfosWithOverQuota(nodeInfos)
	return overQuotaNodeInfos, nil
}

func (s *overQuotaSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	nodeInfos, err := s.SharedLister.NodeInfos().HavePodsWithAffinityList()
	if err != nil {
		return nil, err
	}
	overQuotaNodeInfos, _ := overquota.HookNodeInfosWithOverQuota(nodeInfos)
	return overQuotaNodeInfos, nil
}

func (s *overQuotaSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	nodeInfos, err := s.SharedLister.NodeInfos().HavePodsWithRequiredAntiAffinityList()
	if err != nil {
		return nil, err
	}
	overQuotaNodeInfos, _ := overquota.HookNodeInfosWithOverQuota(nodeInfos)
	return overQuotaNodeInfos, nil
}

func (s *overQuotaSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	nodeInfo, err := s.SharedLister.NodeInfos().Get(nodeName)
	if err != nil {
		return nil, err
	}
	overQuotaNodeInfo, _ := overquota.HookNodeInfoWithOverQuota(nodeInfo)
	return overQuotaNodeInfo, nil
}

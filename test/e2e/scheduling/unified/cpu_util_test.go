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
	"reflect"
	"testing"

	simgak8s "gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	v1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/swarm"
)

func TestCheckCPUIDNotDuplicated(t *testing.T) {
	tc := &testContext{
		caseName: "UT",
	}

	cpuIDInfo := make(map[int]simgak8s.CPUInfo)

	tests := []struct {
		allocatedResults             []*allocatedResult
		cpuIDInfoMap                 map[int]simgak8s.CPUInfo
		expectedContainerCheckResult bool
		expectedHostCheckResult      bool
	}{
		{
			// 3 个 allocatedResult，cpuset 均不重复，两个维度的检查（容器 & 宿主机）均通过
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 1, 2, 3},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{4, 5, 6, 7},
				},
				{
					index:    3,
					sn:       "UT-3",
					cpushare: false,
					cpuset:   []int{8, 9, 10, 11},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
			expectedHostCheckResult:      true,
		},
		{
			// 2 个 allocatedResult，容器维度 cpuset 不重复，宿主机维度 cpuset 有重复
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT",
					cpushare: false,
					cpuset:   []int{0, 1, 2, 3},
				},
				{
					index:    2,
					sn:       "UT",
					cpushare: false,
					cpuset:   []int{3, 4, 5, 6},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
			expectedHostCheckResult:      false,
		},
		{
			// 2 个 allocatedResult，容器维度 & 宿主机维度 cpuset 均有重复
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT",
					cpushare: false,
					cpuset:   []int{1, 1, 2, 3},
				},
				{
					index:    2,
					sn:       "UT",
					cpushare: false,
					cpuset:   []int{3, 4, 5, 6},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: false,
			expectedHostCheckResult:      false,
		},
	}

	for i, test := range tests {
		ret, _ := checkContainerCPUIDNotDuplicated(tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedContainerCheckResult {
			t.Errorf("Case[%d] checkContainerCPUIDNotDuplicated should pass", i)
		}

		ret, _ = checkHostCPUIdNotDuplicated(tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedHostCheckResult {
			t.Errorf("Case[%d] checkHostCPUIdNotDuplicated should pass", i)
		}
	}
}

func TestCheckCPUSetStrategy(t *testing.T) {
	tc := &testContext{
		caseName: "UT",
	}

	// 设置 CPU 拓扑，[0, 1], [2, 3], [4, 5], [6, 7]
	cpuIDInfo := map[int]simgak8s.CPUInfo{
		0: {
			CPUID:    0,
			CoreID:   0,
			SocketID: 0,
		},
		1: {
			CPUID:    1,
			CoreID:   0,
			SocketID: 0,
		},
		2: {
			CPUID:    2,
			CoreID:   1,
			SocketID: 0,
		},
		3: {
			CPUID:    3,
			CoreID:   1,
			SocketID: 0,
		},
		4: {
			CPUID:    4,
			CoreID:   2,
			SocketID: 1,
		},
		5: {
			CPUID:    5,
			CoreID:   2,
			SocketID: 1,
		},
		6: {
			CPUID:    6,
			CoreID:   3,
			SocketID: 1,
		},
		7: {
			CPUID:    7,
			CoreID:   3,
			SocketID: 1,
		},
	}

	tests := []struct {
		allocatedResults             []*allocatedResult
		cpuIDInfoMap                 map[int]simgak8s.CPUInfo
		strategy                     string
		expectedContainerCheckResult bool
		expectedHostCheckResult      bool
		expectedStrategyResult       bool
	}{
		{
			// 2 个 allocatedResult，spread 分布，检查通过
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 2, 4, 6},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{1, 3, 5, 7},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			strategy:                     "spread",
			expectedContainerCheckResult: true,
			expectedHostCheckResult:      true,
			expectedStrategyResult:       true,
		},
		{
			// 2 个 allocatedResult，非 spread 分布，检查不通过
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 1, 4, 6},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{2, 3, 5, 7},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			strategy:                     "spread",
			expectedContainerCheckResult: true,
			expectedHostCheckResult:      true,
			expectedStrategyResult:       false,
		},
		{
			// 2 个 allocatedResult，sameCoreFirst 分布，检查通过
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 1, 6, 7},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{2, 3, 4, 5},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			strategy:                     "sameCoreFirst",
			expectedContainerCheckResult: true,
			expectedHostCheckResult:      true,
			expectedStrategyResult:       true,
		},
		{
			// 2 个 allocatedResult，非 sameCoreFirst 分布，检查不通过
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 1, 4, 6},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{2, 3, 5, 7},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			strategy:                     "sameCoreFirst",
			expectedContainerCheckResult: true,
			expectedHostCheckResult:      true,
			expectedStrategyResult:       false,
		},
	}

	for i, test := range tests {
		ret, _ := checkContainerCPUIDNotDuplicated(tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedContainerCheckResult {
			t.Errorf("Case[%d] checkContainerCPUIDNotDuplicated should pass", i)
		}

		ret, _ = checkHostCPUIdNotDuplicated(tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedHostCheckResult {
			t.Errorf("Case[%d] checkHostCPUIdNotDuplicated should pass", i)
		}

		switch test.strategy {
		case "sameCoreFirst":
			ret, _ = checkContainerSameCoreFirst(tc, test.allocatedResults, test.cpuIDInfoMap)
			if ret != test.expectedStrategyResult {
				t.Errorf("Case[%d] checkContainerSameCoreFirst should pass", i)
			}
		case "spread":
			ret, _ = checkContainerSpread(tc, test.allocatedResults, test.cpuIDInfoMap)
			if ret != test.expectedStrategyResult {
				t.Errorf("Case[%d] checkContainerSpread should pass", i)
			}
		}
	}
}

func TestCheckHostCPUIdNotDuplicated(t *testing.T) {
	tc := &testContext{
		caseName: "UT",
	}
	tests := []struct {
		allocatedResults        []*allocatedResult
		cpuIDInfoMap            map[int]simgak8s.CPUInfo
		expectedHostCheckResult bool
	}{
		{
			// 容器 CPUID 不重合，测试通过
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 2, 4, 6},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{1, 3, 5, 7},
				},
			},
			expectedHostCheckResult: true,
		},
		{
			// 容器 CPUID 重合，测试失败
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: false,
					cpuset:   []int{0, 2, 4, 6},
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: false,
					cpuset:   []int{0, 3, 5, 7},
				},
			},
			expectedHostCheckResult: false,
		},
		{
			// 容器 cpushare=true，即使 cpuset 重合也测试成功
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					cpushare: true,
				},
				{
					index:    2,
					sn:       "UT-2",
					cpushare: true,
				},
			},
			expectedHostCheckResult: true,
		},
	}
	for i, test := range tests {
		ret, _ := checkHostCPUIdNotDuplicated(tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedHostCheckResult {
			t.Errorf("Case[%d] checkHostCPUIdNotDuplicated should %t", i, test.expectedHostCheckResult)
		}
	}
}

func TestCheckContainerShareCPUShouldBeNil(t *testing.T) {
	tcK8s := &testContext{
		caseName: "UT",
		testCases: []resourceCase{
			{
				requestType: requestTypeKubernetes,
			},
		},
	}
	// TODO: alos check sigma request type
	tests := []struct {
		tc                      *testContext
		allocatedResults        []*allocatedResult
		cpuIDInfoMap            map[int]simgak8s.CPUInfo
		expectedHostCheckResult bool
	}{
		{
			// K8s, cpushare 容器 cpuset 为空
			tc: tcK8s,
			allocatedResults: []*allocatedResult{
				{
					// MUST be 0 in this case
					index:    0,
					sn:       "UT-1",
					cpushare: true,
				},
			},
			expectedHostCheckResult: true,
		},
		{
			// K8s, cpushare 容器 cpuset 不为空
			tc: tcK8s,
			allocatedResults: []*allocatedResult{
				{
					index:    0,
					sn:       "UT-1",
					cpushare: true,
					cpuset:   []int{0, 2, 4, 6},
				},
			},
			expectedHostCheckResult: false,
		},
	}
	for i, test := range tests {
		ret, _ := checkContainerShareCPUShouldBeNil(test.tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedHostCheckResult {
			t.Errorf("Case[%d] checkContainerShareCPUShouldBeNil should %t", i, test.expectedHostCheckResult)
		}
	}
}

func TestCheckContainerCpuMutexCPUID(t *testing.T) {
	tc := &testContext{
		caseName: "UT",
		globalRule: &swarm.GlobalRules{
			CPUAllocatePolicy: &swarm.CPUAllocatePolicy{
				CPUMutexConstraint: &swarm.CPUMutexConstraint{
					Apps: []string{
						"app1", "app2",
					},
					ServiceUnits: []string{
						"du1", "du2",
					},
				},
			},
		},
	}

	// 设置 CPU 拓扑，[0, 1], [2, 3], [4, 5], [6, 7]
	cpuIDInfo := map[int]simgak8s.CPUInfo{
		0: {
			CPUID:    0,
			CoreID:   0,
			SocketID: 0,
		},
		1: {
			CPUID:    1,
			CoreID:   0,
			SocketID: 0,
		},
		2: {
			CPUID:    2,
			CoreID:   1,
			SocketID: 0,
		},
		3: {
			CPUID:    3,
			CoreID:   1,
			SocketID: 0,
		},
		4: {
			CPUID:    4,
			CoreID:   2,
			SocketID: 1,
		},
		5: {
			CPUID:    5,
			CoreID:   2,
			SocketID: 1,
		},
		6: {
			CPUID:    6,
			CoreID:   3,
			SocketID: 1,
		},
		7: {
			CPUID:    7,
			CoreID:   3,
			SocketID: 1,
		},
	}

	tests := []struct {
		tc                           *testContext
		allocatedResults             []*allocatedResult
		cpuIDInfoMap                 map[int]simgak8s.CPUInfo
		expectedContainerCheckResult bool
	}{
		{
			// cpu 互斥的应用物理核不重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					appName:  "app1",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					appName:  "app2",
					cpushare: false,
					cpuset:   []int{4, 6},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
		},
		{
			// cpu 互斥的应用物理核重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					appName:  "app1",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					appName:  "app2",
					cpushare: false,
					cpuset:   []int{1, 3},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: false,
		},
		{
			// 非 cpu 互斥的应用物理核重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					appName:  "app3",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					appName:  "app4",
					cpushare: false,
					cpuset:   []int{1, 3},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
		},
		{
			// 互斥与非 cpu 互斥的应用物理核重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					appName:  "app3",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					appName:  "app4",
					cpushare: false,
					cpuset:   []int{1, 3},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
		},
		{
			// cpu 互斥的应用物理核不重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					duName:   "du1",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					duName:   "du2",
					cpushare: false,
					cpuset:   []int{4, 6},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
		},
		{
			// cpu 互斥的应用物理核重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					duName:   "du1",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					duName:   "du2",
					cpushare: false,
					cpuset:   []int{1, 3},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: false,
		},
		{
			// 非 cpu 互斥的应用物理核重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					duName:   "du3",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					duName:   "du4",
					cpushare: false,
					cpuset:   []int{1, 3},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
		},
		{
			// 互斥与非 cpu 互斥的应用物理核重叠
			tc: tc,
			allocatedResults: []*allocatedResult{
				{
					index:    1,
					sn:       "UT-1",
					duName:   "du1",
					cpushare: false,
					cpuset:   []int{0, 2},
				},
				{
					index:    2,
					sn:       "UT-1",
					duName:   "du3",
					cpushare: false,
					cpuset:   []int{1, 3},
				},
			},
			cpuIDInfoMap:                 cpuIDInfo,
			expectedContainerCheckResult: true,
		},
	}
	for i, test := range tests {
		ret, err := checkContainerCpuMutexCPUID(test.tc, test.allocatedResults, test.cpuIDInfoMap)
		if ret != test.expectedContainerCheckResult {
			t.Errorf("Case[%d] checkContainerCpuMutexCPUID should be %t, err: %s", i, test.expectedContainerCheckResult, err)
		}
	}
}

func TestCheckAddOverQuotaPodSpecIfNeed(t *testing.T) {
	tc := &testContext{
		CPUOverQuotaRatio: 2.0,
	}

	tests := []struct {
		tc               *testContext
		podConfig        *pausePodConfig
		expectedAffinity *v1.Affinity
	}{
		{
			tc:        tc,
			podConfig: &pausePodConfig{},
			expectedAffinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      simgak8s.LabelEnableOverQuota,
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			tc: tc,
			podConfig: &pausePodConfig{
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "kubo.test",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"true"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAffinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "kubo.test",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
									{
										Key:      simgak8s.LabelEnableOverQuota,
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			tc: tc,
			podConfig: &pausePodConfig{
				Affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "kubo.test-1",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"1"},
										},
									},
								},
								{
									MatchExpressions: []v1.NodeSelectorRequirement{
										{
											Key:      "kubo.test-2",
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"2"},
										},
									},
								},
							},
						},
					},
				},
			},
			expectedAffinity: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "kubo.test-1",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"1"},
									},
									{
										Key:      simgak8s.LabelEnableOverQuota,
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
							{
								MatchExpressions: []v1.NodeSelectorRequirement{
									{
										Key:      "kubo.test-2",
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"2"},
									},
									{
										Key:      simgak8s.LabelEnableOverQuota,
										Operator: v1.NodeSelectorOpIn,
										Values:   []string{"true"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for i, test := range tests {
		addOverQuotaPodSpecIfNeed(test.tc, test.podConfig)
		if !reflect.DeepEqual(test.podConfig.Affinity, test.expectedAffinity) {
			t.Errorf("Case[%d] addOverQuotaPodSpecIfNeed should be %+v, but got: %+v", i, test.expectedAffinity, test.podConfig.Affinity)
		}
	}
}

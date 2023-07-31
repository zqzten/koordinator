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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	uniapi "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"

	"k8s.io/kubernetes/test/e2e_ak8s/swarm"

	"github.com/koordinator-sh/koordinator/test/e2e/framework"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling"
)

var _ = Describe("[e2e-ak8s][ak8s-scheduler][cpuset][cpu]", func() {
	var cs clientset.Interface
	var nodeList *v1.NodeList

	nodeToAllocatableMapCPU := make(map[string]int64)
	nodeToAllocatableMapMem := make(map[string]int64)
	nodeToAllocatableMapEphemeralStorage := make(map[string]int64)

	nodesInfo := make(map[string]*v1.Node)

	f := framework.NewDefaultFramework(CPUSetNameSpace)

	BeforeEach(func() {
		cs = f.ClientSet
		nodeList = &v1.NodeList{}

		masterNodes, nodeList = getMasterAndWorkerNodesOrDie(cs)

		for i, node := range nodeList.Items {
			waitNodeResourceReleaseComplete(node.Name)
			nodesInfo[node.Name] = &nodeList.Items[i]
			//etcdNodeinfo := swarm.GetNode(node.Name)
			//nodeToAllocatableMapCPU[node.Name] = int64(etcdNodeinfo.LocalInfo.CpuNum * 1000)
			{
				allocatable, found := node.Status.Allocatable[v1.ResourceCPU]
				Expect(found).To(Equal(true))
				nodeToAllocatableMapCPU[node.Name] = allocatable.Value() * 1000
			}
			{
				allocatable, found := node.Status.Allocatable[v1.ResourceMemory]
				Expect(found).To(Equal(true))
				nodeToAllocatableMapMem[node.Name] = allocatable.Value()
			}
			{
				allocatable, found := node.Status.Allocatable[v1.ResourceEphemeralStorage]
				Expect(found).To(Equal(true))
				nodeToAllocatableMapEphemeralStorage[node.Name] = allocatable.Value()
			}
		}
	})

	JustAfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			DumpSchedulerState(f, 0)
		}

	})

	// case描述：1.5超卖场景下的验证 CPUSet 分配成功
	// 步骤 要求每个容器分配的cpu个数不能低于2个，否则这个case会验证失败
	// 1.  1/2 整机核 k8s（预期成功）
	// 2.  1/2 整机核 k8s（预期成功）
	// 3.  1/2 整机核 k8s（预期成功）
	// 4.  1/2 整机核 k8s（预期失败）

	// 验证结果
	// 1. 每个容器的cpu核都不重叠 checkContainerCPUIDNotDuplicated
	// 2. 每个核的超卖比不大于2

	It("[p2] overQuotaCPUSetK8s001: ", func() {
		scheduling.WaitForStableCluster(cs, masterNodes)

		nodeName := GetNodeThatCanRunPod(f)
		Expect(nodeName).ToNot(BeNil())

		framework.Logf("get one node to schedule, nodeName: %s", nodeName)
		AllocatableCPU := nodeToAllocatableMapCPU[nodeName]
		AllocatableMemory := nodeToAllocatableMapMem[nodeName]
		AllocatableDisk := nodeToAllocatableMapEphemeralStorage[nodeName]

		requestedCPU := (AllocatableCPU / 1000) * 1000 / 2
		requestedMemory := AllocatableMemory / 8 //保证一定能扩容出来
		requestedDisk := AllocatableDisk / 8     //保证一定能扩容出来

		// get instanceID by node name
		instanceID := nodesInfo[nodeName].Labels[api.LabelECSInstanceID]
		localInfoString := nodesInfo[nodeName].Annotations[uniapi.AnnotationLocalInfo]
		Expect(localInfoString == "").ShouldNot(BeTrue(), fmt.Sprintf("nodeName:%s, localInfoString is empty", nodeName))
		localInfo := &api.LocalInfo{}
		if err := json.Unmarshal([]byte(localInfoString), localInfo); err != nil {
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("nodeName:%s, localInfoString:%v parse error", nodeName, localInfoString))
		}

		cpuOverQuotaRatio := 1.5

		tests := []resourceCase{
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				cpushare:        false,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				cpushare:        false,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
			},
			{
				cpu:             int64(float64(AllocatableCPU)*cpuOverQuotaRatio) - 2*requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				cpushare:        false,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: false,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
			},
		}
		testContext := &testContext{
			caseName:          "overQuotaCPUSetK8s001",
			cs:                cs,
			localInfo:         localInfo,
			f:                 f,
			testCases:         tests,
			CPUOverQuotaRatio: cpuOverQuotaRatio,
			nodeName:          nodeName,
		}
		testContext.execTests(
			checkContainerCPUIDNotDuplicated,
			checkCPUSetOverquotaRate,
		)

	})

	// case描述：1.25 超卖场景下的cpu互斥，app1和app2应用的 CPU 互斥，app3普通应用
	// 步骤 要求每个容器分配的cpu个数不能低于2个，否则这个case会验证失败
	// 1.  1/4 整机核 app1 k8s（预期成功）
	// 2.  1/4 整机核 app2 k8s（预期成功）
	// 3.  1/4 整机核 app3 k8s（预期成功）
	// 4.  1/4 整机核 app3 k8s（预期成功）
	// 5.  1/4 整机核 app3 k8s（预期成功）
	// 6.  1/4 整机核 app3 k8s（预期成功）
	// 验证结果
	// 1. app1的物理核不重叠， checkContainerCpuMutexCPUID
	// 2. app2的物理核不重叠， checkContainerCpuMutexCPUID
	// 3. app1和app2之间的物理核不重叠 checkHostCPUMutexCPUID
	// 4. 每个容器的逻辑核不重叠 checkContainerCPUIDNotDuplicated
	// 5. 每个核的超卖比不大于2 checkCpusetOverquotaRate
	It("[p3] overQuotaCPUSetK8s002: ", func() {
		Skip("not implemented")

		nodeName := GetNodeThatCanRunPod(f)
		Expect(nodeName).ToNot(BeNil())

		framework.Logf("get one node to schedule, nodeName: %s", nodeName)
		scheduling.WaitForStableCluster(cs, masterNodes)

		AllocatableCPU := nodeToAllocatableMapCPU[nodeName]
		AllocatableMemory := nodeToAllocatableMapMem[nodeName]
		AllocatableDisk := nodeToAllocatableMapEphemeralStorage[nodeName]

		requestedCPU := AllocatableCPU / 4
		requestedMemory := AllocatableMemory / 8 //保证一定能扩容出来
		requestedDisk := AllocatableDisk / 8     //保证一定能扩容出来

		// get ecsInstanceID by node name
		ecsInstanceID := nodesInfo[nodeName].Labels[api.LabelECSInstanceID]
		localInfoString := nodesInfo[nodeName].Annotations[uniapi.AnnotationLocalInfo]
		Expect(localInfoString == "").ShouldNot(BeTrue(), fmt.Sprintf("nodeName:%s, localInfoString is empty", nodeName))
		localInfo := &api.LocalInfo{}
		if err := json.Unmarshal([]byte(localInfoString), localInfo); err != nil {
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("nodeName:%s, localInfoString:%v parse error", nodeName, localInfoString))
		}

		// 必须更新 global rule
		globalRule := &swarm.GlobalRules{
			UpdateTime: time.Now().Format(time.RFC3339),
			CPUAllocatePolicy: &swarm.CPUAllocatePolicy{
				CPUMutexConstraint: &swarm.CPUMutexConstraint{
					Apps: []string{"app1", "app2"},
				},
			},
		}
		swarm.UpdateGlobalConfig(f.ClientSet, globalRule)

		tests := []resourceCase{
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app1"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {ecsInstanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app2"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {ecsInstanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app3"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {ecsInstanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				labels:          map[string]string{api.LabelAppName: "app3"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {ecsInstanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app3"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {ecsInstanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				labels:          map[string]string{api.LabelAppName: "app3"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {ecsInstanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
			},
		}
		testContext := &testContext{
			caseName:          "overQuotaCPUSetK8s002",
			cs:                cs,
			localInfo:         localInfo,
			f:                 f,
			globalRule:        globalRule,
			testCases:         tests,
			nodeName:          nodeName,
			CPUOverQuotaRatio: 1.25,
		}

		testContext.execTests(
			checkContainerCpuMutexCPUID,
			checkContainerCpuMutexCPUID,
			checkHostCPUMutexCPUID,
			checkContainerCPUIDNotDuplicated,
			checkCPUSetOverquotaRate,
		)
	})

	// case描述：1.25 超卖场景下的cpu独占，app1和app2应用的 cpu独占，app3普通应用
	// 步骤 要求每个容器分配的cpu个数不能低于2个，否则这个case会验证失败
	// 1.  1/2 整机核 app1 k8s（预期成功）
	// 2.  1/4 整机核 app2 k8s（预期成功）
	// 3.  1/2 整机核 app3 k8s（预期失败）
	// 4.  1/4 整机核 app3 k8s（预期成功）
	// 验证结果
	// 1. app1的cpu 全局唯一
	// 2. app2的cpu 全局唯一
	// 3. app1和app2之间的物理核不重叠 checkHostCPUMutexCPUID
	// 4. 每个容器的逻辑核不重叠 checkContainerCPUIDNotDuplicated
	// 5. 每个核的超卖比不大于2 checkCpusetOverquotaRate
	It("[p2] overQuotaCpusetK8s004: ", func() {
		Skip("not implemented")
	})
})

package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	sigmak8sapi "gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	uniapi "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"

	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/kubernetes/test/e2e/scheduling"
	"k8s.io/kubernetes/test/e2e_ak8s/swarm"
	"k8s.io/kubernetes/test/e2e_ak8s/util"
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
		// delete sigma2.0 allocplan if exists

	})

	// CPUSet 调度（非超卖）
	// k8s 非超卖的单链路节点上，绑核的操作不会重复使用相同的核，
	// 同一个核不会被超卖掉，即只能分配一次，资源回收之后，可以重复使用
	// 需要测试从 0->1->2->1->0 的复杂场景，
	// 并且各种绑核的策略需要交叉组合测试，以及策略降级的测试
	// 前置：集群中单节点上可分配的 CPU 资源大于 8

	// 步骤：
	// 1. 获取 NodeA 上可分配的 CPU 额度 X
	// 2. k8s 创建新的 PodA，设置 Requests.CPU = 1/2 * X、Limits.CPU = 1/2 * X，
	//    同时设置 Pod.Annotations.AllocSpec...CpuSet.SpreadStrategy="spread"，
	//    观察调度结果，获取 Pod 上分配的 CPUIDs 信息，以及 Node 上的 CPUSharePool 信息
	// 3. k8s 创建新的 PodB，设置 Requests.CPU = X - (1/2 * X)、Limits.CPU = X - (1/2 * X)，
	//    同时设置 Pod.Annotations.AllocSpec...CpuSet.SpreadStrategy="sameCoreFirst"，
	//    观察调度结果，获取 Pod 上分配的 CPUIDs 信息，以及 Node 上的 CPUSharePool 信息
	// 4. 删掉所有 Pod，使用 1/4 的 CPU 额度，创建 2 个 spread 的 Pod，
	//    再创建 2 个 sameCoreFirst 的 Pod，观察调度结果，
	//    获取 Pod 上分配的 CPUIDs 信息，以及 Node 上的 CPUSharePool 信息
	// 5. 删掉所有 Pod，使用 1/8 的 CPU 额度，如果不能整除，那么剩下的作为最后一个 Pod 额度，
	//    随机选择 SpreadStrategy，创建 9 个 Pod，
	//    观察调度结果，获取 Pod 上分配的 CPUIDs 信息，以及 Node 上的 CPUSharePool 信息

	// 验证结果：
	// 1. 第二步第三步 Pod 调度成功，SpreadStrategy 符合预期，
	//    并且每个核只能被分配一次，Node 的 CPUSharePool 得到更新，减去了以及做了绑核的 CPUIDs
	// 2. 第四步中 Pod 调度成功，SpreadStrategy 符合预期，并且每个核只能被分配一次，
	//    Node 的 CPUSharePool 得到更新，减去了以及做了绑核的 CPUIDs
	// 3. 第五步中前九个 Pod 调度成功，SpreadStrategy 不做检查，每个核只能被分配一次，
	//    Node 的 CPUSharePool 得到更新，减去了以及做了绑核的 CPUIDs，第 9 个 Pod 调度失败

	It("[smoke][p0][bvt] cpuset_k8s_case000 A pod with cpuset request should match the strategy, otherwise should down grade.", func() {
		nodeName := GetNodeThatCanRunPod(f)
		Expect(nodeName).ToNot(BeNil())

		framework.Logf("get one node to schedule, nodeName: %s", nodeName)

		// Apply node affinity label to each node
		nodeAffinityKey := "node-for-resource-e2e-test-" + string(uuid.NewUUID())
		framework.AddOrUpdateLabelOnNode(cs, nodeName, nodeAffinityKey, nodeName)
		defer framework.RemoveLabelOffNode(cs, nodeName, nodeAffinityKey)
		framework.ExpectNodeHasLabel(cs, nodeName, nodeAffinityKey, nodeName)

		localInfoString := nodesInfo[nodeName].Annotations[uniapi.AnnotationLocalInfo]
		Expect(localInfoString == "").ShouldNot(BeTrue(), fmt.Sprintf("nodeName:%s, localInfoString is empty", nodeName))
		localInfo := &api.LocalInfo{}
		if err := json.Unmarshal([]byte(localInfoString), localInfo); err != nil {
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("nodeName:%s, localInfoString:%v parse error", nodeName, localInfoString))
		}
		cpuIDInfoMap := make(map[int]api.CPUInfo, len(localInfo.CPUInfos))
		for _, cpuInfo := range localInfo.CPUInfos {
			cpuIDInfoMap[int(cpuInfo.CPUID)] = cpuInfo
		}

		node, err := f.ClientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		framework.ExpectNoError(err)
		quantity, ok := node.Status.Allocatable[v1.ResourceCPU]
		Expect(ok).To(BeTrue())
		allocatableCPU := quantity.Value()
		By("Request a pod with CPUSet strategy = spread.")

		// 做四次循环
		// 第一次，请求 1/2 的整机核，创建 "spread" 的容器 3 个，前两个成功，第三个失败
		// 第二次，请求 1/2 的整机核，创建 "sameCoreFirst" 的容器 3 个，前两个成功，第三个失败
		// 第三次，请求 1/4 的整机核，"spread"/"sameCoreFirst" 的容器各 2 个，均成功，再创建一个，失败
		// 第三次，请求 1/8 的整机核，"spread"/"sameCoreFirst" 随机选择，共创建 8 个，均成功，再创建一个，失败

		type CPUSetTestCase struct {
			CPURequest        int64
			SpreadStrategy    sigmak8sapi.SpreadStrategy
			SkipCheckStrategy bool
			ExpectedResult    bool
		}

		loopTime := 4
		for loop := 1; loop <= loopTime; loop++ {
			tests := []CPUSetTestCase{}

			cpuRequest := allocatableCPU / 2
			testPodCount := 2
			spreadStrategy := sigmak8sapi.SpreadStrategySpread
			skipCheckStrategy := false
			expectedResult := true

			// 第二次，请求 1/2 的整机核，创建 "sameCoreFirst" 的容器 3 个
			if loop == 2 {
				testPodCount = 2
				spreadStrategy = sigmak8sapi.SpreadStrategySameCoreFirst
				skipCheckStrategy = false
			}

			// 第三次，请求 1/4 的整机核，"spread"/"sameCoreFirst" 的容器各 2 个
			if loop == 3 {
				cpuRequest = allocatableCPU / 4
				testPodCount = 4
				skipCheckStrategy = false
			}

			// 第四次，请求 1/8 的整机核，"spread"/"sameCoreFirst" 随机选择
			// 在 ECS 上面最多创建成功 5 个，因此调小
			if loop == 4 {
				cpuRequest = allocatableCPU / 4
				testPodCount = 4
				skipCheckStrategy = true
			}

			for j := 1; j <= testPodCount; j++ {
				// 第三次，平分 SpreadStrategy
				if loop == 3 {
					spreadStrategy = sigmak8sapi.SpreadStrategySpread
					if j%2 == 0 {
						spreadStrategy = sigmak8sapi.SpreadStrategySameCoreFirst
					}
				}

				// 第三次，随机 SpreadStrategy
				if loop == 4 {
					spreadStrategies := []sigmak8sapi.SpreadStrategy{
						sigmak8sapi.SpreadStrategySpread,
						sigmak8sapi.SpreadStrategySameCoreFirst,
					}

					spreadStrategy = spreadStrategies[rand.Intn(len(spreadStrategies))]
				}

				caseItem := CPUSetTestCase{
					CPURequest:        cpuRequest,
					SpreadStrategy:    spreadStrategy,
					SkipCheckStrategy: skipCheckStrategy,
					ExpectedResult:    expectedResult,
				}
				tests = append(tests, caseItem)
			}

			lastCaseItem := CPUSetTestCase{
				CPURequest:        cpuRequest,
				SpreadStrategy:    spreadStrategy,
				SkipCheckStrategy: skipCheckStrategy,
				ExpectedResult:    false,
			}

			tests = append(tests, lastCaseItem)

			for index, test := range tests {
				framework.Logf("Run tests[%d][%d]: %+v", loop, index, test)
			}

			podsToDelete := make([]*v1.Pod, len(tests))
			// 用一个 map 来记录每个 CPUID 被分配了几次
			allocatedCPUIDCountMap := make(map[int]int)
			for i, test := range tests {
				name := "e2e-resource-k8s-cpuset-" + strconv.Itoa(i) + "-" + string(uuid.NewUUID())
				pod := createPausePod(f, pausePodConfig{
					Name: name,
					Labels: map[string]string{
						sigmak8sapi.LabelAppName:    "pod-app-name-for-resource-e2e-test",
						sigmak8sapi.LabelDeployUnit: "pod-deploy-unit-for-resource-e2e-test",
					},
					Annotations: map[string]string{
						sigmak8sapi.AnnotationPodAllocSpec: formatAllocSpecStringWithSpreadStrategy(name, test.SpreadStrategy),
					},
					Resources: &v1.ResourceRequirements{
						Requests: v1.ResourceList{
							v1.ResourceCPU: *resource.NewQuantity(test.CPURequest, "DecimalSI"),
						},
					},
					Affinity: util.GetAffinityNodeSelectorRequirement(nodeAffinityKey, []string{nodeName}),
				})

				if test.ExpectedResult == true {
					framework.Logf("Case[%d], expect pod to be scheduled successfully.", i)
					err := e2epod.WaitTimeoutForPodRunningInNamespace(cs, pod.Name, pod.Namespace, waitForPodRunningTimeout)
					podsToDelete = append(podsToDelete, pod)
					Expect(err).NotTo(HaveOccurred())

					// Get pod and check CPUIDs.
					podRunning, err := f.ClientSet.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
					Expect(err).NotTo(HaveOccurred())

					allocSpecStr := podRunning.Annotations[sigmak8sapi.AnnotationPodAllocSpec]
					allocSpec := &sigmak8sapi.AllocSpec{}
					err = json.Unmarshal([]byte(allocSpecStr), allocSpec)
					Expect(err).NotTo(HaveOccurred())

					CPUIDs := allocSpec.Containers[0].Resource.CPU.CPUSet.CPUIDs
					sort.Ints(CPUIDs)
					framework.Logf("Case[%d]Strategy[%s], CPUIDs: %v", i, test.SpreadStrategy, CPUIDs)

					checkResult := checkCPUSetSpreadStrategy(CPUIDs, cpuIDInfoMap, test.SpreadStrategy, test.SkipCheckStrategy)
					Expect(checkResult).Should(Equal(true), "checkCPUSetSpreadStrategy should pass")

					// 统计每个 CPUID 被分配的次数
					for _, cpuid := range CPUIDs {
						allocatedCPUIDCountMap[cpuid]++
					}

					for cpuid, count := range allocatedCPUIDCountMap {
						framework.Logf("Case[%d] allocatedCPUIDCountMap[%d], count: %d", i, cpuid, count)
						Expect(count).Should(Equal(1), "one cpuid should be allocated once and only once without over quota")
					}

					// TODO(kubo.cph): 这里可以顺便检查一下 Node 上面的 CPUSharePool 是不是动态调整了
				} else {
					framework.Logf("Case[%d], expect pod failed to be scheduled.", i)
					podsToDelete = append(podsToDelete, pod)
					err := e2epod.WaitForPodNameUnschedulableInNamespace(f.ClientSet, pod.Name, f.Namespace.Name)
					Expect(err).To(BeNil(), "expect err to be nil, got %s", err)
				}
			}

			// 已经分配的 CPUIDs 的核数，要正好等于可分配的核数，即每个 CPUID 都被分配了一次
			Expect(len(allocatedCPUIDCountMap)).Should(Equal(int(allocatableCPU)), "all cpuids should be allocated once.")

			wg := &sync.WaitGroup{}
			for _, pod := range podsToDelete {
				if pod == nil {
					continue
				}
				wg.Add(1)
				go func(pod *v1.Pod) {
					defer wg.Done()
					err := util.DeletePod(f.ClientSet, pod)
					Expect(err).NotTo(HaveOccurred(), "delete pod should succeed")
				}(pod)
			}
			wg.Wait()
		}
	})

	// 非超卖场景下的 SameCoreFirst，验证分配的物理核优先
	// 步骤 要求每个容器分配的cpu个数不能低于2个，否则这个case会验证失败
	// 1.  1/2 整机核 k8s（预期成功）
	// 2.  1/2 整机核 k8s（预期成功）
	// 3.  1/2 整机核 k8s（预期失败）

	// 验证结果
	// 1. 所有容器的cpu都不重叠
	// 2. 每个容器的cpu和都不重叠
	// 3. 每个容器的cpu的物理核*2=逻辑和
	It("[p2] cpusetK8s001: Pod with SameCoreFirst strategy, cpuset not overlap", func() {
		nodeName := GetNodeThatCanRunPod(f)
		Expect(nodeName).ToNot(BeNil())

		framework.Logf("get one node to schedule, nodeName: %s", nodeName)
		scheduling.WaitForStableCluster(cs, masterNodes)
		// Apply kubernetes node label to each node

		AllocatableCPU := nodeToAllocatableMapCPU[nodeName]
		AllocatableMemory := nodeToAllocatableMapMem[nodeName]
		AllocatableDisk := nodeToAllocatableMapEphemeralStorage[nodeName]

		requestedCPU := AllocatableCPU / 2
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

		tests := []resourceCase{
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				spreadStrategy:  "sameCoreFirst",
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				spreadStrategy:  "sameCoreFirst",
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: false,
				spreadStrategy:  "sameCoreFirst",
			},
		}
		testContext := &testContext{
			caseName:  "cpusetK8s001",
			cs:        cs,
			localInfo: localInfo,
			f:         f,
			testCases: tests,
			nodeName:  nodeName,
		}
		testContext.execTests(
			checkContainerCPUIDNotDuplicated,
			checkHostCPUIdNotDuplicated,
			checkContainerSameCoreFirst,
		)
	})

	// case描述：非超卖场景下的Spread，
	// 用于：验证分配的物理核优先
	// 步骤 要求每个容器分配的cpu个数不能低于2个，否则这个case会验证失败
	// 1.  1/2 整机核 k8s（预期成功）
	// 2.  1/2 整机核 k8s（预期成功）
	// 3.  1/2 整机核 k8s（预期失败）

	// 验证结果
	// 1. 所有容器的cpu都不重叠
	// 2. 每个容器的cpu和都不重叠
	// 3. 每个容器的cpu的物理核不重叠
	It("[p1] cpusetK8s002: Pod with Spread strategy, cpuset not overlap, physical core not overlap", func() {
		nodeName := GetNodeThatCanRunPod(f)
		Expect(nodeName).ToNot(BeNil())

		framework.Logf("get one node to schedule, nodeName: %s", nodeName)
		scheduling.WaitForStableCluster(cs, masterNodes)
		// Apply kubernetes node label to each node

		AllocatableCPU := nodeToAllocatableMapCPU[nodeName]
		AllocatableMemory := nodeToAllocatableMapMem[nodeName]
		AllocatableDisk := nodeToAllocatableMapEphemeralStorage[nodeName]

		requestedCPU := AllocatableCPU / 2
		requestedMemory := AllocatableMemory / 8 //保证一定能扩容出来
		requestedDisk := AllocatableDisk / 8     //保证一定能扩容出来

		// get nodeIP by node name
		instanceID := nodesInfo[nodeName].Labels[api.LabelECSInstanceID]
		localInfoString := nodesInfo[nodeName].Annotations[uniapi.AnnotationLocalInfo]
		Expect(localInfoString == "").ShouldNot(BeTrue(), fmt.Sprintf("nodeName:%s, localInfoString is empty", nodeName))
		localInfo := &api.LocalInfo{}
		if err := json.Unmarshal([]byte(localInfoString), localInfo); err != nil {
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("nodeName:%s, localInfoString:%v parse error", nodeName, localInfoString))
		}

		tests := []resourceCase{
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				spreadStrategy:  "spread",
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				spreadStrategy:  "spread",
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: false,
				spreadStrategy:  "spread",
			},
		}
		testContext := &testContext{
			caseName:  "cpusetK8s002",
			cs:        cs,
			localInfo: localInfo,
			f:         f,
			testCases: tests,
			nodeName:  nodeName,
		}

		testContext.execTests(
			checkContainerCPUIDNotDuplicated,
			checkHostCPUIdNotDuplicated,
			checkContainerSpread,
		)
	})

	// case描述：非超卖场景下的cpu互斥，app1和app2应用的 CPU 互斥，app3普通应用
	// 步骤 要求每个容器分配的cpu个数不能低于2个，否则这个case会验证失败
	// 1.  1/4 整机核 app1 k8s（预期成功）
	// 2.  1/4 整机核 app2 k8s（预期成功）
	// 3.  1/4 整机核 app3 k8s（预期成功）
	// 4.  1/4 整机核 app3 k8s（预期成功）

	// 验证结果
	// 1. app1的物理核不重叠，
	// 2. app2的物理核不重叠，
	// 3. app1和app2之间的物理核不重叠
	// 4. 每个容器的逻辑和不重叠
	// 5. 所有容器的逻辑和不重叠
	It("[p3] cpusetK8s003: Pod of appcontraints apps, cpu should not overlap", func() {
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

		// get instanceID by node name
		instanceID := nodesInfo[nodeName].Labels[api.LabelECSInstanceID]
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
		err := swarm.UpdateGlobalConfig(f.ClientSet, globalRule)
		defer swarm.RemoveGlobalRule(f.ClientSet)
		Expect(err).To(BeNil(), fmt.Sprintf("update globalrule failed with err %s\n", err))
		tests := []resourceCase{
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app1", api.LabelSite: "default"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app2", api.LabelSite: "default"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				spreadStrategy:  "spread",
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				labels:          map[string]string{api.LabelAppName: "app3", api.LabelSite: "default"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
			},
			{
				cpu:             requestedCPU,
				mem:             requestedMemory,
				ethstorage:      requestedDisk,
				labels:          map[string]string{api.LabelAppName: "app3", api.LabelSite: "default"},
				affinityConfig:  map[string][]string{api.LabelECSInstanceID: {instanceID}},
				requestType:     requestTypeKubernetes,
				shouldScheduled: true,
				spreadStrategy:  "spread",
			},
		}
		testContext := &testContext{
			caseName:   "cpusetK8s003",
			cs:         cs,
			localInfo:  localInfo,
			f:          f,
			globalRule: globalRule,
			testCases:  tests,
			nodeName:   nodeName,
		}

		testContext.execTests(
			checkContainerCPUIDNotDuplicated,
			checkHostCPUIdNotDuplicated,
			checkContainerCpuMutexCPUID,
			checkHostCPUMutexCPUID,
		)
	})
})

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
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	. "github.com/onsi/gomega"
	"gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/test/e2e/framework"
	e2epod "github.com/koordinator-sh/koordinator/test/e2e/framework/pod"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/env"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/swarm"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/util"
)

const (
	requestTypeKubernetes = "kubernetes"
	requestTypePreview    = "preview"
	cleanResource         = "cleanResource"

	CPUSetNameSpace = "ak8s-scheduler-cpuset"
)

type resourceItem struct {
	caseIndex int
	pod       *v1.Pod
	cpushare  bool // 标记预期是否CPUShare
	success   bool // 用于标记，是否调度成功
	beDeleted bool // 用于标记，资源是否被释放
}

type allocatedResult struct {
	index       int
	sn          string
	cpuset      []int
	cpushare    bool
	appName     string
	duName      string
	cpusetShare []int
}

type resourceCase struct {
	cpu                       int64
	mem                       int64
	ethstorage                int64
	cpushare                  bool
	mixrun                    bool    // mixrun is only for ant sigma2
	expectedAvailableResource []int64 // CPU/Memory/Disk
	requestType               string  // "kubernetes"
	shouldScheduled           bool
	spreadStrategy            string
	affinityConfig            map[string][]string
	labels                    map[string]string
	podConstraints            []constraint
	cleanIndexes              []int
	updateIndex               int
	previewCount              int
}

func parseFromPod(tc *testContext, resourceItem *resourceItem, p *v1.Pod) (error, *allocatedResult) {
	result := &allocatedResult{
		index:    resourceItem.caseIndex,
		sn:       p.Name,
		cpushare: resourceItem.cpushare,
	}

	if resourceItem.cpushare {
		if newNode, err := tc.cs.CoreV1().Nodes().Get(context.TODO(), p.Spec.NodeName, metav1.GetOptions{}); err == nil {
			sharePoolStr := newNode.Annotations[api.AnnotationNodeCPUSharePool]
			if sharePoolStr != "" {
				cpusharePool := &api.CPUSharePool{}
				if err := json.Unmarshal([]byte(sharePoolStr), cpusharePool); err != nil {
					return err, nil
				}
				if len(cpusharePool.CPUIDs) > 0 {
					cpusetShare := make([]int, 0, len(cpusharePool.CPUIDs))
					for _, cpuID := range cpusharePool.CPUIDs {
						cpusetShare = append(cpusetShare, int(cpuID))
					}
					result.cpusetShare = cpusetShare
				}
			} else {
				return fmt.Errorf("nodeName: %s, %s is empty", p.Spec.NodeName, api.AnnotationNodeCPUSharePool), nil
			}
		} else {
			return err, nil
		}
	}

	allocSpecRequestString := p.Annotations[api.AnnotationPodAllocSpec]
	if allocSpecRequestString != "" {
		spec := &api.AllocSpec{}
		err := json.Unmarshal([]byte(allocSpecRequestString), spec)
		if err != nil {
			bytes, _ := json.Marshal(p)
			return fmt.Errorf("pod:%s, err:%v", string(bytes), err), nil
		}
		result.cpuset = spec.Containers[0].Resource.CPU.CPUSet.CPUIDs
	}

	result.appName = p.Labels[api.LabelAppName]
	result.duName = p.Labels[api.LabelDeployUnit]
	return nil, result
}

func createAndVerifyK8sPod(tc *testContext, caseIndex int, test resourceCase) {
	var allocSpecRequest = &api.AllocSpec{}
	setAllocSpecRequest := false

	config := &pausePodConfig{
		Labels: test.labels,
		Name:   "scheduler-" + strconv.Itoa(caseIndex) + "-" + string(uuid.NewUUID()),
		Resources: &v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:              *resource.NewMilliQuantity(test.cpu, "DecimalSI"),
				v1.ResourceMemory:           *resource.NewQuantity(test.mem, "DecimalSI"),
				v1.ResourceEphemeralStorage: *resource.NewQuantity(test.ethstorage, "DecimalSI"),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:              *resource.NewMilliQuantity(test.cpu, "DecimalSI"),
				v1.ResourceMemory:           *resource.NewQuantity(test.mem, "DecimalSI"),
				v1.ResourceEphemeralStorage: *resource.NewQuantity(test.ethstorage, "DecimalSI"),
			},
		},
	}

	if !test.cpushare {
		allocSpecRequest.Containers = []api.Container{
			{
				Name: config.Name,
				Resource: api.ResourceRequirements{
					CPU: api.CPUSpec{
						CPUSet: &api.CPUSetSpec{
							SpreadStrategy: api.SpreadStrategy(test.spreadStrategy),
						},
					},
				},
			},
		}
		setAllocSpecRequest = true
	}

	if tc.globalRule != nil {
		if tc.globalRule.CPUAllocatePolicy != nil && tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint != nil {
			isCpuMutex := false
			for _, appName := range tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.Apps {
				if appName == test.labels[api.LabelAppName] {
					isCpuMutex = true
					break
				}
			}

			if isCpuMutex && !test.cpushare {
				allocSpecRequest.Affinity = &api.Affinity{
					CPUAntiAffinity: &api.CPUAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
							{
								Weight: 1,
								PodAffinityTerm: v1.PodAffinityTerm{
									TopologyKey: api.TopologyKeyPhysicalCore,
									LabelSelector: &metav1.LabelSelector{
										MatchExpressions: []metav1.LabelSelectorRequirement{
											{
												Key:      api.LabelAppName,
												Operator: metav1.LabelSelectorOpIn,
												Values:   tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.Apps,
											},
										},
									},
								},
							},
						},
					},
				}
				setAllocSpecRequest = true
			}
		}
	}

	if len(test.podConstraints) > 0 {
		podAffinityTerms := constrainsToPodAffinityTerms(test.podConstraints)
		if allocSpecRequest.Affinity == nil {
			allocSpecRequest.Affinity = &api.Affinity{}
		}
		if allocSpecRequest.Affinity.PodAntiAffinity == nil {
			allocSpecRequest.Affinity.PodAntiAffinity = &api.PodAntiAffinity{}
		}
		allocSpecRequest.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = podAffinityTerms
		setAllocSpecRequest = true
	}

	if setAllocSpecRequest {
		allocSpecBytes, _ := json.Marshal(&allocSpecRequest)
		config.Annotations = map[string]string{
			api.AnnotationPodAllocSpec: string(allocSpecBytes),
		}
	}
	if len(test.affinityConfig) > 0 {
		config.Affinity = util.GetAffinityNodeSelectorRequirementAndMap(test.affinityConfig)
	}
	addOverQuotaPodSpecIfNeed(tc, config)
	pod := createPausePod(tc.f, *config)
	if tc.resourceToDelete == nil {
		tc.resourceToDelete = map[int]*resourceItem{}
	}
	if test.shouldScheduled {
		err := util.WaitTimeoutForPodStatus(tc.cs, pod, v1.PodRunning, waitForPodRunningTimeout)
		Expect(err).To(BeNil())

		newPod, err := tc.cs.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		if err == nil && newPod != nil {
			pod = newPod
		} else {
			framework.Logf("refresh pod:%s err:%v", pod.Name, err)
		}
		framework.Logf("create pod:%+v", newPod)
		tc.resourceToDelete[caseIndex] = &resourceItem{caseIndex: caseIndex, pod: pod, success: err == nil, cpushare: test.cpushare}
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("CaseName: %s, Case[%d] Pod: %s, k8s, expect pod to be scheduled successfully.", tc.caseName, caseIndex, pod.Name))
		Expect(newPod.Spec.NodeName).NotTo(Equal(""),
			fmt.Sprintf("CaseName: %s, Case[%d] Pod: %s, k8s, expect pod to be scheduled successfully, but nodeName is empty.", tc.caseName, caseIndex, pod.Name))
	} else {
		err := e2epod.WaitForPodNameUnschedulableInNamespace(tc.cs, pod.Name, pod.Namespace)
		Expect(err).To(BeNil())

		tc.resourceToDelete[caseIndex] = &resourceItem{caseIndex: caseIndex, pod: pod, success: false, cpushare: test.cpushare}
		if len(tc.testCases) > 0 {
			Expect(err).To(BeNil(), "CaseName: %s, Case[%d], Pod: %s, k8s, expect pod failed to be scheduled. params: %+v, err: %v",
				tc.caseName, caseIndex, pod.Name, tc.testCases[caseIndex], err)
		} else {
			Expect(err).To(BeNil(), "CaseName: %s, Case[%d], Pod: %s, k8s, expect pod failed to be scheduled. params: %+v, err: %v",
				tc.caseName, caseIndex, pod.Name, nil, err)
		}
	}
}

func addOverQuotaPodSpecIfNeed(tc *testContext, config *pausePodConfig) {
	if tc.CPUOverQuotaRatio != 0 && tc.CPUOverQuotaRatio != 1 {
		nodeSeletor := v1.NodeSelectorRequirement{
			Key:      api.LabelEnableOverQuota,
			Operator: v1.NodeSelectorOpIn,
			Values:   []string{"true"},
		}

		addOverQuotaAffinity(config, nodeSeletor)
	}
}

func addOverQuotaAffinity(config *pausePodConfig, nodeSeletor v1.NodeSelectorRequirement) {
	if config.Affinity == nil {
		config.Affinity = &v1.Affinity{}
	}
	if config.Affinity.NodeAffinity == nil {
		config.Affinity.NodeAffinity = &v1.NodeAffinity{}
	}
	if config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &v1.NodeSelector{}
	}
	terms := config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if terms == nil {
		terms = []v1.NodeSelectorTerm{
			{
				MatchExpressions: []v1.NodeSelectorRequirement{nodeSeletor},
			},
		}
	} else {
		for i := range terms {
			terms[i].MatchExpressions = append(terms[i].MatchExpressions, nodeSeletor)
		}
	}

	config.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = terms
}

func cleanJob(tc *testContext) {
	//time.Sleep(3000*time.Second)
	if tc.globalRule != nil {
		swarm.RemoveGlobalRule(tc.f.ClientSet)
	}
	cleanContainers(tc, nil)
}

// 用于释放指定 case 的资源
func releaseResource(tc *testContext, caseIndexes []int) map[string]map[string]struct{} {
	caseIndexSet := map[int]bool{}
	for _, v := range caseIndexes {
		caseIndexSet[v] = true
	}
	hostSnToContainerSnSet := map[string]map[string]struct{}{}
	for caseIndex, p := range tc.resourceToDelete {
		if p.beDeleted {
			continue
		}
		if len(caseIndexes) > 0 && !caseIndexSet[caseIndex] {
			continue
		}
		if p.pod != nil {
			klog.Infof("k8s clean name: %s", p.pod.Name)
			err := util.DeletePod(tc.cs, p.pod)
			//err := tc.cs.CoreV1().Pods(p.pod.Namespace).Delete(p.pod.Name, nil)
			Expect(err).ShouldNot(HaveOccurred(), fmt.Sprintf("name: %s, when delete, err: %v", p.pod.Name, err))
			containerSnMap := hostSnToContainerSnSet[p.pod.Spec.NodeName]
			if containerSnMap == nil {
				containerSnMap = map[string]struct{}{}
			}
			containerSnMap[p.pod.Name] = struct{}{}
			hostSnToContainerSnSet[p.pod.Spec.NodeName] = containerSnMap
		}
		p.beDeleted = true
	}
	return hostSnToContainerSnSet
}

// 用于指定case的容器资源型销毁，并且等待资源释放干净
func cleanContainers(tc *testContext, caseIndexes []int) {
	hostSnToContainerSnMap := releaseResource(tc, caseIndexes)
	framework.Logf("cleanContainers caseName: %s, releaseResource ret: %+v", tc.caseName, hostSnToContainerSnMap)
	wg := &sync.WaitGroup{}
	errorMap := &sync.Map{}
	for hostSn, containerSnMap := range hostSnToContainerSnMap {
		wg.Add(1)
		go func(containerSnMap map[string]struct{}, hostSn string) {
			defer wg.Done()
			err := waitContainerDestroyed(tc, containerSnMap, hostSn)
			if err != nil {
				errorMap.Store(hostSn, err)
			}
		}(containerSnMap, hostSn)
	}
	wg.Wait()

	errorMap.Range(func(key, value interface{}) bool {
		Expect(value).To(BeNil(), fmt.Sprintf("tc.name:%s, wait hostSn:%s, release errorInfo:%v", tc.caseName, key, value))
		return true
	})
}

func waitContainerDestroyed(tc *testContext, containerSnMap map[string]struct{}, hostSn string) error {
	return wait.Poll(2*time.Second, 5*time.Minute, func() (done bool, err error) {
		podMap := swarm.GetHostPods(tc.f.ClientSet, hostSn)
		if len(podMap) == 0 {
			return true, nil
		}
		for containerSn := range containerSnMap {
			if podMap[containerSn] != nil {
				return false, nil
			}
		}
		return true, nil
	})
}

func waitNodeResourceReleaseComplete(hostSn string) {
	//if env.Tester == env.TesterJituan {
	//	wait.Poll(2*time.Second, 5*time.Minute, func() (done bool, err error) {
	//		return len(swarm.GetHostPod(hostSn)) == 0, nil
	//	})
	//}
}

type testContext struct {
	testCases            []resourceCase
	caseName             string
	resourceToDelete     map[int]*resourceItem
	cs                   clientset.Interface
	localInfo            *api.LocalInfo
	f                    *framework.Framework
	globalRule           *swarm.GlobalRules
	CPUOverQuotaRatio    float64
	MemoryOverQuotaRatio float64
	nodeName             string
	// 默认为 false，这种情况下，集团 sigma2.0 创建容器其实是 mock；如果设置为 true，则真实创建容器
	isSigmaNotMock bool
}

func (tc *testContext) execTests(checkFuncList ...checkFunc) {
	defer cleanJob(tc)
	// Apply over quota labels to each node
	if tc.CPUOverQuotaRatio != 0 && tc.CPUOverQuotaRatio != 1 {
		Expect(tc.nodeName).ShouldNot(BeEmpty(), "tc.nodeName is MUST required")
		framework.AddOrUpdateLabelOnNode(tc.cs, tc.nodeName, api.LabelEnableOverQuota, "true")
		defer framework.RemoveLabelOffNode(tc.cs, tc.nodeName, api.LabelEnableOverQuota)
		framework.ExpectNodeHasLabel(tc.cs, tc.nodeName, api.LabelEnableOverQuota, "true")

		framework.AddOrUpdateLabelOnNode(tc.cs, tc.nodeName, api.LabelCPUOverQuota, fmt.Sprintf("%v", tc.CPUOverQuotaRatio))
		defer framework.RemoveLabelOffNode(tc.cs, tc.nodeName, api.LabelCPUOverQuota)
		framework.ExpectNodeHasLabel(tc.cs, tc.nodeName, api.LabelCPUOverQuota, fmt.Sprintf("%v", tc.CPUOverQuotaRatio))
	}
	if tc.CPUOverQuotaRatio != 0 && tc.CPUOverQuotaRatio != 1 {
		// 2.0 给机器打超卖标
		// 只有 CPUOverQuota， 没有 MemoryOverQuota 时， MemoryOverQuotaRatio = 1
		if tc.MemoryOverQuotaRatio == 0 {
			tc.MemoryOverQuotaRatio = 1.0
		}

	} else {
		// 只有 MemoryOverQuota， 没有 CPUOverQuota 时， CPUOverQuotaRatio = 1
		if tc.MemoryOverQuotaRatio != 0 {
			tc.CPUOverQuotaRatio = 1.0
		}
	}

	if tc.MemoryOverQuotaRatio != 0 && tc.MemoryOverQuotaRatio != 1 {
		framework.AddOrUpdateLabelOnNode(tc.cs, tc.nodeName, api.LabelMemOverQuota, fmt.Sprintf("%v", tc.MemoryOverQuotaRatio))
		defer framework.RemoveLabelOffNode(tc.cs, tc.nodeName, api.LabelMemOverQuota)
		framework.ExpectNodeHasLabel(tc.cs, tc.nodeName, api.LabelMemOverQuota, fmt.Sprintf("%v", tc.MemoryOverQuotaRatio))
	}

	for i, test := range tc.testCases {
		framework.Logf("caseName: %v, caseIndex: %v, test: %+v", tc.caseName, i, test)
		switch test.requestType {
		case requestTypeKubernetes:
			createAndVerifyK8sPod(tc, i, test)

		case cleanResource:
			framework.Logf("caseName: %v, caseIndex: %v, cleanResource, indexes: %v", tc.caseName, i, test.cleanIndexes)
			cleanContainers(tc, test.cleanIndexes)
		}
		if len(checkFuncList) > 0 {
			checkResult(tc, checkFuncList)
		}
	}
}

func (tc *testContext) isCpuMono(result *allocatedResult) bool {
	if tc.globalRule == nil {
		return false
	}
	if tc.globalRule.CPUAllocatePolicy != nil && tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint != nil {
		for _, appName := range tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.Apps {
			if result.appName == appName {
				return true
			}
		}

		for _, duName := range tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.ServiceUnits {
			if result.duName == duName {
				return true
			}
		}
	}

	return false
}

func (tc *testContext) isCpuMutex(result *allocatedResult) bool {
	if tc.globalRule == nil {
		return false
	}

	if tc.globalRule.CPUAllocatePolicy != nil && tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint != nil {
		for _, appName := range tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.Apps {
			if result.appName == appName {
				return true
			}
		}

		for _, duName := range tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.ServiceUnits {
			if result.duName == duName {
				return true
			}
		}
	}
	return false
}

func checkResult(tc *testContext, checkFuncList []checkFunc) {
	// 验证结果工具
	allocatedResults := make([]*allocatedResult, 0, len(tc.resourceToDelete))
	for _, ri := range tc.resourceToDelete {
		if ri.success && !ri.beDeleted {
			if ri.pod != nil {
				if newPod, err := tc.cs.CoreV1().Pods(ri.pod.Namespace).Get(context.TODO(), ri.pod.Name, metav1.GetOptions{}); err == nil {
					if err, allocatedResult := parseFromPod(tc, ri, newPod); err == nil {
						allocatedResults = append(allocatedResults, allocatedResult)
					} else {
						Expect(err).ShouldNot(HaveOccurred())
					}
				} else {
					Expect(err).ShouldNot(HaveOccurred())
				}
			}
		}
	}
	cpuIDInfoMap := make(map[int]api.CPUInfo, len(tc.localInfo.CPUInfos))
	for _, cpuInfo := range tc.localInfo.CPUInfos {
		cpuIDInfoMap[int(cpuInfo.CPUID)] = cpuInfo
	}

	for _, checkFun := range checkFuncList {
		ret, msg := checkFun(tc, allocatedResults, cpuIDInfoMap)
		errorMsg := fmt.Sprintf("checkFun return err: %s", msg)
		if ret == false && env.Tester == env.TesterJituan && tc.nodeName != "" {
			podMap := swarm.GetHostPods(tc.f.ClientSet, tc.nodeName)
			bodys, _ := json.MarshalIndent(podMap, "", "\t")
			errorMsg += ",nodeName:" + tc.nodeName + "GetHostPod:" + string(bodys)
		}
		Expect(ret).Should(BeTrue(), errorMsg)
	}
}

type checkFunc = func(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string)

// 校验 cpushare 容器的 cpuset 为空
func checkContainerShareCPUShouldBeNil(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	for _, result := range cpuSetResults {
		if !result.cpushare {
			continue
		}
		switch tc.testCases[result.index].requestType {
		case requestTypeKubernetes:
			if !(len(result.cpuset) == 0) {
				return false, fmt.Sprintf("failed to check k8s checkContainerShareCPUShouldBeNil caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, should not be empty",
					tc.caseName, result.index, result.sn, result.cpuset)
			}
		}
	}

	return true, ""
}

// 校验容器的 sharePool == cpu.refCnt < math.ceil（超卖比）
func checkSharePool(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	if tc.CPUOverQuotaRatio == 0 {
		tc.CPUOverQuotaRatio = 1.0
	}

	maxRefCnt := int(math.Ceil(tc.CPUOverQuotaRatio))
	cpuIDRefCnt := map[int]int{}
	cpuIDMono := map[int]int{}
	for _, result := range cpuSetResults {
		cpuMono := tc.isCpuMono(result)
		if !result.cpushare {
			for _, cpuID := range result.cpuset {
				cpuIDRefCnt[cpuID] += 1
				if cpuMono {
					cpuIDMono[cpuID] += 1
				}
			}
		}
	}
	sharePool := make([]int, 0, len(cpuIDInfoMap))
	for cpuID := range cpuIDInfoMap {
		if cpuIDRefCnt[cpuID] < maxRefCnt && cpuIDMono[cpuID] == 0 {
			sharePool = append(sharePool, cpuID)
		}
	}
	sort.Ints(sharePool)

	for _, result := range cpuSetResults {
		if result.cpushare {
			sort.Ints(result.cpusetShare)
			if !reflect.DeepEqual(result.cpusetShare, sharePool) {
				return false, fmt.Sprintf("failed to check checkSharePool caseName: %s, caseIndex: %v, sn: %s, cpusetShare: %v, not equal expect sharePool: %+v",
					tc.caseName, result.index, result.sn, result.cpusetShare, sharePool)
			}
		}
	}

	return true, ""
}

// 校验容器的绑核规则符合 sameCoreFirst
func checkContainerSameCoreFirst(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	for _, cpuSetResult := range cpuSetResults {
		if cpuSetResult.cpushare {
			continue
		}
		coreIDSet := map[int32]bool{}
		for _, cpuID := range cpuSetResult.cpuset {
			info := cpuIDInfoMap[cpuID]
			coreIDSet[info.SocketID*1000+info.CoreID] = true
		}
		expectCoreNum := int(math.Ceil(float64(len(cpuSetResult.cpuset)) / 2.0))
		if !(len(coreIDSet) == expectCoreNum) {
			return false, fmt.Sprintf("failed to check checkContainerSameCoreFirst caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, check len(coreID)=expectCoreNum fail",
				tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset)
		}
	}

	return true, ""
}

// 校验容器的绑核规则符合 spread
func checkContainerSpread(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	for _, cpuSetResult := range cpuSetResults {
		if cpuSetResult.cpushare {
			continue
		}
		coreIDSet := map[int32]bool{}
		for _, cpuID := range cpuSetResult.cpuset {
			info := cpuIDInfoMap[cpuID]
			coreIDSet[info.SocketID*1000+info.CoreID] = true
		}
		if !(len(coreIDSet) == len(cpuSetResult.cpuset)) {
			return false, fmt.Sprintf("failed to check checkContainerSpread caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, check len(coreID)=len(cpuID) fail",
				tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset)
		}
	}
	return true, ""
}

// 校验是 CPU 互斥应用的容器物理核都不重叠
func checkContainerCpuMutexCPUID(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	if tc.globalRule == nil || len(tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.Apps) == 0 {
		return true, ""
	}

	coreIDMap := map[int32][]int{}
	for _, result := range cpuSetResults {
		if result.cpushare {
			continue
		}
		if !tc.isCpuMutex(result) {
			continue
		}
		for _, cpuID := range result.cpuset {
			info := cpuIDInfoMap[cpuID]
			coreID := info.SocketID*1000 + info.CoreID
			if coreIDMap[coreID] != nil && (len(coreIDMap[coreID]) != 0) {
				return false, fmt.Sprintf("checkContainerCpuMutexCPUID caseName:%s, caseIndex:%v, sn:%s, cpuset:%v, duplicateCore:%v, cpuIDs:%v",
					tc.caseName, result.index, result.sn, result.cpuset, coreID, append(coreIDMap[coreID], cpuID))
			}
			coreIDMap[coreID] = []int{cpuID}
		}
	}

	return true, ""
}

// 校验宿主机上的 CPU 互斥应用的物理核都不重叠
func checkHostCPUMutexCPUID(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	if tc.globalRule == nil || len(tc.globalRule.CPUAllocatePolicy.CPUMutexConstraint.Apps) == 0 {
		return true, ""
	}
	coreIDCPUIDMap := map[int32][]int{}
	coreIDSnMap := map[int32]*allocatedResult{}
	for _, result := range cpuSetResults {
		if result.cpushare {
			continue
		}
		if !tc.isCpuMutex(result) {
			continue
		}
		for _, cpuID := range result.cpuset {
			info := cpuIDInfoMap[cpuID]
			coreID := info.SocketID*1000 + info.CoreID
			if coreIDCPUIDMap[coreID] != nil && !(coreIDCPUIDMap[coreID] == nil) {
				return false, fmt.Sprintf("checkHostCPUMutexCPUID caseName:%s, caseIndex:%v, sn:%s, cpuset:%v, sn:%s, cpuset:%v, duplicate at coreID:%v",
					tc.caseName, result.index, result.sn, result.cpuset, coreIDSnMap[coreID].sn, coreIDSnMap[coreID].cpuset, coreID)
			}
			coreIDCPUIDMap[coreID] = []int{cpuID}
			coreIDSnMap[coreID] = result
		}
	}
	return true, ""
}

// 校验宿主机上的所有容器的逻辑核都不重叠
func checkHostCPUIdNotDuplicated(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	hostCPUIDCount := map[int]int{}
	for _, cpuSetResult := range cpuSetResults {
		if cpuSetResult.cpushare {
			continue
		}
		for _, cpuID := range cpuSetResult.cpuset {
			hostCPUIDCount[cpuID]++
		}
	}

	for cpuID, count := range hostCPUIDCount {
		// 非超卖场景，每个核最多只被分配一次
		// TODO(kubo.cph), 可以引入超卖比，判断每个核被分配的次数不超过超卖比
		if !(count == 1) {
			return false, fmt.Sprintf("failed to checkHostCPUIdNotDuplicated caseName: %s, cpuID: %v, duplicated on host",
				tc.caseName, cpuID)
		}
	}

	return true, ""
}

// 校验每个容器的核分配策略正确
func checkContainerCPUStrategyRightWithSameSocket(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	for _, cpuSetResult := range cpuSetResults {
		if cpuSetResult.cpushare {
			continue
		}
		strategy := tc.testCases[cpuSetResult.index].spreadStrategy
		coreIDMap := map[int32]int{}
		socketIDMap := map[int32]int{}
		if strategy == "spread" || strategy == "default" {
			for _, cpuID := range cpuSetResult.cpuset {
				info := cpuIDInfoMap[cpuID]
				coreID := info.SocketID*1000 + info.CoreID
				if _, ok := coreIDMap[coreID]; ok {
					return false, fmt.Sprintf("failed to checkContainerCPUStrategyRight caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, cpuID: %v, not spread.",
						tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset, cpuID)
				}
				coreIDMap[coreID] += 1
				socketIDMap[info.SocketID] += 1
			}
			if len(socketIDMap) > 1 {
				return false, fmt.Sprintf("failed to checkContainerCPUStrategyRight caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, socketID: %+v, not same socket.",
					tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset, socketIDMap)
			}
		} else if strategy == "sameCoreFirst" {
			for _, cpuID := range cpuSetResult.cpuset {
				info := cpuIDInfoMap[cpuID]
				coreID := info.SocketID*1000 + info.CoreID
				coreIDMap[coreID] += 1
				socketIDMap[info.SocketID] += 1
			}
			for cpuID, coreCount := range coreIDMap {
				if coreCount < 2 {
					return false, fmt.Sprintf("failed to checkContainerCPUStrategyRight caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, cpuID: %v, not sameCore.",
						tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset, cpuID)
				}
			}
			if len(socketIDMap) > 1 {
				return false, fmt.Sprintf("failed to checkContainerCPUStrategyRight caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, socketID: %+v, not same socket.",
					tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset, socketIDMap)
			}
		}
	}
	return true, ""
}

// 校验每个容器的逻辑核不重叠
func checkContainerCPUIDNotDuplicated(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	for _, cpuSetResult := range cpuSetResults {
		if cpuSetResult.cpushare {
			continue
		}
		cpuIDSet := map[int]int{}
		for _, cpuID := range cpuSetResult.cpuset {
			cpuIDSet[cpuID]++
		}

		for _, cpuID := range cpuSetResult.cpuset {
			if !(cpuIDSet[cpuID] == 1) {
				return false, fmt.Sprintf("failed to checkContainerCPUIDNotDuplicated caseName: %s, caseIndex: %v, sn: %s, cpuset: %v, cpuID: %v, duplicated on container",
					tc.caseName, cpuSetResult.index, cpuSetResult.sn, cpuSetResult.cpuset, cpuID)
			}
		}
	}
	return true, ""
}

// 校验每个核超卖比不超过 math.ceil(overquotaratio)
func checkCPUSetOverquotaRate(tc *testContext, cpuSetResults []*allocatedResult, cpuIDInfoMap map[int]api.CPUInfo) (bool, string) {
	if tc.CPUOverQuotaRatio == 0 {
		tc.CPUOverQuotaRatio = 1.0
	}

	maxRefCnt := int(math.Ceil(tc.CPUOverQuotaRatio))

	for _, result := range cpuSetResults {
		if result.cpushare {
			continue
		}

		coreIDMap := map[int32]int{}
		for _, cpuID := range result.cpuset {
			info := cpuIDInfoMap[cpuID]
			coreID := info.SocketID*1000 + info.CoreID
			coreIDMap[coreID] += 1
		}

		totalUsedCores := 0
		for _, cpuID := range result.cpuset {
			info := cpuIDInfoMap[cpuID]
			coreID := info.SocketID*1000 + info.CoreID
			totalUsedCores += coreIDMap[coreID]
			if coreIDMap[coreID] > maxRefCnt {
				return false, fmt.Sprintf("checkCpusetOverquotaRate caseName:%s, caseIndex:%v, sn:%s, cpuset:%v, OverquotaCore:%v, cpuIDs:%v",
					tc.caseName, result.index, result.sn, result.cpuset, coreID, cpuID)
			}
		}

		// 上面的检查只查了每个核的数目不超过 ceil(CPUOverQuotaRatio)
		// 检查 cpu-over-quota 是小数时，是否只使用了超卖数目以内的核数，通过使用的核数是否超过超卖后的总核数判断
		if int64(totalUsedCores) > int64(math.Floor(float64(len(tc.localInfo.CPUInfos))*tc.CPUOverQuotaRatio)) {
			return false, fmt.Sprintf("checkCpusetOverquotaRate failed.")
		}
	}
	return true, ""
}

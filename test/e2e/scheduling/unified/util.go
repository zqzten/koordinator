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
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	sigmak8sapi "gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	v1helper "k8s.io/component-helpers/scheduling/corev1"
	"k8s.io/klog/v2"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	"github.com/koordinator-sh/koordinator/test/e2e/framework"
	e2enode "github.com/koordinator-sh/koordinator/test/e2e/framework/node"
	e2epod "github.com/koordinator-sh/koordinator/test/e2e/framework/pod"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/env"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/swarm"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling/unified/util"
	imageutils "github.com/koordinator-sh/koordinator/test/utils/image"
)

// variable set in BeforeEach, never modified afterwards
var masterNodes sets.String

// the timeout of waiting for pod running.
var waitForPodRunningTimeout = 5 * time.Minute

const containerPrefix = "container-"

type pausePodConfig struct {
	Name                              string
	Affinity                          *v1.Affinity
	Annotations, Labels, NodeSelector map[string]string
	Resources                         *v1.ResourceRequirements
	Tolerations                       []v1.Toleration
	NodeName                          string
	Ports                             []v1.ContainerPort
	OwnerReferences                   []metav1.OwnerReference
	PriorityClassName                 string
	ResourcesForMultiContainers       []v1.ResourceRequirements
}

func initPausePod(f *framework.Framework, conf pausePodConfig) *v1.Pod {
	pauseImage := imageutils.GetPauseImageName()

	if conf.Labels == nil {
		conf.Labels = make(map[string]string)
	}

	if _, ok := conf.Labels[sigmak8sapi.LabelInstanceGroup]; !ok {
		conf.Labels[sigmak8sapi.LabelInstanceGroup] = "scheduler-e2e-instance-group"
	}

	if _, ok := conf.Labels[sigmak8sapi.LabelSite]; !ok {
		conf.Labels[sigmak8sapi.LabelSite] = "scheduler-e2e-site"
	}

	if _, ok := conf.Labels[sigmak8sapi.LabelAppName]; !ok {
		conf.Labels[sigmak8sapi.LabelAppName] = "scheduler-e2e-app-name"
	}

	if _, ok := conf.Labels[sigmak8sapi.LabelDeployUnit]; !ok {
		conf.Labels[sigmak8sapi.LabelDeployUnit] = "scheduler-e2e-depoly-unit"
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            conf.Name,
			Labels:          conf.Labels,
			Annotations:     conf.Annotations,
			OwnerReferences: conf.OwnerReferences,
		},
		Spec: v1.PodSpec{
			NodeSelector: conf.NodeSelector,
			Affinity:     conf.Affinity,
			Containers: []v1.Container{
				{
					Name:  conf.Name,
					Image: pauseImage,
					Ports: conf.Ports,
				},
			},
			Tolerations:       conf.Tolerations,
			NodeName:          conf.NodeName,
			PriorityClassName: conf.PriorityClassName,
		},
	}
	if conf.Resources != nil {
		pod.Spec.Containers[0].Resources = *conf.Resources
	}

	if len(conf.ResourcesForMultiContainers) > 0 {
		pod.Spec.Containers = []v1.Container{}
	}

	for i, r := range conf.ResourcesForMultiContainers {
		c := v1.Container{
			Name:      containerPrefix + strconv.Itoa(i),
			Image:     pauseImage,
			Ports:     conf.Ports,
			Resources: r,
		}
		pod.Spec.Containers = append(pod.Spec.Containers, c)
	}

	if env.GetTester() == env.TesterJituan {
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, v1.Toleration{
			Key:      sigmak8sapi.LabelResourcePool,
			Operator: v1.TolerationOpEqual,
			Value:    "sigma_public",
			Effect:   v1.TaintEffectNoSchedule,
		})
		pod.Spec.Tolerations = append(pod.Spec.Tolerations, v1.Toleration{
			Key:      sigmak8sapi.LabelIsECS,
			Operator: v1.TolerationOpExists,
			Effect:   v1.TaintEffectNoSchedule,
		})
	}

	return pod
}

func createPausePod(f *framework.Framework, conf pausePodConfig) *v1.Pod {
	pausePod := initPausePod(f, conf)
	if len(pausePod.Name) > 50 {
		pausePod.Name = pausePod.Name[:49] + "a"
	}
	pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), pausePod, metav1.CreateOptions{})
	framework.ExpectNoError(err)
	return pod
}

func runPausePod(f *framework.Framework, conf pausePodConfig) *v1.Pod {
	pod := createPausePod(f, conf)
	framework.ExpectNoError(e2epod.WaitForPodRunningInNamespace(f.ClientSet, pod))
	pod, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Get(context.TODO(), conf.Name, metav1.GetOptions{})
	framework.ExpectNoError(err)
	return pod
}

func runPodAndGetNodeName(f *framework.Framework, conf pausePodConfig) string {
	// launch a pod to find a node which can launch a pod. We intentionally do
	// not just take the node list and choose the first of them. Depending on the
	// cluster and the scheduler it might be that a "normal" pod cannot be
	// scheduled onto it.
	pod := runPausePod(f, conf)

	By("Explicitly delete pod here to free the resource it takes.")
	err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Delete(context.TODO(), pod.Name, *metav1.NewDeleteOptions(0))
	framework.ExpectNoError(err)

	return pod.Spec.NodeName
}

func getRequestedCPU(pod v1.Pod) int64 {
	var result int64
	for _, container := range pod.Spec.Containers {
		result += container.Resources.Requests.Cpu().MilliValue()
	}
	return result
}

func getRequestedMem(pod v1.Pod) int64 {
	var result int64
	for _, container := range pod.Spec.Containers {
		result += container.Resources.Requests.Memory().Value()
	}
	return result
}

func getRequestedStorageEphemeralStorage(pod v1.Pod) int64 {
	var result int64
	for _, container := range pod.Spec.Containers {
		result += container.Resources.Requests.StorageEphemeral().Value()
	}
	return result
}

func getPodsByLabels(c clientset.Interface, ns string, labelsMap map[string]string) *v1.PodList {
	selector := labels.SelectorFromSet(labels.Set(labelsMap))
	allPods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	framework.ExpectNoError(err)
	return allPods
}

// GetNodeThatCanRunPod return a nodename that can run pod.
func GetNodeThatCanRunPod(f *framework.Framework) string {
	By("Trying to launch a pod without a label to get a node which can launch it.")

	node, err := GetSmallestNumberOfPodNodes(f, nil)
	framework.ExpectNoError(err)
	framework.Logf("smallest node=%s, kubernetes.io/hostname=%s", node.Name, node.Labels[sigmak8sapi.LabelHostname])
	nodeName := runPodAndGetNodeName(f, pausePodConfig{
		Name:     "without-label",
		Affinity: util.GetAffinityNodeSelectorRequirement(sigmak8sapi.LabelHostname, []string{node.Labels[sigmak8sapi.LabelHostname]}),
	})
	return nodeName
}

// GetSmallestNumberOfPodNodes return the node that has the smallest number of pod
func GetSmallestNumberOfPodNodes(f *framework.Framework, excludeNodes []string) (*v1.Node, error) {
	nodeList, err := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	sort.Slice(nodeList.Items, func(i, j int) bool {
		return nodeList.Items[i].Name < nodeList.Items[j].Name
	})
	var targetNode v1.Node
	var smallestNodeName string
	var smallest int
findLoop:
	for _, node := range nodeList.Items {
		if !isNodeSchedulable(&node) {
			framework.Logf("node %s is unschedulable", node.Name)
			continue
		}
		for _, nodeName := range excludeNodes {
			if node.Name == nodeName {
				continue findLoop
			}
		}
		if node.Labels[sigmak8sapi.LabelResourcePool] != "sigma_public" {
			continue
		}

		allocPlans := swarm.GetHostPods(f.ClientSet, node.Name)
		if allocPlans == nil {
			continue
		}
		if smallest == 0 || smallest > len(allocPlans) {
			smallest = len(allocPlans)
			smallestNodeName = node.Name
			node.DeepCopyInto(&targetNode)
		}
	}
	framework.Logf("smallestNodeName=%s", smallestNodeName)
	return &targetNode, nil
}

func getNodeThatCanRunPodWithoutToleration(f *framework.Framework) string {
	By("Trying to launch a pod without a toleration to get a node which can launch it.")
	return runPodAndGetNodeName(f, pausePodConfig{Name: "without-toleration"})
}

// create pod which using hostport on the specified node according to the nodeSelector
func creatHostPortPodOnNode(f *framework.Framework, podName, ns, hostIP string, port int32, protocol v1.Protocol, nodeSelector map[string]string, expectScheduled bool) {
	createPausePod(f, pausePodConfig{
		Name: podName,
		Ports: []v1.ContainerPort{
			{
				HostPort:      port,
				ContainerPort: 80,
				Protocol:      protocol,
				HostIP:        hostIP,
			},
		},
		NodeSelector: nodeSelector,
	})

	err := e2epod.WaitForPodNotPending(f.ClientSet, ns, podName)
	if expectScheduled {
		framework.ExpectNoError(err)
	}
}

func scheduleSuccessEvent(podName, nodeName string) func(*v1.Event) bool {
	return func(e *v1.Event) bool {
		return e.Type == v1.EventTypeNormal &&
			e.Reason == "Scheduled" &&
			strings.HasPrefix(e.Name, podName) &&
			strings.Contains(e.Message, fmt.Sprintf("Successfully assigned %v to %v", podName, nodeName))
	}
}

func scheduleFailureEvent(podName string) func(*v1.Event) bool {
	return func(e *v1.Event) bool {
		return strings.HasPrefix(e.Name, podName) &&
			e.Type == "Warning" &&
			e.Reason == "FailedScheduling"
	}
}

// getAvailableResourceOnNode return available cpu/memory/disk size of this node.
// TODO, we need to take sigma node into account.
func getAvailableResourceOnNode(f *framework.Framework, nodeName string) []int64 {
	node, err := f.ClientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	framework.ExpectNoError(err)

	var availableCPU, availableMemory, availableDisk int64 = 0, 0, 0

	allocatableCPU, found := node.Status.Allocatable[v1.ResourceCPU]
	Expect(found).To(Equal(true))
	availableCPU = allocatableCPU.MilliValue()

	allocatableMomory, found := node.Status.Allocatable[v1.ResourceMemory]
	Expect(found).To(Equal(true))
	availableMemory = allocatableMomory.Value()

	allocatableDisk, found := node.Status.Allocatable[v1.ResourceEphemeralStorage]
	Expect(found).To(Equal(true))
	availableDisk = allocatableDisk.Value()

	pods, err := f.ClientSet.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{
		FieldSelector: fields.Set{
			"spec.nodeName": nodeName,
		}.AsSelector().String(),
	})
	framework.ExpectNoError(err)

	for _, pod := range pods.Items {
		if pod.Status.Phase != v1.PodSucceeded && pod.Status.Phase != v1.PodFailed {
			availableCPU -= getRequestedCPU(pod)
			availableMemory -= getRequestedMem(pod)
			availableDisk -= getRequestedStorageEphemeralStorage(pod)
		}
	}

	return []int64{availableCPU, availableMemory, availableDisk}
}

func formatAllocSpecStringWithSpreadStrategyForMultiContainers(name string, strategy sigmak8sapi.SpreadStrategy, count int) string {
	allocSpecRequest := &sigmak8sapi.AllocSpec{
		Containers: []sigmak8sapi.Container{},
	}

	for i := 0; i < count; i++ {
		c := sigmak8sapi.Container{
			Name: containerPrefix + strconv.Itoa(i),
			Resource: sigmak8sapi.ResourceRequirements{
				CPU: sigmak8sapi.CPUSpec{
					CPUSet: &sigmak8sapi.CPUSetSpec{
						SpreadStrategy: strategy,
					},
				},
			},
		}
		allocSpecRequest.Containers = append(allocSpecRequest.Containers, c)
	}

	allocSpecBytes, err := json.Marshal(&allocSpecRequest)
	if err != nil {
		return ""
	}

	return string(allocSpecBytes)
}

func formatAllocSpecStringWithSpreadStrategy(name string, strategy sigmak8sapi.SpreadStrategy) string {
	allocSpecRequest := &sigmak8sapi.AllocSpec{
		Containers: []sigmak8sapi.Container{
			{
				Name: name,
				Resource: sigmak8sapi.ResourceRequirements{
					CPU: sigmak8sapi.CPUSpec{
						CPUSet: &sigmak8sapi.CPUSetSpec{
							SpreadStrategy: strategy,
						},
					},
				},
			},
		},
	}

	allocSpecBytes, err := json.Marshal(&allocSpecRequest)
	if err != nil {
		return ""
	}

	return string(allocSpecBytes)
}

// checkCPUSetSpreadStrategy check the given CPUIDs whether match the given strategy.
func checkCPUSetSpreadStrategy(cpuIDs []int,
	cpuIDInfoMap map[int]sigmak8sapi.CPUInfo,
	strategy sigmak8sapi.SpreadStrategy, skip bool) bool {
	if skip {
		return true
	}

	sort.Ints(cpuIDs)

	switch strategy {
	case sigmak8sapi.SpreadStrategySameCoreFirst:
		coreIDSet := map[int32]bool{}
		for _, cpuID := range cpuIDs {
			info := cpuIDInfoMap[cpuID]
			coreIDSet[info.SocketID*1000+info.CoreID] = true
		}
		expectCoreNum := int(math.Ceil(float64(len(cpuIDs)) / 2.0))
		if !(len(coreIDSet) == expectCoreNum) {
			framework.Logf("PhysicalCoreID[%d] has %d processors, not match spread strategy.", len(coreIDSet), expectCoreNum)
			return false
		}
		return true
	case sigmak8sapi.SpreadStrategySpread:
		coreIDSet := map[int32]bool{}
		for _, cpuID := range cpuIDs {
			info := cpuIDInfoMap[cpuID]
			coreIDSet[info.SocketID*1000+info.CoreID] = true
		}
		if !(len(coreIDSet) == len(cpuIDs)) {
			framework.Logf("PhysicalCoreID[%d] has %d processors, not match spread strategy.", len(coreIDSet), len(cpuIDs))
			return false
		}
		return true
	default:
		return false
	}
}

// 检查cpu overquota 下总核数不超过上限，每个核的使用次数不超过 overquota 允许的个数，例如：
// overquota = 2 时， 每个核不超过 2 次
// overquota = 1.5 时， 有一半的核不超过 2 次，剩下的不超过 1 次
func checkCPUOverQuotaCoreBinding(processorIDToCntMap map[int]int, cpuTotalNum int, overQuota float64) bool {
	overQuotaCpuNum := 0
	for _, cnt := range processorIDToCntMap {
		if cnt > int(math.Ceil(overQuota)) {
			return false
		} else {
			overQuotaCpuNum += cnt
		}
	}

	if overQuotaCpuNum > cpuTotalNum*int(math.Ceil(overQuota)) {
		return false
	}
	return true
}

// if cnt==0, print unlimit nodes
func DumpSchedulerState(f *framework.Framework, cnt int) {
	nodes, _ := f.ClientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if nodes == nil {
		return
	}

	if cnt < 0 {
		cnt = 0
	}
	if cnt > len(nodes.Items) {
		cnt = len(nodes.Items)
	}
	for index, node := range nodes.Items {
		if index > cnt && cnt != 0 {
			break
		}
		allocplan := swarm.GetHostPods(f.ClientSet, node.Name)
		klog.Infof("******** Output of scheduler allocplan")
		tmp, _ := json.MarshalIndent(allocplan, "  ", "  ")
		if tmp != nil {
			fmt.Println(string(tmp))
		} else {
			fmt.Println("allocplan is nil")
		}
	}
}

// the same fuction as in framwork, except that skip the taint node check.
func getMasterAndWorkerNodesOrDie(c clientset.Interface) (sets.String, *v1.NodeList) {
	nodes := &v1.NodeList{}
	masters := sets.NewString()
	all, err := c.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	Expect(err).To(BeNil())
	for _, n := range all.Items {
		if DeprecatedMightBeMasterNode(n.Name) {
			masters.Insert(n.Name)
		} else if isNodeSchedulable(&n) {
			nodes.Items = append(nodes.Items, n)
		}
	}
	return masters, nodes
}

func isNodeSchedulable(node *v1.Node) bool {
	tolerations := []v1.Toleration{
		{
			Key:      sigmak8sapi.LabelResourcePool,
			Operator: v1.TolerationOpEqual,
			Value:    "sigma_public",
			Effect:   v1.TaintEffectNoSchedule,
		},
		{
			Key:      sigmak8sapi.LabelIsECS,
			Operator: v1.TolerationOpEqual,
			Value:    "true",
			Effect:   v1.TaintEffectNoSchedule,
		},
	}
	matchedUntoleratedTaint := false
	for _, taint := range node.Spec.Taints {
		if !v1helper.TolerationsTolerateTaint(tolerations, &taint) {
			matchedUntoleratedTaint = true
			break
		}
	}
	if matchedUntoleratedTaint {
		return false
	}

	nodeReady := e2enode.IsNodeReady(node)
	return !node.Spec.Unschedulable && nodeReady
}

func DeprecatedMightBeMasterNode(nodeName string) bool {
	// We are trying to capture "master(-...)?$" regexp.
	// However, using regexp.MatchString() results even in more than 35%
	// of all space allocations in ControllerManager spent in this function.
	// That's why we are trying to be a bit smarter.
	if strings.HasSuffix(nodeName, "master") {
		return true
	}
	if len(nodeName) >= 10 {
		return strings.HasSuffix(nodeName[:len(nodeName)-3], "master-")
	}
	return false
}

var (
	timeout  = 10 * time.Minute
	waitTime = 2 * time.Second
)

// WaitForStableCluster waits until all existing pods are scheduled and returns their amount.
func WaitForStableCluster(c clientset.Interface, workerNodes sets.String) int {
	startTime := time.Now()
	// Wait for all pods to be scheduled.
	allScheduledPods, allNotScheduledPods := getScheduledAndUnscheduledPods(c, workerNodes)
	for len(allNotScheduledPods) != 0 {
		time.Sleep(waitTime)
		if startTime.Add(timeout).Before(time.Now()) {
			framework.Logf("Timed out waiting for the following pods to schedule")
			for _, p := range allNotScheduledPods {
				framework.Logf("%v/%v", p.Namespace, p.Name)
			}
			framework.Failf("Timed out after %v waiting for stable cluster.", timeout)
			break
		}
		allScheduledPods, allNotScheduledPods = getScheduledAndUnscheduledPods(c, workerNodes)
	}
	return len(allScheduledPods)
}

// getScheduledAndUnscheduledPods lists scheduled and not scheduled pods in all namespaces, with succeeded and failed pods filtered out.
func getScheduledAndUnscheduledPods(c clientset.Interface, workerNodes sets.String) (scheduledPods, notScheduledPods []v1.Pod) {
	pods, err := c.CoreV1().Pods(metav1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})
	framework.ExpectNoError(err, fmt.Sprintf("listing all pods in namespace %q while waiting for stable cluster", metav1.NamespaceAll))

	// API server returns also Pods that succeeded. We need to filter them out.
	filteredPods := make([]v1.Pod, 0, len(pods.Items))
	for _, p := range pods.Items {
		if !podTerminated(p) {
			filteredPods = append(filteredPods, p)
		}
	}
	pods.Items = filteredPods
	return GetPodsScheduled(workerNodes, pods)
}

// GetPodsScheduled returns a number of currently scheduled and not scheduled Pods on worker nodes.
func GetPodsScheduled(workerNodes sets.String, pods *v1.PodList) (scheduledPods, notScheduledPods []v1.Pod) {
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && workerNodes.Has(pod.Spec.NodeName) {
			_, scheduledCondition := podutil.GetPodCondition(&pod.Status, v1.PodScheduled)
			framework.ExpectEqual(scheduledCondition != nil, true)
			if scheduledCondition != nil {
				framework.ExpectEqual(scheduledCondition.Status, v1.ConditionTrue)
				scheduledPods = append(scheduledPods, pod)
			}
		} else if pod.Spec.NodeName == "" {
			notScheduledPods = append(notScheduledPods, pod)
		}
	}
	return
}

func podTerminated(p v1.Pod) bool {
	return p.Status.Phase == v1.PodSucceeded || p.Status.Phase == v1.PodFailed
}

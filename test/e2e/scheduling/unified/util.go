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
	"github.com/sirupsen/logrus"
	sigmak8sapi "gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	v1helper "k8s.io/kubernetes/pkg/apis/core/v1/helper"

	"github.com/koordinator-sh/koordinator/test/e2e/e2e_ak8s/env"
	"github.com/koordinator-sh/koordinator/test/e2e/e2e_ak8s/swarm"
	"github.com/koordinator-sh/koordinator/test/e2e/e2e_ak8s/util"

	"github.com/koordinator-sh/koordinator/test/e2e/framework"
	e2enode "github.com/koordinator-sh/koordinator/test/e2e/framework/node"
	e2epod "github.com/koordinator-sh/koordinator/test/e2e/framework/pod"
	"github.com/koordinator-sh/koordinator/test/e2e/scheduling"
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
	pauseImage := util.SigmaPauseImage
	if pauseImage == "" {
		pauseImage = "k8s.gcr.io/pause-amd64"
	}

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

// createPausePodAction returns a closure that creates a pause pod upon invocation.
func createPausePodAction(f *framework.Framework, conf pausePodConfig) scheduling.Action {
	return func() error {
		_, err := f.ClientSet.CoreV1().Pods(f.Namespace.Name).Create(context.TODO(), initPausePod(f, conf), metav1.CreateOptions{})
		return err
	}
}

// WaitForSchedulerAfterAction performs the provided action and then waits for
// scheduler to act on the given pod.
func WaitForSchedulerAfterAction(f *framework.Framework, action scheduling.Action, podName string, expectSuccess bool) {
	predicate := scheduleFailureEvent(podName)
	if expectSuccess {
		predicate = scheduleSuccessEvent(podName, "" /* any node */)
	}
	success, err := scheduling.ObserveEventAfterAction(f.ClientSet, f.Namespace.Name, predicate, action)
	Expect(err).NotTo(HaveOccurred())
	Expect(success).To(Equal(true))
}

// TODO: upgrade calls in PodAffinity tests when we're able to run them
func verifyResult(c clientset.Interface, expectedScheduled int, expectedNotScheduled int, ns string) {
	allPods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
	framework.ExpectNoError(err)
	scheduledPods, notScheduledPods := scheduling.GetPodsScheduled(masterNodes, allPods)

	printed := false
	printOnce := func(msg string) string {
		if !printed {
			printed = true
			return msg
		}
		return ""
	}

	Expect(len(notScheduledPods)).To(Equal(expectedNotScheduled), printOnce(fmt.Sprintf("Not scheduled Pods: %#v", notScheduledPods)))
	Expect(len(scheduledPods)).To(Equal(expectedScheduled), printOnce(fmt.Sprintf("Scheduled Pods: %#v", scheduledPods)))
}

// verifyReplicasResult is wrapper of verifyResult for a group pods with same "name: labelName" label, which means they belong to same RC
func verifyReplicasResult(c clientset.Interface, expectedScheduled int, expectedNotScheduled int, ns string, labelName string) {
	allPods := getPodsByLabels(c, ns, map[string]string{"name": labelName})
	scheduledPods, notScheduledPods := scheduling.GetPodsScheduled(masterNodes, allPods)

	printed := false
	printOnce := func(msg string) string {
		if !printed {
			printed = true
			return msg
		}
		return ""
	}

	Expect(len(notScheduledPods)).To(Equal(expectedNotScheduled), printOnce(fmt.Sprintf("Not scheduled Pods: %#v", notScheduledPods)))
	Expect(len(scheduledPods)).To(Equal(expectedScheduled), printOnce(fmt.Sprintf("Scheduled Pods: %#v", scheduledPods)))
}

func getPodsByLabels(c clientset.Interface, ns string, labelsMap map[string]string) *v1.PodList {
	selector := labels.SelectorFromSet(labels.Set(labelsMap))
	allPods, err := c.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{LabelSelector: selector.String()})
	framework.ExpectNoError(err)
	return allPods
}

func runAndKeepPodWithLabelAndGetNodeName(f *framework.Framework) (string, string) {
	// launch a pod to find a node which can launch a pod. We intentionally do
	// not just take the node list and choose the first of them. Depending on the
	// cluster and the scheduler it might be that a "normal" pod cannot be
	// scheduled onto it.
	By("Trying to launch a pod with a label to get a node which can launch it.")
	pod := runPausePod(f, pausePodConfig{
		Name:   "with-label-" + string(uuid.NewUUID()),
		Labels: map[string]string{"security": "S1"},
	})
	return pod.Spec.NodeName, pod.Name
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
		logrus.Infof("******** Output of scheduler allocplan")
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

	nodeReady := e2enode.IsConditionSetAsExpected(node, v1.NodeReady, true)
	networkReady := e2enode.IsConditionUnset(node, v1.NodeNetworkUnavailable) ||
		e2enode.IsConditionSetAsExpectedSilent(node, v1.NodeNetworkUnavailable, false)
	return !node.Spec.Unschedulable && nodeReady && networkReady
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

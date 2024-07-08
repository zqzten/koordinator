package runtime

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/frameworkcache"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/noderesource"
	nodepolicy "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/policy"
)

var (
	ErrShouldSkipThisPod = fmt.Errorf("skip this pod")
	//
	ErrPolicyIsNotMatched = fmt.Errorf("the policy is not matched")
	// ErrMsgNotEnoughDevices defines the error message of node has enough devices to allocate
	ErrMsgNotEnoughDevices = "node(s) didn't have enough device resources to allocate"
	// ErrMsgUnSupportPolicy defines the error message of unsupporting policies
	ErrMsgUnSupportPolicy = "Unknown policy for plugin %v,only support:[SingleDevice,MultiDevices,ReplicaDevice]"
	// ErrMsgNotFoundPolicyConfig returns an error message that not found policy configuration
	ErrMsgNotFoundPolicyConfig = "not found any policy configurations"
	// ErrMsgUnSupportAlgorithm defines the error message of unsupporting algorithm
	ErrMsgUnSupportAlgorithm = "plugin(%v) does not support this algorithm,only support algorithm: [binpack, spread]"
	// ErrMsgFailedToGetDeviceResources defines the error message which get device resources failed
	ErrMsgFailedToGetDeviceResources = "failed to get device resources for pod(%v)"
	// ErrMsgDefaultPolicyMoreThanOne returns an error message that default policy not only one
	ErrMsgDefaultPolicyMoreThanOne = "only supports one default policy"
	// ErrMsgNodeNotInCache returns an error message that node information is not found in frameworkcache
	ErrMsgNodeNotInCache = "node(s) didn't be in frameworkcache"
	// ErrMsgNotEnoughActualResources returns an error message that requested resources surpass what the actual device can provide when overselling
	ErrMsgNotEnoughActualResources = "the requested resources surpass what the actual device can provide"
)

type DeviceSharingRuntime struct {
	// the plugin name
	name string
	// resources' names related to plugins
	resourceNames []v1.ResourceName
	// policyInstances stores the all policy instances
	policyInstances []PolicyInstance
	// framework cache
	frameworkCache *frameworkcache.FrameworkCache
}

func NewDeviceSharingRuntime(name string, resourceNames []v1.ResourceName, frameworkCache *frameworkcache.FrameworkCache) *DeviceSharingRuntime {
	return &DeviceSharingRuntime{
		name:           name,
		resourceNames:  resourceNames,
		frameworkCache: frameworkCache,
	}
}

// GetPolicyInstances returns the policy instances
func (d *DeviceSharingRuntime) GetPolicyInstances() []PolicyInstance {
	return d.policyInstances
}

func (d *DeviceSharingRuntime) Name() string {
	return d.name
}

func (d *DeviceSharingRuntime) Init(pcs []PolicyConfig, addOrUpdateNode AddOrUpdateNode, addOrUpdatePod AddOrUpdatePod, eventFuncs []interface{}) error {
	// 1.builds policy instances
	if err := d.buildPolicyInstances(pcs); err != nil {
		return err
	}
	// 2.builds node informations
	nodes, err := d.frameworkCache.GetHandle().SharedInformerFactory().Core().V1().Nodes().Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	pods, err := d.frameworkCache.GetHandle().SharedInformerFactory().Core().V1().Pods().Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	// build node resource cache
	for _, node := range nodes {
		addOrUpdateNode(node)
	}
	// recover use resources from pod
	for _, pod := range pods {
		addOrUpdatePod(pod)
	}
	for _, eventFunc := range eventFuncs {
		if err := d.frameworkCache.AddEventFunc(eventFunc); err != nil {
			return err
		}
	}
	return nil
}

// buildPolicyInstances builds policy instances by policy configuration
func (d *DeviceSharingRuntime) buildPolicyInstances(pcs []PolicyConfig) error {
	result := []PolicyInstance{}
	names := []string{"SingleDevice", "ReplicaDevice", "MultiDevices"}
	newPolicyConfigs := []PolicyConfig{}
	for _, pc := range pcs {
		for _, policyName := range names {
			newPolicyConfigs = append(newPolicyConfigs, PolicyConfig{
				PolicyName:    policyName,
				AlgorithmName: pc.AlgorithmName,
				NodeSelectors: pc.NodeSelectors,
			})
		}
	}
	for _, policyConfig := range newPolicyConfigs {
		var algorithm nodepolicy.Algorithm
		var algorithmR nodepolicy.Algorithm
		var policy nodepolicy.Policy
		// create algorithm by algorithm type
		switch nodepolicy.AlgorithmType(policyConfig.AlgorithmName) {
		case nodepolicy.Binpack:
			algorithm = nodepolicy.NewBinpackAlgorithm()
			algorithmR = nodepolicy.NewReplicaBinpackAlgorithm()
		case nodepolicy.Spread:
			algorithm = nodepolicy.NewSpreadAlgorithm()
			algorithmR = nodepolicy.NewReplicaSpreadAlgorithm()
		default:
			return fmt.Errorf(ErrMsgUnSupportAlgorithm, d.name)
		}
		// create policy by policy type
		switch nodepolicy.PolicyType(policyConfig.PolicyName) {
		case nodepolicy.MultiDevices:
			factory := nodepolicy.MultiDevicesPolicyFactory{}
			policy = factory.Create(algorithm)
		case nodepolicy.SingleDevice:
			factory := nodepolicy.SingleDevicePolicyFactory{}
			policy = factory.Create(algorithm)
		case nodepolicy.ReplicaDevice:
			factory := nodepolicy.ReplicaDevicePolicyFactory{}
			policy = factory.Create(algorithmR)
		default:
			return fmt.Errorf(ErrMsgUnSupportPolicy, d.name)
		}
		if policyConfig.NodeSelectors == nil {
			policyConfig.NodeSelectors = map[string]string{}
		}
		isDefault := false
		if len(policyConfig.NodeSelectors) == 0 {
			isDefault = true
		}
		klog.V(4).Infof("succeed to create policy instance by policy configuration %v", policyConfig)
		result = append(result, PolicyInstance{
			Policy:    policy,
			Config:    policyConfig,
			IsDefault: isDefault,
		})
	}
	klog.Infof("succeed to create %v policy instances", len(result))
	d.policyInstances = result
	return nil
}

// FilterNode is invoked by the Filter function of scheduler framework plugin
// it checks the given node has enough devices for the pod request
// func (d *DeviceSharingRuntime) FilterNode(state *framework.CycleState, pod *v1.Pod, node *v1.Node, searchNodePolicy SearchNodePolicy, filterNodeByPolicy FitlerNodeByPolicy) *framework.Status {
func (d *DeviceSharingRuntime) FilterNode(state *framework.CycleState, args FilterNodeArgs) *framework.Status {
	pod := args.SchedulePod
	node := args.Node
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	if !IsMyPod(pod, d.resourceNames...) {
		return nil
	}
	// if node has no resources,skip it
	if !IsMyNode(node, d.resourceNames...) {
		klog.Warningf("pod=%v,node=%v,message=[node is invalid for gpushare,it has none of resource in %v]", podFullName, node.Name, d.resourceNames)
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("none(s) should have at least one resource in %v", d.resourceNames))
	}
	nodeName := node.Name
	// if not found extend node information,build this node
	nrm, err := args.GetNodeResourceManager(state, nodeName)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("%v", err))
	}
	// if currently available device units of the node is more than request,
	// the node is a candidate node,otherwise skip this node
	policyInstance, err := args.SearchNodePolicy(pod, node, d.policyInstances)
	if err != nil {
		if err == ErrPolicyIsNotMatched {
			klog.Infof("pod: %v,node: %v,node is satisfied: %v,policy: NotFound",
				podFullName,
				nodeName,
				false,
			)
			return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("%v", err))
		}
		return framework.NewStatus(framework.Error, fmt.Sprintf("%v", err))
	}
	printInfo := []string{}
	nodeResources := GetNodeResourcesByNames(node, d.resourceNames...)
	podRequestResources := GetPodRequestResourcesByNames(pod, d.resourceNames...)
	for _, resourceName := range d.resourceNames {
		// if not found devices in node information,update node
		if nodeResources[resourceName] == 0 {
			continue
		}
		info := nrm.GetNodeResourceCache(resourceName).String()
		printInfo = append(printInfo, fmt.Sprintf("ResourceName: %v,Devices: %v", resourceName, info))
	}
	// check the node has enough available devices to allocate
	satisfied, err := args.FitlerNodeByPolicy(state, pod, node, policyInstance, nrm)
	if err != nil {
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("%v", err))
	}
	if !satisfied {
		klog.V(5).Infof("Plugin=%v,Phase=Filter,Pod=%v,Node=%v,NodeIsSatisfied=%v,Policy=%v,NodeResourceCache=%v",
			d.name,
			podFullName,
			nodeName,
			satisfied,
			policyInstance.Config,
			strings.Join(printInfo, ";"),
		)
		lister := d.frameworkCache.GetHandle().SharedInformerFactory().Core().V1().Pods().Lister()
		for _, resourceName := range d.resourceNames {
			if podRequestResources[resourceName] == 0 {
				continue
			}
			nrm.GetNodeResourceCache(resourceName).CheckAndCleanInvaildPods(lister)
		}
		return framework.NewStatus(framework.Unschedulable, ErrMsgNotEnoughDevices)
	}
	return nil
}

// ScoreNode is invoked by the Score function of scheduler framework plugin
// it scores every node and return the score
func (d *DeviceSharingRuntime) ScoreNode(state *framework.CycleState, pod *v1.Pod, nodeName string, allocatDeviceResources AllocateDeviceResources) (int64, *framework.Status) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// if pod is not my pod,skip it
	if !IsMyPod(pod, d.resourceNames...) {
		return 0, nil
	}
	// get the node
	node, err := d.frameworkCache.GetHandle().ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return 0, framework.NewStatus(framework.UnschedulableAndUnresolvable, fmt.Sprintf("failed to get node from client-go: %v", err))
	}
	// if node has no resources,skip it
	if !IsMyNode(node, d.resourceNames...) {
		klog.Warningf("pod=%v,node=%v,message=[node is invalid for gpushare,it has none of resource in %v]", podFullName, node.Name, d.resourceNames)
		return 0, framework.NewStatus(framework.Unschedulable, fmt.Sprintf("none(s) should have at least one resource in %v", d.resourceNames))
	}
	// if the pod is not a pod we care about,skip to check the node
	allocations, score, err := d.allocate(state, pod, node, "score", allocatDeviceResources)
	if err != nil {
		klog.Errorf("failed to get allocation for pod %v,reason: %v", err)
		return 0, framework.NewStatus(framework.Error, err.Error())
	}
	if len(allocations) == 0 {
		return 0, framework.NewStatus(framework.Unschedulable, ErrMsgNotEnoughDevices)
	}
	return int64(score), nil
}

// AllocateAssumedDevices is invoked by the Resolve function of scheduler framework plugin
// it write the allocation to the cache, it the pod is failed in binding phase,the alloction
// will be deleted from the cache
func (d *DeviceSharingRuntime) ReserveResources(state *framework.CycleState, pod *v1.Pod, nodeName string, allocatDeviceResources AllocateDeviceResources, buildPodAnnotations BuildPodAnnotations) *framework.Status {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// if pod is not my pod,skip it
	if !IsMyPod(pod, d.resourceNames...) {
		return nil
	}
	// get the node
	node, err := d.frameworkCache.GetHandle().ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, fmt.Sprintf("failed to get node from client-go: %v", err))
	}
	// if node has no resources,skip it
	if !IsMyNode(node, d.resourceNames...) {
		klog.Warningf("pod=%v,node=%v,message=[node is invalid for gpushare,it has none of resource in %v]", podFullName, node.Name, d.resourceNames)
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("none(s) should have at least one resource in %v", d.resourceNames))
	}
	allocations, _, err := d.allocate(state, pod, node, "reserve", allocatDeviceResources)
	if err != nil {
		klog.Errorf("failed to get allocation for pod %v,reason: %v", podFullName, err)
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	if len(allocations) == 0 {
		return framework.NewStatus(framework.Unschedulable, ErrMsgNotEnoughDevices)
	}
	// add allocations to node cache
	nrm := d.frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
	for resourceName, podResource := range allocations {
		d.frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
		nrm.GetNodeResourceCache(resourceName).AddAllocatedPodResources(podResource)
	}
	annotations, err := buildPodAnnotations(pod, allocations, nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to build annotations of pod %v,reason: %v", podFullName, err))
	}
	// must get new pod from client-go cache,make sure the pod is new
	newPod, err := d.frameworkCache.GetHandle().SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).Get(pod.Name)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to get pod from client-go cache,reason: %v", err))
	}
	// update pod annotations to api server
	_, err = UpdatePodAnnotations(d.frameworkCache.GetHandle().ClientSet(), newPod, annotations)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	klog.V(5).Infof("Plugin=%v,Phase=Reserve,Pod=%v,Node=%v,Message: succeed to update pod annotations",
		d.name,
		podFullName,
		nodeName,
	)
	return nil
}

func (d *DeviceSharingRuntime) allocate(state *framework.CycleState, pod *v1.Pod, node *v1.Node, phase string, allocatDeviceResources AllocateDeviceResources) (map[v1.ResourceName]noderesource.PodResource, int, error) {
	// if the pod is not a pod we care about,skip to check the node
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// get policy instance from cycle state
	allocations, score, err := allocatDeviceResources(state, pod, node, d.policyInstances)
	if err != nil {
		return nil, 0, err
	}
	if len(allocations) != 0 {
		allocationLog := []string{}
		for resourceName, assumedPodResource := range allocations {
			allocationLog = append(allocationLog, fmt.Sprintf("%v: %v", resourceName, assumedPodResource.AllocatedResourcesToString()))
		}
		printLog := klog.Infof
		if phase == "score" {
			printLog = klog.V(5).Infof
		}
		printLog("Plugin=%v,Phase=%v,Pod=%v,Node=%v,Score=%v,AssumedPodResource=[%v]",
			d.name,
			phase,
			podFullName,
			node.Name,
			score,
			strings.Join(allocationLog, ";"),
		)
	}
	return allocations, score, nil
}

// UnReserveResources is invoked by the Unreserve function of scheduler framework plugin
// if the binding is failed,this function will rollback the allocation which has been written to cache
func (d *DeviceSharingRuntime) UnReserveResources(state *framework.CycleState, pod *v1.Pod, nodeName string) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// if pod is not my pod,skip it
	if !IsMyPod(pod, d.resourceNames...) {
		return
	}
	if !d.frameworkCache.NodeIsInCache(nodeName) {
		return
	}
	podRequestResources := GetPodRequestResourcesByNames(pod, d.resourceNames...)
	nrm := d.frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
	for _, resourceName := range d.resourceNames {
		if podRequestResources[resourceName] == 0 {
			continue
		}
		nrm.GetNodeResourceCache(resourceName).RemoveAllocatedPodResources(pod.UID)
	}
	klog.Warningf("Plugin=%v,Phase=UnReserve,Pod=%v,Node=%v,Message: succeed to rollback assumed podresource",
		d.name,
		podFullName,
		nodeName,
	)
}

/*
// BindPod is invoked by the Bind function of scheduler framework plugin
func (d *DeviceSharingRuntime) BindPod(pod *v1.Pod, nodeName string, buildPodAnnotations BuildPodAnnotations) *framework.Status {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// if the pod is not a pod we care about,skip binding pod to the node
	if !IsDeviceSharingPod(d.resourceNames, pod) {
		return framework.NewStatus(framework.Skip)
	}
	nrm := d.frameworkCache.GetExtendNodeInfo(nodeName).GetNodeResourceManager()
	allocations := map[types.ResourceName]*noderesource.PodResource{}
	for _, resourceName := range d.resourceNames {
		count := GetDeviceSharingPodRequestCount(resourceName, pod)
		if count <= 0 {
			continue
		}
		p := nrm.GetNodeResourceCache(resourceName).GetAllocatedPodResource(pod.UID)
		if p == nil {
			klog.Errorf("not found assumed pod resource(%v) for pod %v on node cache %v", resourceName, podFullName, nodeName)
			continue
		}
		allocations[resourceName] = p
	}

	annotations, err := buildPodAnnotations(pod, allocations, nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to build annotations of pod %v,reason: %v", podFullName, err))
	}
	klog.V(5).Infof("Plugin=%v,Phase=Bind,Pod=%v,Node=%v,Message: add new pod annotations: %v",
		d.name,
		podFullName,
		nodeName,
		annotations,
	)
	// update pod annotations and cannot use function passed parameters 'pod' to update annotation
	// must get new pod from client-go cache
	newPod, err := d.frameworkCache.GetHandle().SharedInformerFactory().Core().V1().Pods().Lister().Pods(pod.Namespace).Get(pod.Name)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("failed to get pod from client-go cache,reason: %v", err))
	}
	newPod, err = UpdatePodAnnotations(d.frameworkCache.GetHandle().ClientSet(), newPod, annotations)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	klog.V(5).Infof("Plugin=%v,Phase=Bind,Pod=%v,Node=%v,Message: succeed to update pod annotations",
		d.name,
		podFullName,
		nodeName,
	)
	// bind pod to node
	binding := &v1.Binding{
		ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID},
		Target:     v1.ObjectReference{Kind: "Node", Name: nodeName},
	}
	err = d.frameworkCache.GetHandle().ClientSet().CoreV1().Pods(binding.Namespace).Bind(context.TODO(), binding, metav1.CreateOptions{})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	klog.Infof("Plugin=%v,Phase=Bind,Pod=%v,Node=%v,Message: succeed to bind pod to node",
		d.name,
		podFullName,
		nodeName,
	)
	return nil
}
*/

// update pod annotations
func UpdatePodAnnotations(client clientset.Interface, pod *v1.Pod, annotations map[string]string) (*v1.Pod, error) {
	newPod := updateAnnotations(pod, annotations)
	var updatedPod *v1.Pod
	var err error
	updatedPod, err = client.CoreV1().Pods(newPod.ObjectMeta.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
	if err != nil {
		// the object has been modified; please apply your changes to the latest version and try again
		if strings.Contains(err.Error(), "the object has been modified") {
			// retry
			oldPod, err := client.CoreV1().Pods(pod.ObjectMeta.Namespace).Get(context.TODO(), pod.ObjectMeta.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			newPod = updateAnnotations(oldPod, annotations)
			return client.CoreV1().Pods(newPod.ObjectMeta.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
		}
		return nil, err
	}
	return updatedPod, nil
}

func updateAnnotations(oldPod *v1.Pod, annotations map[string]string) *v1.Pod {
	newPod := oldPod.DeepCopy()
	if len(newPod.ObjectMeta.Annotations) == 0 {
		newPod.ObjectMeta.Annotations = map[string]string{}
	}
	for k, v := range annotations {
		newPod.ObjectMeta.Annotations[k] = v
	}
	return newPod
}

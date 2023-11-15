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

package deviceshare

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	unifiedschedulingv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	schedulingv1alpha1listers "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config/v1beta2"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/deviceshare"
)

const (
	Name = "UnifiedDeviceShare"

	stateKey                 = Name
	ErrInvalidDriverVersion  = "node(s) invalid driver version"
	ErrMismatchDriverVersion = "node(s) mismatch driver version"
)

var (
	_ framework.EnqueueExtensions = &Plugin{}

	_ framework.PreFilterPlugin = &Plugin{}
	_ framework.FilterPlugin    = &Plugin{}
	_ framework.ReservePlugin   = &Plugin{}
	_ framework.PreBindPlugin   = &Plugin{}

	_ frameworkext.ReservationRestorePlugin = &Plugin{}
	_ frameworkext.ReservationFilterPlugin  = &Plugin{}
	_ frameworkext.ReservationPreBindPlugin = &Plugin{}
)

type Plugin struct {
	handle       frameworkext.ExtendedHandle
	deviceLister schedulingv1alpha1listers.DeviceLister
	*deviceshare.Plugin
}

func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	args, err := getDeviceShareArgs(obj)
	if err != nil {
		return nil, err
	}

	internalPlugin, err := deviceshare.New(args, handle)
	if err != nil {
		return nil, err
	}

	extendedHandle, ok := handle.(frameworkext.ExtendedHandle)
	if !ok {
		return nil, fmt.Errorf("expect handle to be type frameworkext.ExtendedHandle, got %T", handle)
	}
	deviceLister := extendedHandle.KoordinatorSharedInformerFactory().Scheduling().V1alpha1().Devices().Lister()

	p := &Plugin{
		handle:       extendedHandle,
		deviceLister: deviceLister,
		Plugin:       internalPlugin.(*deviceshare.Plugin),
	}
	return p, nil
}

func getDeviceShareArgs(obj runtime.Object) (*schedulingconfig.DeviceShareArgs, error) {
	if obj == nil {
		return getDefaultDeviceShareArgs()
	}

	unknownObj, ok := obj.(*runtime.Unknown)
	if !ok {
		return nil, fmt.Errorf("got args of type %T, want *DeviceShareArgs", obj)
	}
	var v1beta2args v1beta2.DeviceShareArgs
	v1beta2.SetDefaults_DeviceShareArgs(&v1beta2args)
	if err := frameworkruntime.DecodeInto(unknownObj, &v1beta2args); err != nil {
		return nil, err
	}
	var args schedulingconfig.DeviceShareArgs
	err := v1beta2.Convert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs(&v1beta2args, &args, nil)
	if err != nil {
		return nil, err
	}
	return &args, nil
}

func getDefaultDeviceShareArgs() (*schedulingconfig.DeviceShareArgs, error) {
	var v1beta2args v1beta2.DeviceShareArgs
	v1beta2.SetDefaults_DeviceShareArgs(&v1beta2args)
	var defaultDeviceShareArgs schedulingconfig.DeviceShareArgs
	err := v1beta2.Convert_v1beta2_DeviceShareArgs_To_config_DeviceShareArgs(&v1beta2args, &defaultDeviceShareArgs, nil)
	if err != nil {
		return nil, err
	}
	return &defaultDeviceShareArgs, nil
}

func (pl *Plugin) Name() string {
	return Name
}

type preFilterState struct {
	skip        bool
	podRequests corev1.ResourceList
}

func (s *preFilterState) Clone() framework.StateData {
	return s
}

func getPreFilterState(cycleState *framework.CycleState) (*preFilterState, *framework.Status) {
	value, err := cycleState.Read(stateKey)
	if err != nil {
		return nil, framework.AsStatus(err)
	}
	state := value.(*preFilterState)
	return state, nil
}

func (pl *Plugin) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod) (*framework.PreFilterResult, *framework.Status) {
	result, status := pl.Plugin.PreFilter(ctx, cycleState, pod)
	if !status.IsSuccess() {
		return nil, status
	}

	state := &preFilterState{
		skip: true,
	}

	state.skip, state.podRequests, status = deviceshare.PreparePod(pod)
	if !status.IsSuccess() {
		return nil, status
	}
	cycleState.Write(stateKey, state)
	return result, nil
}

func (pl *Plugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(nodeInfo.Node()) {
		return nil
	}
	if _, err := pl.deviceLister.Get(node.Name); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	state, status := getPreFilterState(cycleState)
	if !status.IsSuccess() {
		return status
	}
	if state.skip {
		return nil
	}

	if status := pl.checkDriverVersionIfNeed(state, pod, node); !status.IsSuccess() {
		return status
	}

	return pl.Plugin.Filter(ctx, cycleState, pod, nodeInfo)
}

func (pl *Plugin) checkDriverVersionIfNeed(state *preFilterState, pod *corev1.Pod, node *corev1.Node) *framework.Status {
	if pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "rund" && deviceshare.HasDeviceResource(state.podRequests, schedulingv1alpha1.GPU) {
		var matchedVersion string
		var err error
		if isGPUSharedPod(state.podRequests) {
			hostDriverVersion := node.Labels[apiext.LabelGPUDriverVersion]
			if hostDriverVersion == "" {
				return framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrInvalidDriverVersion)
			}
			matchedVersion, err = matchDriverVersions(pod, extunified.NVIDIADriverVersions{hostDriverVersion})
		} else {
			var deviceObj *schedulingv1alpha1.Device
			deviceObj, err = pl.deviceLister.Get(node.Name)
			if err != nil {
				return framework.NewStatus(framework.UnschedulableAndUnresolvable, deviceshare.ErrMissingDevice)
			}
			matchedVersion, err = MatchSupportedVMDriverVersions(pod, deviceObj)
		}
		if err != nil {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrInvalidDriverVersion)
		}
		if matchedVersion == "" {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, ErrMismatchDriverVersion)
		}
	}
	return nil
}

func (pl *Plugin) FilterReservation(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, reservationInfo *frameworkext.ReservationInfo, nodeName string) *framework.Status {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}
	return pl.Plugin.FilterReservation(ctx, cycleState, pod, reservationInfo, nodeName)
}

func (pl *Plugin) Reserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}

	return pl.Plugin.Reserve(ctx, cycleState, pod, nodeName)
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return
	}
	node := nodeInfo.Node()
	if node == nil {
		return
	}
	if extunified.IsVirtualKubeletNode(node) {
		return
	}
	pl.Plugin.Unreserve(ctx, cycleState, pod, nodeName)
}

func (pl *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}
	status := pl.Plugin.PreBind(ctx, cycleState, pod, nodeName)
	if !status.IsSuccess() {
		return status
	}

	allocations, err := apiext.GetDeviceAllocations(pod.Annotations)
	if err != nil {
		return framework.AsStatus(err)
	}
	if len(allocations) == 0 {
		return nil
	}

	if err := pl.appendInternalAnnotations(pod, allocations, node); err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	return nil
}

func (pl *Plugin) PreBindReservation(ctx context.Context, cycleState *framework.CycleState, reservation *schedulingv1alpha1.Reservation, nodeName string) *framework.Status {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if extunified.IsVirtualKubeletNode(node) {
		return nil
	}
	return pl.Plugin.PreBindReservation(ctx, cycleState, reservation, nodeName)
}

func (pl *Plugin) appendInternalAnnotations(obj metav1.Object, allocResult apiext.DeviceAllocations, node *corev1.Node) error {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return nil
	}

	nodeName := node.Name
	device, err := pl.deviceLister.Get(nodeName)
	if err != nil {
		klog.ErrorS(err, "Failed to get Device", "pod", klog.KObj(pod), "node", nodeName)
		return err
	}

	ack.AppendAckAnnotationsIfHasGPUCompute(pod, device, allocResult)
	if k8sfeature.DefaultFeatureGate.Enabled(features.EnableACKGPUShareScheduling) {
		if err := ack.AppendAckAnnotationsIfHasGPUMemory(pod, allocResult); err != nil {
			klog.ErrorS(err, "Failed to append ACK Annotation with GPU Memory", "pod", klog.KObj(pod))
			return err
		}
		if err := ack.AppendAckAnnotationsIfHasGPUCore(pod, allocResult); err != nil {
			klog.ErrorS(err, "Failed to append ACK Annotation with GPU Core", "pod", klog.KObj(pod))
			return err
		}
		if err := appendKoordGPUMemoryRatioIfNeeded(pod, allocResult, node); err != nil {
			klog.ErrorS(err, "Failed to appendKoordGPUMemoryRatioIfNeeded", "pod", klog.KObj(pod))
			return err
		}
	}
	if err := appendUnifiedDeviceAllocStatus(pod, allocResult); err != nil {
		return err
	}
	return appendRundResult(pod, allocResult, pl)
}

func appendKoordGPUMemoryRatioIfNeeded(pod *corev1.Pod, deviceAllocations apiext.DeviceAllocations, node *corev1.Node) error {
	// Pod 如果声明了 ack gpu share 协议，又调度到了只有koordinator dp的节点，
	// 这个时候需要追加 koordinator.sh/gpu-memory-ratio 触发kubelet DP链路
	if node.Labels["__internal_gpu-compatible__"] != "koordinator-gpu-as-ack-gpu" {
		return nil
	}

	podRequests, _ := resource.PodRequestsAndLimits(pod)
	if quantity := podRequests[ack.ResourceAliyunGPUMemory]; quantity.IsZero() {
		return nil
	}

	totalGPUResources := make(corev1.ResourceList)
	for deviceType, allocations := range deviceAllocations {
		if len(allocations) <= 0 {
			continue
		}
		if deviceType != schedulingv1alpha1.GPU {
			continue
		}
		for _, deviceAllocation := range allocations {
			resources := extunified.ConvertToUnifiedGPUResources(deviceAllocation.Resources)
			totalGPUResources = quotav1.Add(totalGPUResources, resources)
		}
	}

	if len(totalGPUResources) > 0 {
		totalGPUMemory := totalGPUResources[unifiedresourceext.GPUResourceMem]
		totalGPUMemoryRatio := totalGPUResources[unifiedresourceext.GPUResourceMemRatio]
		if totalGPUMemory.IsZero() {
			return fmt.Errorf("unreached error but got, missing GPUResourceMem")
		}
		for i := range pod.Spec.Containers {
			container := &pod.Spec.Containers[i]
			if !deviceshare.HasDeviceResource(container.Resources.Requests, schedulingv1alpha1.GPU) {
				continue
			}
			combination, err := deviceshare.ValidateDeviceRequest(container.Resources.Requests)
			if err != nil {
				return err
			}
			resources := deviceshare.ConvertDeviceRequest(container.Resources.Requests, combination)
			gpuMemoryQuantity := resources[apiext.ResourceGPUMemory]
			gpuMemoryRatioQuantity := resources[apiext.ResourceGPUMemoryRatio]
			if gpuMemoryQuantity.IsZero() && gpuMemoryRatioQuantity.IsZero() {
				continue
			}
			needPatch := false
			var memoryRatio int64
			if gpuMemoryQuantity.Value() > 0 {
				needPatch = true
				memoryRatio = gpuMemoryQuantity.Value() * totalGPUMemoryRatio.Value() / totalGPUMemory.Value()
			} else if gpuMemoryRatioQuantity.Value() > 0 {
				needPatch = true
				memoryRatio = gpuMemoryRatioQuantity.Value()
			}
			if needPatch {
				if addContainerGPUResourceForPatch(container, apiext.ResourceGPUMemoryRatio, memoryRatio) {
					setContainerEnv(container, &corev1.EnvVar{Name: extunified.EnvActivelyAddedUnifiedGPUMemoryRatio, Value: "true"})
				}
			}
		}
	}

	return nil
}

func appendUnifiedDeviceAllocStatus(pod *corev1.Pod, deviceAllocations apiext.DeviceAllocations) error {
	if !deviceshare.EnableUnifiedDevice {
		return nil
	}

	allocStatus := &unifiedresourceext.MultiDeviceAllocStatus{}
	allocStatus.AllocStatus = make(map[unifiedschedulingv1beta1.DeviceType][]unifiedresourceext.ContainerDeviceAllocStatus)
	var minors []string
	totalGPUResources := make(corev1.ResourceList)
	for deviceType, allocations := range deviceAllocations {
		if len(allocations) <= 0 {
			continue
		}
		unifiedAllocs := make([]unifiedschedulingv1beta1.Alloc, 0, len(allocations))
		for _, deviceAllocation := range allocations {
			resources := extunified.ConvertToUnifiedGPUResources(deviceAllocation.Resources)
			resourceList := make(map[string]apiresource.Quantity)
			for name, quantity := range resources {
				resourceList[string(name)] = quantity
			}
			unifiedAlloc := unifiedschedulingv1beta1.Alloc{
				Minor:     deviceAllocation.Minor,
				Resources: resourceList,
				IsSharing: !isExclusiveGPURes(resourceList),
			}
			unifiedAllocs = append(unifiedAllocs, unifiedAlloc)
			if deviceType == schedulingv1alpha1.GPU {
				minors = append(minors, strconv.Itoa(int(deviceAllocation.Minor)))
				totalGPUResources = quotav1.Add(totalGPUResources, resources)
			}
		}

		containerDeviceAllocStatuses := make([]unifiedresourceext.ContainerDeviceAllocStatus, 1)
		containerDeviceAllocStatuses[0].DeviceAllocStatus.Allocs = unifiedAllocs
		switch deviceType {
		case schedulingv1alpha1.GPU:
			allocStatus.AllocStatus[unifiedschedulingv1beta1.GPU] = containerDeviceAllocStatuses
		case schedulingv1alpha1.RDMA:
			allocStatus.AllocStatus[unifiedschedulingv1beta1.RDMA] = containerDeviceAllocStatuses
		case schedulingv1alpha1.FPGA:
			allocStatus.AllocStatus[unifiedschedulingv1beta1.FPGA] = containerDeviceAllocStatuses
		}
	}
	data, err := json.Marshal(allocStatus)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[unifiedresourceext.AnnotationMultiDeviceAllocStatus] = string(data)
	visibleDevices := strings.Join(minors, ",")
	if len(minors) > 0 {
		pod.Annotations[unifiedresourceext.AnnotationNVIDIAVisibleDevices] = visibleDevices
	}

	if k8sfeature.DefaultFeatureGate.Enabled(features.UnifiedDeviceScheduling) && len(totalGPUResources) > 0 {
		totalGPUMemory := totalGPUResources[unifiedresourceext.GPUResourceMem]
		totalGPUMemoryRatio := totalGPUResources[unifiedresourceext.GPUResourceMemRatio]
		if totalGPUMemory.IsZero() {
			return fmt.Errorf("unreached error but got, missing GPUResourceMem")
		}
		for i := range pod.Spec.Containers {
			container := &pod.Spec.Containers[i]
			if !deviceshare.HasDeviceResource(container.Resources.Requests, schedulingv1alpha1.GPU) {
				continue
			}
			combination, err := deviceshare.ValidateDeviceRequest(container.Resources.Requests)
			if err != nil {
				return err
			}
			resources := deviceshare.ConvertDeviceRequest(container.Resources.Requests, combination)
			gpuMemoryQuantity := resources[apiext.ResourceGPUMemory]
			gpuMemoryRatioQuantity := resources[apiext.ResourceGPUMemoryRatio]
			if gpuMemoryQuantity.IsZero() && gpuMemoryRatioQuantity.IsZero() {
				continue
			}
			needPatch := false
			var memoryRatio int64
			if gpuMemoryQuantity.Value() > 0 {
				needPatch = true
				memoryRatio = gpuMemoryQuantity.Value() * totalGPUMemoryRatio.Value() / totalGPUMemory.Value()
			} else if gpuMemoryRatioQuantity.Value() > 0 {
				needPatch = true
				memoryRatio = gpuMemoryRatioQuantity.Value()
			}
			if needPatch {
				if addContainerGPUResourceForPatch(container, unifiedresourceext.GPUResourceMemRatio, memoryRatio) {
					// NOTE: Kube Scheduler Framework 通过Filter/Score 选择出一个合适的节点后会 Assume Pod 到 NodeInfo 中，
					// 此时 Pod 的容器中并没有声明资源 alibabacloud.com/gpu-mem-ratio，所以 NodeInfo.Requested 中也不会记录
					// 该资源名称。但我们又会在 PreBind 阶段追加这个资源，导致后续 Pod 被删除时，NodeInfo.RemovePod() 会按照最新的 Pod
					// 清理 NodeInfo.Requested，导致 NodeInfo.Requested.ScalarResources["alibabacloud.com/gpu-mem-ratio"] 变成负数。
					// 后续如果有 Pod 又使用了 alibabacloud.com/gpu-mem-ratio 请求资源时，会导致像插件 NodeResourcesFit.Score 结果变成负数。
					// 这里其实有两种 Fix 方法，一种是在 Reserve 阶段调用 SchedulerCache.Forget，再追加资源再Assume的方式。但这种方式改动量更大一些。
					// 另一种方式就是这一次采用的，Container上追加一个标识，然后再在 Transformer 中处理掉。对齐账本。
					setContainerEnv(container, &corev1.EnvVar{Name: extunified.EnvActivelyAddedUnifiedGPUMemoryRatio, Value: "true"})
				}
				setContainerEnv(container, &corev1.EnvVar{Name: "NVIDIA_VISIBLE_DEVICES", Value: visibleDevices})
			}
		}
	}

	return nil
}

func setContainerEnv(container *corev1.Container, envVar *corev1.EnvVar) {
	for i := range container.Env {
		if container.Env[i].Name == envVar.Name {
			container.Env[i] = *envVar
			return
		}
	}

	container.Env = append(container.Env, *envVar)
}

// addContainerResourceForPatch adds container GPU resources to patch bytes to update pod resource specs
func addContainerGPUResourceForPatch(container *corev1.Container, resourceName corev1.ResourceName, resourceQuantity int64) bool {
	p := apiresource.Quantity{}
	if resourceQuantity <= 0 {
		resourceQuantity = 1
	}
	p.Set(resourceQuantity)
	if container.Resources.Limits == nil {
		container.Resources.Limits = make(corev1.ResourceList)
	}
	if container.Resources.Requests == nil {
		container.Resources.Requests = make(corev1.ResourceList)
	}
	q := container.Resources.Limits[resourceName]
	container.Resources.Limits[resourceName] = p
	container.Resources.Requests[resourceName] = p
	return !q.Equal(p)
}

// res contains exclusive GPU if and only if:
// GPU subResources (gpu-mem-ratio, gpu-core) are multiples of 100 and of the same value.
func isExclusiveGPURes(res map[string]apiresource.Quantity) bool {
	var subResVal int64
	for _, resName := range []corev1.ResourceName{unifiedresourceext.GPUResourceMemRatio, unifiedresourceext.GPUResourceCore} {
		if value, ok := res[resName.String()]; ok && (value.Value() > 0) {
			if value.Value()%100 != 0 {
				// sub resource not in full 100s
				return false
			}
			if subResVal == 0 {
				subResVal = value.Value()
			} else {
				if subResVal != value.Value() {
					// sub resources not of the same value
					return false
				}
			}
		} else {
			// missing one of the two sub resources
			return false
		}
	}
	return true
}

func getDevicesBusID(pod *corev1.Pod, allocResult apiext.DeviceAllocations, deviceLister schedulingv1alpha1listers.DeviceLister) (map[schedulingv1alpha1.DeviceType]map[int]string, error) {
	device, err := deviceLister.Get(pod.Spec.NodeName)
	if err != nil {
		return nil, fmt.Errorf("not found nodeDevice for node %v", pod.Spec.NodeName)
	}

	pciInfos, err := extunified.GetDevicePCIInfos(device.Annotations)
	if err != nil {
		return nil, err
	}
	pciInfoMap := map[schedulingv1alpha1.DeviceType]map[int32]extunified.DevicePCIInfo{}
	for _, v := range pciInfos {
		m := pciInfoMap[v.Type]
		if m == nil {
			m = map[int32]extunified.DevicePCIInfo{}
			pciInfoMap[v.Type] = m
		}
		m[v.Minor] = v
	}

	devicesBusID := map[schedulingv1alpha1.DeviceType]map[int]string{}

	for deviceType, allocations := range allocResult {
		if deviceType != schedulingv1alpha1.GPU && deviceType != extunified.NVSwitchDeviceType {
			continue
		}
		devices := pciInfoMap[deviceType]
		if len(devices) == 0 {
			return nil, fmt.Errorf("not found %v devices on node %v", deviceType, pod.Spec.NodeName)
		}
		m := devicesBusID[deviceType]
		if m == nil {
			m = map[int]string{}
			devicesBusID[deviceType] = m
		}
		for _, v := range allocations {
			d, ok := devices[v.Minor]
			if !ok {
				return nil, fmt.Errorf("not found %v device by minor %d on node %v", deviceType, v.Minor, pod.Spec.NodeName)
			}
			m[int(v.Minor)] = d.BusID
		}
	}

	return devicesBusID, nil
}

type deviceMinorBusIDPair struct {
	minor int
	busID string
}

func toDeviceMinorBusIDPairs(busIDs map[int]string) []deviceMinorBusIDPair {
	if len(busIDs) == 0 {
		return nil
	}
	var pairs []deviceMinorBusIDPair
	for k, v := range busIDs {
		pairs = append(pairs, deviceMinorBusIDPair{
			minor: k,
			busID: v,
		})
	}
	return pairs
}

func appendRundResult(pod *corev1.Pod, allocResult apiext.DeviceAllocations, pl *Plugin) error {
	if pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "rund" {
		return nil
	}
	gpuAllocations := allocResult[schedulingv1alpha1.GPU]
	nvSwitchAllocations := allocResult[extunified.NVSwitchDeviceType]
	if len(gpuAllocations) != 0 && isGPUSharedPod(gpuAllocations[0].Resources) && len(nvSwitchAllocations) == 0 {
		return nil
	}
	extendedHandle, ok := pl.handle.(frameworkext.ExtendedHandle)
	if !ok {
		return fmt.Errorf("expect handle to be type frameworkext.ExtendedHandle, got %T", pl.handle)
	}
	deviceLister := extendedHandle.KoordinatorSharedInformerFactory().Scheduling().V1alpha1().Devices().Lister()
	devicesBusID, err := getDevicesBusID(pod, allocResult, deviceLister)
	if err != nil {
		return err
	}

	var passthroughDevices []string
	var nvSwitches []string
	for _, deviceType := range []schedulingv1alpha1.DeviceType{schedulingv1alpha1.GPU, extunified.NVSwitchDeviceType} {
		busIDs := devicesBusID[deviceType]
		if len(busIDs) == 0 {
			continue
		}
		pairs := toDeviceMinorBusIDPairs(busIDs)
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i].minor < pairs[j].minor
		})
		for _, v := range pairs {
			passthroughDevices = append(passthroughDevices, v.busID)
			if deviceType == extunified.NVSwitchDeviceType {
				nvSwitches = append(nvSwitches, strconv.Itoa(v.minor))
			}
		}
	}

	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	if len(passthroughDevices) > 0 {
		pod.Annotations[extunified.AnnotationRundPassthoughPCI] = strings.Join(passthroughDevices, ",")
	}
	if len(nvSwitches) > 0 {
		pod.Annotations[extunified.AnnotationRundNVSwitchOrder] = strings.Join(nvSwitches, ",")
	}

	if len(allocResult[schedulingv1alpha1.GPU]) > 0 {
		device, err := deviceLister.Get(pod.Spec.NodeName)
		if err != nil {
			return err
		}
		matchedVersion, err := MatchSupportedVMDriverVersions(pod, device)
		if err != nil {
			return nil
		}
		if matchedVersion == "" {
			return fmt.Errorf("unmatched driver versions")
		}
		pod.Annotations[extunified.AnnotationRundNvidiaDriverVersion] = matchedVersion
	}
	return nil
}

func isGPUSharedPod(resourceList corev1.ResourceList) bool {
	if !deviceshare.HasDeviceResource(resourceList, schedulingv1alpha1.GPU) {
		return false
	}
	quantity := resourceList[apiext.ResourceGPUMemoryRatio]
	memRatioAlloc := quantity.Value()
	quantity = resourceList[apiext.ResourceGPUCore]
	utilAlloc := quantity.Value()
	return memRatioAlloc < 100 || utilAlloc < 100
}

func MatchSupportedVMDriverVersions(pod *corev1.Pod, device *schedulingv1alpha1.Device) (string, error) {
	driverVersions, err := extunified.GetDriverVersions(device.Annotations)
	if err != nil {
		return "", err
	}
	return matchDriverVersions(pod, driverVersions)
}

func matchDriverVersions(pod *corev1.Pod, driverVersions extunified.NVIDIADriverVersions) (string, error) {
	if len(driverVersions) == 0 {
		return "", nil
	}
	sort.Strings(driverVersions)
	selector, err := extunified.GetGPUSelector(pod.Annotations)
	if err != nil {
		return "", err
	}
	if len(selector.DriverVersions) == 0 {
		return driverVersions[0], nil
	}

	versions := sets.NewString(driverVersions...)
	var matchedVersion string
	for _, v := range selector.DriverVersions {
		if versions.Has(v) {
			matchedVersion = v
			break
		}
	}
	return matchedVersion, nil
}

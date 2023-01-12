package resourcesummary

import (
	"context"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	cosv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

var GPURatioBasedResourceNames = map[corev1.ResourceName]struct{}{
	extension.KoordGPU:                     {},
	extension.GPUCore:                      {},
	extension.GPUMemoryRatio:               {},
	unifiedresourceext.GPUResourceAlibaba:  {},
	unifiedresourceext.GPUResourceCore:     {},
	unifiedresourceext.GPUResourceMemRatio: {},
}

var GPUMemBytesResourceNames = map[corev1.ResourceName]struct{}{
	extension.GPUMemory:               {},
	unifiedresourceext.GPUResourceMem: {},
}

func (r *Reconciler) getGPUCapacityForCandidateNodes(ctx context.Context, candidateNodes *corev1.NodeList,
) (map[string]corev1.ResourceList, error) {
	nodeOwnedGPUCapacity := map[string]corev1.ResourceList{}
	for _, node := range candidateNodes.Items {
		device := &schedulingv1alpha1.Device{}
		err := r.Client.Get(ctx, client.ObjectKey{Name: node.Name}, device)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
			unifiedDevice := &cosv1beta1.Device{}
			klog.V(5).Infof("ResourceSummary, getGPUCapacityForCandidateNodes, nodeName: %s", node.Name)
			err = r.Client.Get(ctx, client.ObjectKey{Name: node.Name}, unifiedDevice)
			if err != nil {
				klog.V(5).Infof("ResourceSummary, getGPUCapacityForCandidateNodes, get UnifiedDevice error: %s, nodeName: %s", err.Error(), node.Name)
				if !errors.IsNotFound(err) {
					return nil, err
				}
				return nil, nil
			}
			nodeOwnedGPUCapacity[node.Name] = getGPUCapacityFromUnifiedDevice(unifiedDevice)
			continue
		}
		nodeOwnedGPUCapacity[node.Name] = getGPUCapacityFromKoordDevice(device)
	}
	return nodeOwnedGPUCapacity, nil
}

func getGPUCapacityFromKoordDevice(device *schedulingv1alpha1.Device) corev1.ResourceList {
	if device == nil {
		return nil
	}
	capacity := corev1.ResourceList{}
	for _, deviceInfo := range device.Spec.Devices {
		if deviceInfo.Type == schedulingv1alpha1.GPU {
			capacity = quotav1.Add(capacity, deviceInfo.Resources)
		}
	}
	return extunified.ConvertToUnifiedGPUResources(capacity)
}

func getGPUCapacityFromUnifiedDevice(device *cosv1beta1.Device) corev1.ResourceList {
	klog.V(5).Infof("ResourceSummary getGPUCapacityFromUnifiedDevice, device: %+v", device)
	if device == nil {
		return nil
	}
	capacity := corev1.ResourceList{}
	for _, deviceInfo := range device.Spec.Devices {
		if deviceInfo.DeviceType == cosv1beta1.GPU {
			if deviceInfo.GPU != nil {
				for _, deviceSpec := range deviceInfo.GPU.List {
					for resourceName, quantity := range deviceSpec.Resources {
						capacity = quotav1.Add(capacity, corev1.ResourceList{corev1.ResourceName(resourceName): quantity})
					}
				}
			}
			continue
		}
	}
	klog.Infof("ResourceSummary getGPUCapacityFromUnifiedDevice, capacity: %+v", capacity)
	return capacity
}

func addGPUCapacityToNodeAllocatable(nodeAllocatable, gpuCapacity corev1.ResourceList) {
	if gpuMemRatio, ok := convergeToGPUMemRatioIfExists(gpuCapacity); ok {
		for resourceName := range GPURatioBasedResourceNames {
			nodeAllocatable[resourceName] = *resource.NewQuantity(gpuMemRatio, resource.DecimalSI)
		}
	}
	if gpuMemBytes, ok := convergeToGPUMemBytesIfExists(gpuCapacity); ok && gpuMemBytes != nil {
		for resourceName := range GPUMemBytesResourceNames {
			nodeAllocatable[resourceName] = *gpuMemBytes
		}
	}
	delete(nodeAllocatable, extension.NvidiaGPU)
	delete(nodeAllocatable, unifiedresourceext.GPUResourceEncode)
	delete(nodeAllocatable, unifiedresourceext.GPUResourceDecode)
}

func convergeToGPUMemBytesIfExists(resourceList corev1.ResourceList) (*resource.Quantity, bool) {
	if resourceList == nil || len(resourceList) == 0 {
		return nil, false
	}
	for resourceName := range GPUMemBytesResourceNames {
		if quantity, ok := resourceList[resourceName]; ok {
			return &quantity, true
		}
	}
	return nil, false
}

func fillGPUResource(gpuRequest, gpuCapacity corev1.ResourceList) {
	gpuMemRatio, hasGPU := convergeToGPUMemRatioIfExists(gpuRequest)
	if !hasGPU {
		return
	}
	for resourceName := range GPURatioBasedResourceNames {
		gpuRequest[resourceName] = *resource.NewQuantity(gpuMemRatio, resource.DecimalSI)
	}
	if gpuCapacity != nil {
		gpuMemoryRatioQuantity, ratioExists := gpuCapacity[unifiedresourceext.GPUResourceMemRatio]
		gpuMemoryQuantity, memBytesExists := gpuCapacity[unifiedresourceext.GPUResourceMem]
		if ratioExists && memBytesExists {
			memBytes := memRatioToBytes(gpuMemRatio, gpuMemoryQuantity, gpuMemoryRatioQuantity)
			gpuRequest[unifiedresourceext.GPUResourceMem] = memBytes
			gpuRequest[extension.GPUMemory] = memBytes
		}
	}
	delete(gpuRequest, extension.NvidiaGPU)
	delete(gpuRequest, unifiedresourceext.GPUResourceEncode)
	delete(gpuRequest, unifiedresourceext.GPUResourceDecode)
}

func convergeToGPUMemRatioIfExists(resourceList corev1.ResourceList) (int64, bool) {
	if resourceList == nil || len(resourceList) == 0 {
		return 0, false
	}
	for resourceName := range GPURatioBasedResourceNames {
		if quantity, ok := resourceList[resourceName]; ok {
			return quantity.Value(), true
		}
	}
	if quantity, ok := resourceList[extension.NvidiaGPU]; ok {
		return quantity.Value() * 100, true
	}
	return 0, false
}

func memRatioToBytes(ratio int64, nodeMemory, nodeRatio resource.Quantity) resource.Quantity {
	return *resource.NewQuantity(ratio*nodeMemory.Value()/nodeRatio.Value(), resource.BinarySI)
}

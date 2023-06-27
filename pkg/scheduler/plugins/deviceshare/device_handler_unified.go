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

	"github.com/spf13/pflag"
	cosv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	cosclientset "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	cosinformers "gitlab.alibaba-inc.com/cos/unified-resource-api/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
)

var EnableUnifiedDevice bool

func init() {
	pflag.BoolVar(&EnableUnifiedDevice, "enable-unified-device", EnableUnifiedDevice, "enable unified device, disable by default")
}

func registerUnifiedDeviceEventHandler(deviceCache *nodeDeviceCache, handle framework.Handle) {
	if !EnableUnifiedDevice {
		return
	}

	cosClientSet, ok := handle.(cosclientset.Interface)
	if !ok {
		kubeConfig := *handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		var err error
		cosClientSet, err = cosclientset.NewForConfig(&kubeConfig)
		if err != nil {
			klog.Fatalf("Failed to build COS ClientSet, err: %v", err)
		}
	}

	cosInformerFactory := cosinformers.NewSharedInformerFactoryWithOptions(cosClientSet, 0)
	cosDeviceInformer := cosInformerFactory.Scheduling().V1beta1().Devices().Informer()
	cosDeviceInformer.AddEventHandler(&unifiedDeviceEventHandler{deviceCache: deviceCache})
	cosInformerFactory.Start(context.TODO().Done())
	cosInformerFactory.WaitForCacheSync(context.TODO().Done())
}

type unifiedDeviceEventHandler struct {
	deviceCache *nodeDeviceCache
}

func (n *unifiedDeviceEventHandler) OnAdd(obj interface{}) {
	device, ok := obj.(*cosv1beta1.Device)
	if !ok {
		klog.Errorf("device cache add failed to parse, obj %T", obj)
		return
	}

	koordDevice := convertUnifiedDeviceToKoordDevice(device)
	n.deviceCache.updateNodeDevice(device.Name, koordDevice)
	klog.V(4).InfoS("device cache added", "Device", klog.KObj(device))
}

func (n *unifiedDeviceEventHandler) OnUpdate(oldObj, newObj interface{}) {
	_, oldOK := oldObj.(*cosv1beta1.Device)
	newDevice, newOK := newObj.(*cosv1beta1.Device)
	if !oldOK || !newOK {
		klog.Errorf("device cache update failed to parse, oldObj %T, newObj %T", oldObj, newObj)
		return
	}
	koordDevice := convertUnifiedDeviceToKoordDevice(newDevice)
	n.deviceCache.updateNodeDevice(newDevice.Name, koordDevice)
	klog.V(4).InfoS("device cache updated", "Device", klog.KObj(newDevice))
}

func (n *unifiedDeviceEventHandler) OnDelete(obj interface{}) {
	var device *cosv1beta1.Device
	switch t := obj.(type) {
	case *cosv1beta1.Device:
		device = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		device, ok = t.Obj.(*cosv1beta1.Device)
		if !ok {
			return
		}
	default:
		return
	}
	n.deviceCache.removeNodeDevice(device.Name)
	klog.V(4).InfoS("device cache deleted", "Device", klog.KObj(device))
}

func convertUnifiedDeviceToKoordDevice(unifiedDevice *cosv1beta1.Device) *schedulingv1alpha1.Device {
	device := &schedulingv1alpha1.Device{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: unifiedDevice.Annotations,
			Labels:      unifiedDevice.Labels,
			Name:        unifiedDevice.Name,
		},
	}
	for _, v := range unifiedDevice.Spec.Devices {
		var deviceInfo schedulingv1alpha1.DeviceInfo
		switch v.DeviceType {
		case cosv1beta1.GPU:
			for _, deviceSpec := range v.GPU.List {
				deviceInfo.Type = schedulingv1alpha1.GPU
				deviceInfo.UUID = deviceSpec.ID
				deviceInfo.Minor = pointer.Int32(deviceSpec.Minor)
				deviceInfo.Health = deviceSpec.Health
				resources := make(corev1.ResourceList)
				for resourceName, quantity := range deviceSpec.Resources {
					resources[corev1.ResourceName(resourceName)] = quantity
				}
				resources = unified.ConvertToKoordGPUResources(resources)
				deviceInfo.Resources = resources
				device.Spec.Devices = append(device.Spec.Devices, deviceInfo)
			}
		}
	}
	return device
}

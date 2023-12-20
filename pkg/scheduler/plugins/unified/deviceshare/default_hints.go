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
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/kubernetes/pkg/api/v1/resource"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

// NOTE：过去调度器为了支持 RunD 场景而内置了一些约定逻辑，这些逻辑现在可以通过 JointAllocate 和 DeviceAllocateHints 协议表述
// 后续这些逻辑将移植到 webhook 中
func fillDefaultDeviceAllocateHint(pod *corev1.Pod) (*corev1.Pod, error) {
	if !k8sfeature.DefaultFeatureGate.Enabled(features.EnableDefaultDeviceAllocateHint) {
		return pod, nil
	}
	hints, err := apiext.GetDeviceAllocateHints(pod.Annotations)
	if err != nil {
		return nil, err
	}
	if hints == nil {
		hints = apiext.DeviceAllocateHints{}
	}

	requests, _ := resource.PodRequestsAndLimits(pod)

	// RunD 不仅需要直通 GPU，还要根据型号决定是否直通 NVSwitch，比如 A100/A800 都可以直通 NVSwitch 实现 GPU 间互联
	if !declareNVSwitchHint(hints) &&
		HasDeviceResource(requests, schedulingv1alpha1.GPU) &&
		pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "rund" {
		if q := requests[extunified.ResourcePPU]; q.IsZero() {
			hints[extunified.NVSwitchDeviceType] = &apiext.DeviceHint{}
		}
	}

	// 需要直通 NVSwitch，需要强行塞 koordinator.sh/nvswitch 资源到 Pod Request 中，触发后续 DeviceShare 调度流程
	fakeResources := corev1.ResourceList{}
	if declareNVSwitchHint(hints) && !HasDeviceResource(requests, extunified.NVSwitchDeviceType) {
		fakeResources[extunified.NVSwitchResource] = *apiresource.NewQuantity(100, apiresource.DecimalSI)
	}

	if HasDeviceResource(requests, schedulingv1alpha1.RDMA) {
		rdmaHint := hints[schedulingv1alpha1.RDMA]
		if rdmaHint == nil {
			rdmaHint = &apiext.DeviceHint{}
			hints[schedulingv1alpha1.RDMA] = rdmaHint
		}

		// 内部场景中，/etc/sysconf/pcie_topo 文件中定义的 PF type 分为 pf_system 和 pf_worker 两种
		// 走 koord-scheduler 处理的 Pod 只需要 pf_worker 类型的 PF
		if rdmaHint.Selector == nil {
			rdmaHint.Selector = &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"type": string(extunified.RDMADeviceTypeWorker),
				},
			}
		}

		// 针对 Reservation 做了个特殊处理，目前 PAI 场景中，Reservation 只是用来配合 LRN 完成资源预留的，因此只支持 Reservation 申请 PF，不允许分配 VF
		// 其他的 Pod，都需要分配 VF。VF type 分为 vf_cpu 和 vf_gpu，还有一个是我们没有用到的 vf_storage;
		// 另外因为历史原因，过去没有 type，我们强行给这种没有 type 的 VF 分配了一个 vf_general 类型
		if !reservationutil.IsReservePod(pod) && shouldAllocateVF(pod) && rdmaHint.VFSelector == nil {
			vfType := string(extunified.VFDeviceTypeCPU)
			hasGPU := HasDeviceResource(requests, schedulingv1alpha1.GPU)
			if hasGPU {
				vfType = string(extunified.VFDeviceTypeGPU)
			}
			rdmaHint.VFSelector = &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "type",
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							vfType,
							string(extunified.VFDeviceTypeGeneral),
						},
					},
				},
			}
		} else if pod.Spec.HostNetwork && (pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "rund") {
			// PAI-Damo 场景里，都是hostNetwork，也没有使用SR-IOV虚拟化，所有Pod共享整机PF。
			// 这里其实有待商榷的，但是内部就这么一个特殊场景
			if rdmaHint.AllocateStrategy == "" {
				rdmaHint.AllocateStrategy = apiext.ApplyForAllDeviceAllocateStrategy
			}
		}
	}

	jointAllocate, err := apiext.GetDeviceJointAllocate(pod.Annotations)
	if err != nil {
		return nil, err
	}

	if jointAllocate == nil &&
		HasDeviceResource(requests, schedulingv1alpha1.GPU) &&
		HasDeviceResource(requests, schedulingv1alpha1.RDMA) {
		// 强行要求 GPU 走联合分配逻辑，按拓扑分配
		jointAllocate = &apiext.DeviceJointAllocate{
			DeviceTypes: []schedulingv1alpha1.DeviceType{
				schedulingv1alpha1.GPU,
			},
		}
		if !pod.Spec.HostNetwork {
			// 像 PAI-Damo 这类，不需要走联合分配RDMA，但是PAI-serverless 就需要了。
			jointAllocate.DeviceTypes = append(jointAllocate.DeviceTypes, schedulingv1alpha1.RDMA)
		}
	}

	pod = pod.DeepCopy()
	if !quotav1.IsZero(fakeResources) {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{
			Name: "__koordinator_unified_device_share_fake_container__",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					extunified.NVSwitchResource: *apiresource.NewQuantity(100, apiresource.DecimalSI),
				},
			},
		})
	}
	if err := apiext.SetDeviceAllocateHints(pod, hints); err != nil {
		return nil, err
	}
	if jointAllocate != nil {
		if err := apiext.SetDeviceJointAllocate(pod, jointAllocate); err != nil {
			return nil, err
		}
	}
	return pod, nil
}

func declareNVSwitchHint(hint apiext.DeviceAllocateHints) bool {
	return hint[extunified.NVSwitchDeviceType] != nil
}

func shouldAllocateVF(pod *corev1.Pod) bool {
	return !pod.Spec.HostNetwork || (pod.Spec.RuntimeClassName != nil && *pod.Spec.RuntimeClassName == "rund")
}

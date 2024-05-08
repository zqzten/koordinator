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

package volumebinding

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/eci"
)

const (
	ErrInvalidDiskInfos             = "node(s) invalid disk infos"
	ErrInsufficientEphemeralStorage = "Insufficient ephemeral-storage"
	ErrInsufficientLocalVolume      = "Insufficient local-volume"
	ErrInsufficientIOLimit          = "Insufficient IOLimit"
	ERrInvalidIOLimitInfo           = "node(s) invalid IOLimit infos"
)

func GetDefaultStorageClassIfVirtualKubelet(node *corev1.Node, pod *corev1.Pod, storageClassName string, classLister storagelisters.StorageClassLister) string {
	if !unified.AffinityECI(pod) || !unified.IsVirtualKubeletNode(node) {
		return storageClassName
	}
	if !unified.SupportLocalPV(classLister, storageClassName, nil) && !unified.SupportLVMOrQuotaPathOrDevice(classLister, storageClassName, nil) {
		return storageClassName
	}
	if eci.DefaultECIProfile.DefaultStorageClass == "" {
		return storageClassName
	}
	return eci.DefaultECIProfile.DefaultStorageClass
}

func ReplaceStorageClassIfVirtualKubelet(node *corev1.Node, pod *corev1.Pod, claim *corev1.PersistentVolumeClaim, classLister storagelisters.StorageClassLister) *corev1.PersistentVolumeClaim {
	className := storagehelpers.GetPersistentVolumeClaimClass(claim)
	if className == "" {
		return claim
	}

	defaultClassName := GetDefaultStorageClassIfVirtualKubelet(node, pod, className, classLister)
	if defaultClassName != className {
		claim = claim.DeepCopy()
		claim.Spec.StorageClassName = &defaultClassName
		if _, ok := claim.Annotations[corev1.BetaStorageClassAnnotation]; ok {
			claim.Annotations[corev1.BetaStorageClassAnnotation] = defaultClassName
		}
	}
	return claim
}

func HasEnoughStorageCapacity(nodeStorageInfo *NodeStorageInfo, pod *v1.Pod, systemDiskInBytes int64, localVolumesInBytes map[string]int64, storageClassLister storagelisters.StorageClassLister, preemptiveUsedStorage int64) *framework.Status {
	requiredStorageInBytes := systemDiskInBytes + GetRequestedStorageInBytes(pod, storageClassLister)

	localVolumeFree := nodeStorageInfo.GetFree()
	if requiredStorageInBytes > 0 {
		graphDiskPath := nodeStorageInfo.GraphDiskPath
		graphDiskFree := localVolumeFree.VolumeSize[graphDiskPath] + preemptiveUsedStorage
		if requiredStorageInBytes > graphDiskFree {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientEphemeralStorage)
		}
	}

	for hostPath, sizeInBytes := range localVolumesInBytes {
		if sizeInBytes > 0 && sizeInBytes > localVolumeFree.VolumeSize[hostPath] {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientLocalVolume)
		}
	}

	return nil
}

func isAllIoLimitRequestZero(reqIoLimit map[string]*unified.LocalStorageIOLimitInfo) bool {
	res := true
	for _, ioLimit := range reqIoLimit {
		if !ioLimit.IsZero() {
			res = false
		}
	}
	return res
}

func HasEnoughIOCapacity(
	nodeStorageInfo *NodeStorageInfo,
	pod *v1.Pod,
	reqIoLimit map[string]*unified.LocalStorageIOLimitInfo,
) *framework.Status {
	if len(reqIoLimit) == 0 {
		return nil
	}
	// 如果 ioLimit 请求为 0，跳过 iolimit 检查
	if isAllIoLimitRequestZero(reqIoLimit) {
		return nil
	}

	localVolumeFree := nodeStorageInfo.GetFree()
	if localVolumeFree.IOLimits == nil {
		return framework.NewStatus(framework.Unschedulable, ERrInvalidIOLimitInfo)
	}

	nodeIOLimit := localVolumeFree.IOLimits
	for hostPath, ioLimit := range reqIoLimit {
		if ioLimit == nil {
			continue
		}
		if len(nodeIOLimit) == 0 {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientIOLimit)
		}
		if ioLimit.ReadIops > 0 && nodeIOLimit[hostPath] != nil && ioLimit.ReadIops > nodeIOLimit[hostPath].ReadIops {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientIOLimit)
		}
		if ioLimit.WriteIops > 0 && nodeIOLimit[hostPath] != nil && ioLimit.WriteIops > nodeIOLimit[hostPath].WriteIops {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientIOLimit)
		}
		if ioLimit.ReadBps > 0 && nodeIOLimit[hostPath] != nil && ioLimit.ReadBps > nodeIOLimit[hostPath].ReadBps {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientIOLimit)
		}
		if ioLimit.WriteBps > 0 && nodeIOLimit[hostPath] != nil && ioLimit.WriteBps > nodeIOLimit[hostPath].WriteBps {
			return framework.NewStatus(framework.Unschedulable, ErrInsufficientIOLimit)
		}
	}
	return nil
}

func GetPVSelectedNode(pv *corev1.PersistentVolume) string {
	selectedNode := pv.Annotations[storagehelpers.AnnSelectedNode]
	if selectedNode != "" {
		return selectedNode
	}
	if pv.Spec.NodeAffinity != nil && pv.Spec.NodeAffinity.Required != nil {
		for _, term := range pv.Spec.NodeAffinity.Required.NodeSelectorTerms {
			if len(term.MatchExpressions) == 0 {
				continue
			}
			labelSelector, err := nodeSelectorRequirementsAsSelector(term.MatchExpressions)
			if err != nil {
				continue
			}
			value, found := labelSelector.RequiresExactMatch("topology.sigma.ali/node")
			if found {
				return value
			}
		}
	}
	return ""
}

// nodeSelectorRequirementsAsSelector converts the []NodeSelectorRequirement api type into a struct that implements
// labels.Selector.
func nodeSelectorRequirementsAsSelector(nsm []corev1.NodeSelectorRequirement) (labels.Selector, error) {
	if len(nsm) == 0 {
		return labels.Nothing(), nil
	}
	selector := labels.NewSelector()
	for _, expr := range nsm {
		var op selection.Operator
		switch expr.Operator {
		case corev1.NodeSelectorOpIn:
			op = selection.In
		case corev1.NodeSelectorOpNotIn:
			op = selection.NotIn
		case corev1.NodeSelectorOpExists:
			op = selection.Exists
		case corev1.NodeSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		case corev1.NodeSelectorOpGt:
			op = selection.GreaterThan
		case corev1.NodeSelectorOpLt:
			op = selection.LessThan
		default:
			return nil, fmt.Errorf("%q is not a valid node selector operator", expr.Operator)
		}
		r, err := labels.NewRequirement(expr.Key, op, expr.Values)
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*r)
	}
	return selector, nil
}

func GetPVCIOLimitInfo(pvc *corev1.PersistentVolumeClaim) (*unified.LocalStorageIOLimitInfo, error) {
	pvcIOLimitInfo := &unified.LocalStorageIOLimitInfo{}
	pvcIOLimit, ok := pvc.Annotations[unified.IOLimitAnnotationOnPVC]
	// if pvc does not have iolimit, iolimit values are 0
	if !ok {
		return pvcIOLimitInfo, nil
	}
	if err := json.Unmarshal([]byte(pvcIOLimit), &pvcIOLimitInfo); err != nil {
		klog.Errorf("unmarshal pvc io limit info from pvc[%s/%s] failed:，%v", pvc.Namespace, pvc.Name, err)
		return nil, err
	}
	return pvcIOLimitInfo, nil
}

func GetRequestedStorageInBytes(pod *corev1.Pod, classLister storagelisters.StorageClassLister) int64 {
	requests := resource.PodRequests(pod, resource.PodResourcesOptions{})
	q := requests[v1.ResourceEphemeralStorage]
	requiredStorageInBytes := q.Value()
	requiredStorageInBytes += unified.CalcLocalInlineVolumeSize(pod.Spec.Volumes, classLister)
	return requiredStorageInBytes
}

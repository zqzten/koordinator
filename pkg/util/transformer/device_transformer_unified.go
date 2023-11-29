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

package transformer

import (
	"strconv"

	"k8s.io/apimachinery/pkg/util/sets"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
)

func init() {
	deviceTransformers = append(deviceTransformers, TransformDeviceTopology)
}

func TransformDeviceTopology(deviceObj *schedulingv1alpha1.Device) {
	if !utilfeature.DefaultFeatureGate.Enabled(features.EnableTransformerDeviceTopology) {
		return
	}

	deviceTopology, err := unified.GetDeviceTopology(deviceObj.Annotations)
	if err != nil {
		return
	}

	pciInfos, err := unified.GetDevicePCIInfos(deviceObj.Annotations)
	if err != nil {
		return
	}
	pciInfoMap := map[schedulingv1alpha1.DeviceType]map[int32]unified.DevicePCIInfo{}
	for _, v := range pciInfos {
		m := pciInfoMap[v.Type]
		if m == nil {
			m = map[int32]unified.DevicePCIInfo{}
			pciInfoMap[v.Type] = m
		}
		m[v.Minor] = v
	}
	rdmaTopology, err := unified.GetRDMATopology(deviceObj.Annotations)
	if err != nil {
		return
	}

	rdmaLabels := map[int32]map[string]string{}
	for _, socket := range deviceTopology.NUMASockets {
		for _, node := range socket.NUMANodes {
			for _, pcie := range node.PCIESwitches {
				minorsByType := map[schedulingv1alpha1.DeviceType]sets.Int32{}
				minorsByType[schedulingv1alpha1.GPU] = sets.NewInt32(pcie.GPUs...)
				rdmaMinors := sets.NewInt32()
				minorsByType[schedulingv1alpha1.RDMA] = rdmaMinors
				for _, rdma := range pcie.RDMAs {
					rdmaMinors.Insert(rdma.Minor)
					rdmaType := rdma.Type
					if rdmaType == "" {
						rdmaType = unified.RDMADeviceTypeWorker
					}
					labels := map[string]string{
						"type": string(rdmaType),
					}
					rdmaLabels[rdma.Minor] = labels
				}
				for i := range deviceObj.Spec.Devices {
					deviceInfo := &deviceObj.Spec.Devices[i]
					minor := pointer.Int32Deref(deviceInfo.Minor, 0)
					minors := minorsByType[deviceInfo.Type]
					if minors.Has(minor) && deviceInfo.Topology == nil {
						pciInfo := pciInfoMap[deviceInfo.Type]
						deviceInfo.Topology = &schedulingv1alpha1.DeviceTopology{
							SocketID: socket.Index,
							NodeID:   node.Index,
							PCIEID:   strconv.Itoa(int(pcie.Index)),
							BusID:    pciInfo[minor].BusID,
						}
					}
				}
			}
		}
	}

	for i := range deviceObj.Spec.Devices {
		deviceInfo := &deviceObj.Spec.Devices[i]
		minor := pointer.Int32Deref(deviceInfo.Minor, 0)
		if deviceInfo.Type == schedulingv1alpha1.RDMA {
			deviceInfo.Labels = rdmaLabels[minor]
			vfGroup := map[string][]schedulingv1alpha1.VirtualFunction{}
			vfs := rdmaTopology.VFs[minor]
			for _, vf := range vfs {
				vfType := vf.Type
				if vfType == "" {
					vfType = unified.VFDeviceTypeGeneral
				}
				vfGroup[string(vfType)] = append(vfGroup[string(vfType)], schedulingv1alpha1.VirtualFunction{
					Minor: vf.Minor,
					BusID: vf.BusID,
				})
			}
			for vfType, group := range vfGroup {
				deviceInfo.VFGroups = append(deviceInfo.VFGroups, schedulingv1alpha1.VirtualFunctionGroup{
					Labels: map[string]string{
						"type": vfType,
					},
					VFs: group,
				})
			}
		}
	}
}

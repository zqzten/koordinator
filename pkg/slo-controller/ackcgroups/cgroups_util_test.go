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

package cgroups

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func containerInfoSpecEqual(a, b *resourcesv1alpha1.ContainerInfoSpec) (bool, string) {
	if a.Name != b.Name {
		return false, fmt.Sprintf("NodeName not equal %v!=%v", a.Name, b.Name)
	}
	if a.NodeName != b.NodeName {
		return false, fmt.Sprintf("NodeName not equal %v!=%v", a.NodeName, b.NodeName)
	}
	if a.CPUSet != b.CPUSet {
		return false, fmt.Sprintf("cpu set not equal %v!=%v", a.CPUSet, b.CPUSet)
	}
	if !reflect.DeepEqual(a.LLCSpec, b.LLCSpec) {
		return false, fmt.Sprintf("llc not equal %v!=%v", a.LLCSpec, b.LLCSpec)
	}
	if !reflect.DeepEqual(a.BlkioSpec, b.BlkioSpec) {
		return false, fmt.Sprintf("BlkioSpec not equal %v!=%v", a.BlkioSpec, b.BlkioSpec)
	}
	if a.Cpu.MilliValue() != b.Cpu.MilliValue() {
		return false, fmt.Sprintf("Cpu not equal %v!=%v", a.Cpu.MilliValue(), b.Cpu.MilliValue())
	}
	if a.Memory.Value() != b.Memory.Value() {
		return false, fmt.Sprintf("Memory not equal %v!=%v", a.Memory.Value(), b.Memory.Value())
	}
	if !reflect.DeepEqual(a.CpuSetSpec, b.CpuSetSpec) {
		return false, fmt.Sprintf("CpuSetSpec set not equal %v!=%v", a.CpuSetSpec, b.CpuSetSpec)
	}
	if !reflect.DeepEqual(a.CpuAcctSpec, b.CpuAcctSpec) {
		return false, fmt.Sprintf("CpuAcctSpec set not equal %v!=%v", a.CpuAcctSpec, b.CpuAcctSpec)
	}
	return true, ""
}

func containerCgroupListEqual(a, b []*resourcesv1alpha1.ContainerInfoSpec) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("container list length not equal %v!=%v", len(a), len(b))
	}
	for i := range a {
		if equal, msg := containerInfoSpecEqual(a[i], b[i]); !equal {
			return false, fmt.Sprintf("container item %v not equal, detail:%v", i, msg)
		}
	}
	return true, ""
}

func podCgroupsListEqual(a, b []*resourcesv1alpha1.PodCgroupsInfo) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("pod list length not equal %v!=%v", len(a), len(b))
	}
	for i := range a {
		if a[i].PodNamespace != b[i].PodNamespace || a[i].PodName != b[i].PodName {
			return false, fmt.Sprintf("pod item %v meta not equal, detail:\n%+v\n%+v", i, a[i], b[i])
		}
		if equal, msg := containerCgroupListEqual(a[i].Containers, b[i].Containers); !equal {
			return false, fmt.Sprintf("pod item %v not equal, detail:%v", i, msg)
		}
	}
	return true, ""
}

func nodeCgroupsListEqual(a, b []*resourcesv1alpha1.NodeCgroupsInfo) (bool, string) {
	if len(a) != len(b) {
		return false, fmt.Sprintf("node list length not equal %v!=%v", len(a), len(b))
	}
	for i := range a {
		if a[i].CgroupsNamespace != b[i].CgroupsNamespace || a[i].CgroupsName != b[i].CgroupsName {
			return false, fmt.Sprintf("node item %v meta not equal, detail:\n%+v\n%+v", i, a[i], b[i])
		}
		if equal, msg := podCgroupsListEqual(a[i].PodCgroups, b[i].PodCgroups); !equal {
			return false, fmt.Sprintf("node item %v not equal, detail:%v", i, msg)
		}
	}
	return true, ""
}

func Test_insertOrReplaceContainerSpecToNode(t *testing.T) {
	testContainer1 := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	type args struct {
		cgroupsNamespace string
		cgroupName       string
		pod              *corev1.Pod
		containerSpec    []resourcesv1alpha1.ContainerInfoSpec
		nodeCgroups      []*resourcesv1alpha1.NodeCgroupsInfo
	}
	tests := []struct {
		name        string
		args        args
		want        []*resourcesv1alpha1.NodeCgroupsInfo
		wantUpdated bool
	}{
		{
			name: "insert new to node cgoups",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupName:       testCgroupName,
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testPodNs,
						Name:      testPodName,
					},
				},
				containerSpec: []resourcesv1alpha1.ContainerInfoSpec{
					testContainer1,
				},
				nodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{},
			},
			want: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								&testContainer1,
							},
						},
					},
				},
			},
			wantUpdated: true,
		},
		{
			name: "insert duplicate to node cgoups",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupName:       testCgroupName,
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testPodNs,
						Name:      testPodName,
					},
				},
				containerSpec: []resourcesv1alpha1.ContainerInfoSpec{
					testContainer1,
				},
				nodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									&testContainer1,
								},
							},
						},
					},
				},
			},
			want: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								&testContainer1,
							},
						},
					},
				},
			},
			wantUpdated: false,
		},
		{
			name: "insert already exist to node cgoups",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupName:       testCgroupName,
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testPodNs,
						Name:      testPodName,
					},
				},
				containerSpec: []resourcesv1alpha1.ContainerInfoSpec{
					testContainer1,
				},
				nodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      "another-cgroups-name",
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									&testContainer1,
								},
							},
						},
					},
				},
			},
			want: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								&testContainer1,
							},
						},
					},
				},
			},
			wantUpdated: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotUpdated := insertOrReplaceContainerSpecToNode(tt.args.cgroupsNamespace, tt.args.cgroupName, tt.args.pod,
				tt.args.containerSpec, tt.args.nodeCgroups)
			if equal, msg := nodeCgroupsListEqual(got, tt.want); !equal {
				t.Errorf("insertOrReplaceContainerSpecToNode() not equal, message %v", msg)
			}
			if gotUpdated != tt.wantUpdated {
				t.Errorf("insertOrReplaceContainerSpecToNode() updated not equal, want %v, got %v",
					tt.wantUpdated, gotUpdated)
			}
		})
	}
}

func Test_insertPodCgroupToNode(t *testing.T) {
	testContainer1 := &resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	type args struct {
		cgroupsNamespace string
		cgroupName       string
		newPodCgroup     *resourcesv1alpha1.PodCgroupsInfo
		oldNodeCgroups   []*resourcesv1alpha1.NodeCgroupsInfo
	}
	tests := []struct {
		name string
		args args
		want []*resourcesv1alpha1.NodeCgroupsInfo
	}{
		{
			name: "insert new to node",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupName:       testCgroupName,
				newPodCgroup: &resourcesv1alpha1.PodCgroupsInfo{
					PodNamespace: testPodNs,
					PodName:      testPodName,
					Containers: []*resourcesv1alpha1.ContainerInfoSpec{
						testContainer1,
					},
				},
				oldNodeCgroups: nil,
			},
			want: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
		},
		{
			name: "insert duplicate to node",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupName:       testCgroupName,
				newPodCgroup: &resourcesv1alpha1.PodCgroupsInfo{
					PodNamespace: testPodNs,
					PodName:      testPodName,
					Containers: []*resourcesv1alpha1.ContainerInfoSpec{
						testContainer1,
					},
				},
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			want: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insertPodCgroupToNode(tt.args.cgroupsNamespace, tt.args.cgroupName, tt.args.newPodCgroup,
				tt.args.oldNodeCgroups)
			if equal, msg := nodeCgroupsListEqual(got, tt.want); !equal {
				t.Errorf("insertPodCgroupToNode() not equal, msg: %v", msg)
			}
		})
	}
}

func Test_removeOneEntireCgroupsIfExist(t *testing.T) {
	testContainer1 := &resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	type args struct {
		cgroupsNamespace string
		cgroupsName      string
		oldNodeCgroups   []*resourcesv1alpha1.NodeCgroupsInfo
	}
	tests := []struct {
		name           string
		args           args
		wantNodeCgroup []*resourcesv1alpha1.NodeCgroupsInfo
		wantExist      bool
	}{
		{
			name: "remove exist in node cgroup",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupsName:      testCgroupName,
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{},
			wantExist:      true,
		},
		{
			name: "remove not exist in node cgroup",
			args: args{
				cgroupsNamespace: testCgroupNs,
				cgroupsName:      testCgroupName,
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: "test-cgroup-ns-not-exist",
						CgroupsName:      "test-cgroup-jobName-not-exist",
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: "test-cgroup-ns-not-exist",
					CgroupsName:      "test-cgroup-jobName-not-exist",
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
			wantExist: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodeCgroup, gotExist := removeOneEntireCgroupsIfExist(tt.args.cgroupsNamespace, tt.args.cgroupsName, tt.args.oldNodeCgroups)
			if equal, msg := nodeCgroupsListEqual(gotNodeCgroup, tt.wantNodeCgroup); !equal {
				t.Errorf("removeOneEntireCgroupsIfExist() not equal, msg %v", msg)
			}
			if gotExist != tt.wantExist {
				t.Errorf("removeOneEntireCgroupsIfExist() got1 = %v, wantNodeCgroup %v", gotExist, tt.wantExist)
			}
		})
	}
}

func Test_removeEntireCgroupsIfExist(t *testing.T) {
	testContainer1 := &resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	type args struct {
		cgroupsNamespacedName map[string]struct{}
		oldNodeCgroups        []*resourcesv1alpha1.NodeCgroupsInfo
	}
	tests := []struct {
		name           string
		args           args
		wantNodeCgroup []*resourcesv1alpha1.NodeCgroupsInfo
		wantExist      bool
	}{
		{
			name: "remove exist in node cgorup",
			args: args{
				cgroupsNamespacedName: map[string]struct{}{
					types.NamespacedName{Namespace: testCgroupNs, Name: testCgroupName}.String(): {},
				},
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{},
			wantExist:      true,
		},
		{
			name: "remove not exist in node cgorup",
			args: args{
				cgroupsNamespacedName: map[string]struct{}{
					types.NamespacedName{Namespace: testCgroupNs, Name: testCgroupName}.String(): {},
				},
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: "test-cgroup-ns-not-exist",
						CgroupsName:      "test-cgroup-jobName-not-exist",
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: "test-cgroup-ns-not-exist",
					CgroupsName:      "test-cgroup-jobName-not-exist",
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
			wantExist: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodeCgroup, gotExist := removeEntireCgroupsIfExist(tt.args.cgroupsNamespacedName, tt.args.oldNodeCgroups)
			if equal, msg := nodeCgroupsListEqual(gotNodeCgroup, tt.wantNodeCgroup); !equal {
				t.Errorf("removeEntireCgroupsIfExist() not equal, msg %v", msg)
			}
			if gotExist != tt.wantExist {
				t.Errorf("removeEntireCgroupsIfExist() got1 = %v, want %v", gotExist, tt.wantExist)
			}
		})
	}
}

func Test_removeOnePodCgroupFromNodeIfExist(t *testing.T) {
	testContainer1 := &resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	testPodNs2 := "test-pod-ns2"
	testPodName2 := "test-pod-name2"
	type args struct {
		podNamespace   string
		podName        string
		oldNodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo
	}
	tests := []struct {
		name           string
		args           args
		wantNodeCgroup []*resourcesv1alpha1.NodeCgroupsInfo
		wantExist      bool
	}{
		{
			name: "remove exist in node group",
			args: args{
				podNamespace: testPodNs,
				podName:      testPodName,
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{},
			wantExist:      true,
		},
		{
			name: "remove exist with remain in node group",
			args: args{
				podNamespace: testPodNs,
				podName:      testPodName,
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
							{
								PodNamespace: testPodNs2,
								PodName:      testPodName2,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs2,
							PodName:      testPodName2,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
			wantExist: true,
		},
		{
			name: "remove not exist in node group",
			args: args{
				podNamespace: "test-pod-ns-not-exist",
				podName:      "test-pod-jobName-not-exist",
				oldNodeCgroups: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
			wantExist: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodeCgroup, gotExist := removeOnePodCgroupFromNodeIfExist(tt.args.podNamespace, tt.args.podName, tt.args.oldNodeCgroups)
			if equal, msg := nodeCgroupsListEqual(gotNodeCgroup, tt.wantNodeCgroup); !equal {
				t.Errorf("removeOnePodCgroupFromNodeIfExist() not equal, msg %v", msg)
			}
			if gotExist != tt.wantExist {
				t.Errorf("removeOnePodCgroupFromNodeIfExist() got1 = %v, want %v", gotExist, tt.wantExist)
			}
		})
	}
}

func Test_removePodCgroupsFromNodeIfExist(t *testing.T) {
	testContainer1 := &resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	testPodNs2 := "test-pod-ns2"
	testPodName2 := "test-pod-name2"
	type args struct {
		podsNamespacedName map[string]struct{}
		oldNodeCgroupsList []*resourcesv1alpha1.NodeCgroupsInfo
	}
	tests := []struct {
		name           string
		args           args
		wantNodeCgroup []*resourcesv1alpha1.NodeCgroupsInfo
		wantExist      bool
	}{
		{
			name: "remove exist in node group",
			args: args{
				podsNamespacedName: map[string]struct{}{
					types.NamespacedName{Namespace: testPodNs, Name: testPodName}.String(): {},
				},
				oldNodeCgroupsList: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{},
			wantExist:      true,
		},
		{
			name: "remove exist with remain in node group",
			args: args{
				podsNamespacedName: map[string]struct{}{
					types.NamespacedName{Namespace: testPodNs, Name: testPodName}.String(): {},
				},
				oldNodeCgroupsList: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
							{
								PodNamespace: testPodNs2,
								PodName:      testPodName2,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs2,
							PodName:      testPodName2,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
			wantExist: true,
		},
		{
			name: "remove not exist in node group",
			args: args{
				podsNamespacedName: map[string]struct{}{
					types.NamespacedName{Namespace: "test-pod-ns-not-exist", Name: "test-pod-jobName-not-exist"}.String(): {},
				},
				oldNodeCgroupsList: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
			},
			wantNodeCgroup: []*resourcesv1alpha1.NodeCgroupsInfo{
				{
					CgroupsNamespace: testCgroupNs,
					CgroupsName:      testCgroupName,
					PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
						{
							PodNamespace: testPodNs,
							PodName:      testPodName,
							Containers: []*resourcesv1alpha1.ContainerInfoSpec{
								testContainer1,
							},
						},
					},
				},
			},
			wantExist: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNodeCgroup, gotExist := removePodCgroupsFromNodeIfExist(tt.args.podsNamespacedName, tt.args.oldNodeCgroupsList)
			if equal, msg := nodeCgroupsListEqual(gotNodeCgroup, tt.wantNodeCgroup); !equal {
				t.Errorf("removePodCgroupsFromNodeIfExist() not equal, msg %v", msg)
			}
			if gotExist != tt.wantExist {
				t.Errorf("removePodCgroupsFromNodeIfExist() gotExist = %v, want %v", gotExist, tt.wantExist)
			}
		})
	}
}

func Test_cgroupsSub(t *testing.T) {
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	testDepNs := "test-dep-ns"
	testDepName := "test-dep-jobName"
	testJobNs := "test-job-ns"
	testJobName := "test-job-jobName"
	type args struct {
		l *resourcesv1alpha1.Cgroups
		r *resourcesv1alpha1.Cgroups
	}
	tests := []struct {
		name string
		args args
		want *resourcesv1alpha1.Cgroups
	}{
		{
			name: "cgroup sub",
			args: args{
				l: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testCgroupName,
						Namespace: testCgroupNs,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodNs,
							Namespace: testPodName,
						},
						DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
							Name:      testDepNs,
							Namespace: testDepName,
						},
					},
				},
				r: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testCgroupName,
						Namespace: testCgroupNs,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodNs,
							Namespace: testPodName,
						},
						JobInfo: resourcesv1alpha1.JobInfoSpec{
							Name:      testJobNs,
							Namespace: testJobName,
						},
					},
				},
			},
			want: &resourcesv1alpha1.Cgroups{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testCgroupName,
					Namespace: testCgroupNs,
				},
				Spec: resourcesv1alpha1.CgroupsSpec{
					DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
						Name:      testDepNs,
						Namespace: testDepName,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cgroupsSub(tt.args.l, tt.args.r); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cgroupsSub() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_containsCgroups(t *testing.T) {
	testContainer1 := &resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container",
		Cpu:    *resource.NewQuantity(2, resource.DecimalSI),
		Memory: *resource.NewQuantity(4096, resource.BinarySI),
	}
	testCgroupNs := "test-cgroup-ns"
	testCgroupName := "test-cgroup-jobName"
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	type args struct {
		nodeCgroupsList []*resourcesv1alpha1.NodeCgroupsInfo
		cgroups         *resourcesv1alpha1.Cgroups
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "contains cgroup",
			args: args{
				nodeCgroupsList: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
				cgroups: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testCgroupName,
						Namespace: testCgroupNs,
					},
				},
			},
			want: true,
		},
		{
			name: "contains cgroup",
			args: args{
				nodeCgroupsList: []*resourcesv1alpha1.NodeCgroupsInfo{
					{
						CgroupsNamespace: testCgroupNs,
						CgroupsName:      testCgroupName,
						PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
							{
								PodNamespace: testPodNs,
								PodName:      testPodName,
								Containers: []*resourcesv1alpha1.ContainerInfoSpec{
									testContainer1,
								},
							},
						},
					},
				},
				cgroups: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cgroup-jobName-not-exist",
						Namespace: "test-cgroup-ns-not-exist",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsCgroups(tt.args.nodeCgroupsList, tt.args.cgroups); got != tt.want {
				t.Errorf("containsCgroups() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_matchCgroupsPodInfoSpec(t *testing.T) {
	testPodNs := "test-pod-ns"
	testPodName := "test-pod-jobName"
	type args struct {
		pod     *corev1.Pod
		podInfo *resourcesv1alpha1.PodInfoSpec
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "pod match",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testPodName,
						Namespace: testPodNs,
					},
				},
				podInfo: &resourcesv1alpha1.PodInfoSpec{
					Name:      testPodName,
					Namespace: testPodNs,
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchCgroupsPodInfoSpec(tt.args.pod, tt.args.podInfo); got != tt.want {
				t.Errorf("matchCgroupsPodInfoSpec() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_matchCgroupsDeploymentInfoSpc(t *testing.T) {
	testDepNs := "test-dep-ns"
	testDepName := "test-dep-jobName"
	type args struct {
		deployment     *appv1.Deployment
		deploymentInfo *resourcesv1alpha1.DeploymentInfoSpec
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "deployment match",
			args: args{
				deployment: &appv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testDepName,
						Namespace: testDepNs,
					},
				},
				deploymentInfo: &resourcesv1alpha1.DeploymentInfoSpec{
					Name:      testDepName,
					Namespace: testDepNs,
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchCgroupsDeploymentInfoSpec(tt.args.deployment, tt.args.deploymentInfo); got != tt.want {
				t.Errorf("matchCgroupsDeploymentInfoSpec() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_matchCgroupsJobInfoSpec(t *testing.T) {
	testJobNs := "test-job-ns"
	testJobName := "test-job-jobName"
	type args struct {
		pod     *corev1.Pod
		jobInfo *resourcesv1alpha1.JobInfoSpec
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "job match",
			args: args{
				pod: &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-jobName",
						Namespace: testJobNs,
						Labels: map[string]string{
							PodLabelKeyJobName: testJobName,
						},
					},
				},
				jobInfo: &resourcesv1alpha1.JobInfoSpec{
					Name:      testJobName,
					Namespace: testJobNs,
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchCgroupsJobInfoSpec(tt.args.pod, tt.args.jobInfo); got != tt.want {
				t.Errorf("matchCgroupsJobInfoSpec() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getPodsByJobInfo(t *testing.T) {
	testNs := "test-ns"
	testJobName := "test-job-name"
	testPodName := "test-pod-name"
	type args struct {
		obj          []runtime.Object
		jobNamespace string
		jobName      string
	}
	tests := []struct {
		name    string
		args    args
		want    []*corev1.Pod
		wantErr bool
	}{
		{
			name: "get pods of job",
			args: args{
				obj: []runtime.Object{
					newJob(testNs, testJobName),
					newPod(testNs, testPodName, map[string]string{PodLabelKeyJobName: testJobName}),
				},
				jobNamespace: testNs,
				jobName:      testJobName,
			},
			want: []*corev1.Pod{
				newPod(testNs, testPodName, map[string]string{PodLabelKeyJobName: testJobName}),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.args.obj...)
			client := clientBuilder.Build()
			got, err := getPodsByJobInfo(client, tt.args.jobNamespace, tt.args.jobName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPodsByJobInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got, "getPodsByJobInfo")
		})
	}
}

func newJob(namespace, name string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: "1",
		},
	}
}

func newPod(namespace, name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			ResourceVersion: "1",
		},
	}
}

func Test_getPodsByDeployment(t *testing.T) {
	testNs := "test-ns"
	testDepName := "test-dep-name"
	testDepUid := "test-dep-uid"
	testRsName := "test-rs-name"
	testRsUid := "test-rs-uid"
	testPodName := "test-pod-name"

	selectorLabels := map[string]string{
		"deployment-name": testDepName,
	}

	testDepSelector := &metav1.LabelSelector{
		MatchLabels: selectorLabels,
	}

	testOwnerDep := metav1.OwnerReference{
		Kind: ownerKindDeployment,
		Name: testDepName,
		UID:  types.UID(testDepUid),
	}
	testOwnerRs := metav1.OwnerReference{
		Kind: ownerKindReplicaSet,
		Name: testRsName,
		UID:  types.UID(testRsUid),
	}

	testDep := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDepName,
			Namespace: testNs,
			UID:       types.UID(testDepUid),
		},
		Spec: appv1.DeploymentSpec{
			Selector: testDepSelector,
		},
		Status: appv1.DeploymentStatus{},
	}

	testRs := &appv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRsName,
			Namespace: testNs,
			Labels:    selectorLabels,
			UID:       types.UID(testRsUid),
			OwnerReferences: []metav1.OwnerReference{
				testOwnerDep,
			},
		},
	}

	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
			Labels:    selectorLabels,
			OwnerReferences: []metav1.OwnerReference{
				testOwnerRs,
			},
		},
	}

	type args struct {
		obj          []runtime.Object
		depNamespace string
		depName      string
	}
	tests := []struct {
		name    string
		args    args
		want    []*corev1.Pod
		wantErr bool
	}{
		{
			name: "get pods of deployment",
			args: args{
				obj: []runtime.Object{
					testDep,
					testRs,
					testPod,
				},
				depNamespace: testNs,
				depName:      testDepName,
			},
			want: []*corev1.Pod{
				testPod,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.args.obj...)
			client := clientBuilder.Build()
			got, err := getPodsByDeployment(client, tt.args.depNamespace, tt.args.depName)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPodsByDeployment() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPodsByDeployment() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getDeploymentByPod(t *testing.T) {
	testNs := "test-ns"
	testDepName := "test-dep-name"
	testDepUid := "test-dep-uid"
	testRsName := "test-rs-name"
	testRsUid := "test-rs-uid"
	testPodName := "test-pod-name"

	selectorLabels := map[string]string{
		"deployment-name": testDepName,
	}

	testDepSelector := &metav1.LabelSelector{
		MatchLabels: selectorLabels,
	}

	testOwnerDep := metav1.OwnerReference{
		Kind: ownerKindDeployment,
		Name: testDepName,
		UID:  types.UID(testDepUid),
	}
	testOwnerRs := metav1.OwnerReference{
		Kind: ownerKindReplicaSet,
		Name: testRsName,
		UID:  types.UID(testRsUid),
	}

	testDep := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDepName,
			Namespace: testNs,
			UID:       types.UID(testDepUid),
		},
		Spec: appv1.DeploymentSpec{
			Selector: testDepSelector,
		},
		Status: appv1.DeploymentStatus{},
	}

	testRs := &appv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRsName,
			Namespace: testNs,
			Labels:    selectorLabels,
			UID:       types.UID(testRsUid),
			OwnerReferences: []metav1.OwnerReference{
				testOwnerDep,
			},
		},
	}

	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNs,
			Labels:    selectorLabels,
			OwnerReferences: []metav1.OwnerReference{
				testOwnerRs,
			},
		},
	}
	type args struct {
		obj []runtime.Object
		pod *corev1.Pod
	}
	tests := []struct {
		name    string
		args    args
		want    *appv1.Deployment
		wantErr bool
	}{
		{
			name: "get deployment by pod",
			args: args{
				obj: []runtime.Object{
					testDep,
					testRs,
					testPod,
				},
				pod: testPod,
			},
			want:    testDep,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.args.obj...)
			client := clientBuilder.Build()
			got, err := getDeploymentByPod(client, tt.args.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("getDeploymentByPod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.ObjectMeta, tt.want.ObjectMeta) {
				t.Errorf("getDeploymentByPod() got = %v, want %v", got, tt.want)
			}
		})
	}
}

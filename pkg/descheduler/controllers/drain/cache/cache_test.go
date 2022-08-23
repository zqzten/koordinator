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

package cache

import (
	"fmt"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

func TestNodeInfo_updateStatus(t *testing.T) {
	type args struct {
		n *v1.Node
	}
	tests := []struct {
		name string
		args args
		want *NodeInfo
	}{
		{
			name: "drainable",
			args: args{
				n: &v1.Node{
					Status: v1.NodeStatus{
						Allocatable: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
			want: &NodeInfo{
				Name: "123",
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Free: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("500m"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Score: 50,
				Pods: map[types.UID]*PodInfo{
					"": {
						Request: v1.ResourceList{
							v1.ResourceCPU:     resource.MustParse("500m"),
							v1.ResourceMemory:  resource.MustParse("1Gi"),
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Migratable: true,
					},
				},
				Reservation: nil,
				Drainable:   true,
			},
		}, {
			name: "drainable false reservation > 0",
			args: args{
				n: &v1.Node{
					Status: v1.NodeStatus{
						Allocatable: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("3Gi"),
						},
					},
				},
			},
			want: &NodeInfo{
				Name: "123",
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("3Gi"),
				},
				Free: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("500m"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Score: 58,
				Pods: map[types.UID]*PodInfo{
					"": {
						Request: v1.ResourceList{
							v1.ResourceCPU:     resource.MustParse("500m"),
							v1.ResourceMemory:  resource.MustParse("1Gi"),
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Migratable: true,
					},
				},
				Reservation: map[string]struct{}{"1": {}},
				Drainable:   false,
			},
		}, {
			name: "drainable false",
			args: args{
				n: &v1.Node{
					Status: v1.NodeStatus{
						Allocatable: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse("1"),
							v1.ResourceMemory: resource.MustParse("3Gi"),
						},
					},
				},
			},
			want: &NodeInfo{
				Name: "123",
				Allocatable: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("3Gi"),
				},
				Free: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("500m"),
					v1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Score: 58,
				Pods: map[types.UID]*PodInfo{
					"": {
						Request: v1.ResourceList{
							v1.ResourceCPU:     resource.MustParse("500m"),
							v1.ResourceMemory:  resource.MustParse("1Gi"),
							v1.ResourceStorage: resource.MustParse("1Gi"),
						},
						Migratable: false,
					},
				},
				Reservation: nil,
				Drainable:   false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ni := &NodeInfo{
				Name:        tt.want.Name,
				Pods:        tt.want.Pods,
				Reservation: tt.want.Reservation,
			}
			ni.updateStatus(tt.args.n)
			for i := range ni.Free {
				q := ni.Free[i]
				fmt.Println((&q).String())
				ni.Free[i] = q
			}
			if !reflect.DeepEqual(ni, tt.want) {
				t.Errorf("NodeInfo.updateStatus() = %v, want %v", ni, tt.want)
			}
		})
	}
}

func TestNodeInfo_addPodToCache(t *testing.T) {
	type args struct {
		p *v1.Pod
	}
	tests := []struct {
		name string
		args args
		want *PodInfo
	}{
		{
			name: "ignore fasle, migratable true",
			args: args{
				p: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "testns",
						Name:      "test",
						UID:       "123",
						OwnerReferences: []metav1.OwnerReference{
							{
								Controller: pointer.Bool(true),
							},
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						SchedulerName: "koord-scheduler",
					},
				},
			},
			want: &PodInfo{
				NamespacedName: types.NamespacedName{
					Namespace: "testns",
					Name:      "test",
				},
				UID: "123",
				Request: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Ignore:     false,
				Migratable: true,
			},
		}, {
			name: "ignore fasle, migratable false",
			args: args{
				p: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "testns",
						Name:      "test",
						UID:       "123",
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
					},
				},
			},
			want: &PodInfo{
				NamespacedName: types.NamespacedName{
					Namespace: "testns",
					Name:      "test",
				},
				UID: "123",
				Request: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Ignore:     false,
				Migratable: false,
			},
		}, {
			name: "ignore fasle, migratable false pv",
			args: args{
				p: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "testns",
						Name:      "test",
						UID:       "123",
						OwnerReferences: []metav1.OwnerReference{
							{
								Controller: pointer.Bool(true),
							},
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						Volumes: []v1.Volume{
							{
								VolumeSource: v1.VolumeSource{
									PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
								},
							},
						},
					},
				},
			},
			want: &PodInfo{
				NamespacedName: types.NamespacedName{
					Namespace: "testns",
					Name:      "test",
				},
				UID: "123",
				Request: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Ignore:     false,
				Migratable: false,
			},
		}, {
			name: "ignore true, migratable true",
			args: args{
				p: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "testns",
						Name:      "test",
						UID:       "123",
						OwnerReferences: []metav1.OwnerReference{
							{
								Kind:       "DaemonSet",
								Controller: pointer.Bool(true),
							},
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Resources: v1.ResourceRequirements{
									Requests: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
						SchedulerName: "ahe-scheduler",
					},
				},
			},
			want: &PodInfo{
				NamespacedName: types.NamespacedName{
					Namespace: "testns",
					Name:      "test",
				},
				UID: "123",
				Request: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("1"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Ignore:     true,
				Migratable: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ni := &NodeInfo{
				Pods: map[types.UID]*PodInfo{},
			}
			if got := ni.addPodToCache(tt.args.p); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeInfo.addPodToCache() = %v, want %v", got, tt.want)
			}
		})
	}
}

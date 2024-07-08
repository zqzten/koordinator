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
	"context"
	"testing"

	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func TestCgroupsInfoRecycler_recycleCgroups(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupName := "test-cgroup-name"

	testPodName := "test-pod-name"
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodName,
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}

	testPodNameOfDep := "test-pod-name-of-dep"
	testRsName := "test-rs-name"
	testRsUid := types.UID("test-rs-uid")
	testDepName := "test-dep-name"
	testDepUid := types.UID("test-dep-uid")
	selectorLabels := map[string]string{
		"deployment-name": testDepName,
	}
	testDepSelector := &metav1.LabelSelector{
		MatchLabels: selectorLabels,
	}
	testContainerCgroups := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container-name",
		CPUSet: "2,3",
	}
	testDep := &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testDepName,
			UID:       testDepUid,
		},
		Spec: appv1.DeploymentSpec{
			Selector: testDepSelector,
		},
	}
	testRs := &appv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testRsName,
			Labels:    selectorLabels,
			UID:       testRsUid,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: ownerKindDeployment,
					UID:  testDepUid,
				},
			},
		},
	}
	testPodOfDep := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodNameOfDep,
			Labels:    selectorLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: ownerKindReplicaSet,
					UID:  testRsUid,
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}

	testPodNameOfJob := "test-pod-name-of-job"
	testJobName := "test-job-name"
	testJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testJobName,
		},
	}
	testPodOfJob := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodNameOfJob,
			Labels: map[string]string{
				PodLabelKeyJobName: testJobName,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}

	type fields struct {
		obj           []runtime.Object
		originNodeSLO *slov1alpha1.NodeSLO
	}
	type args struct {
		oldCgroup *resourcesv1alpha1.Cgroups
		newCgroup *resourcesv1alpha1.Cgroups
	}
	type want struct {
		nodeSLO *slov1alpha1.NodeSLO
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "recycle cgroup old(job+pod)-new(job)=job part",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testPodOfDep,
					testPodOfJob,
					testJob,
					testDep,
					testRs,
				},
				originNodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodName,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
											{
												PodNamespace: testNs,
												PodName:      testPodNameOfJob,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				oldCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupName,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
						JobInfo: resourcesv1alpha1.JobInfoSpec{
							Name:      testJobName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
					},
				},
				newCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupName,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						JobInfo: resourcesv1alpha1.JobInfoSpec{
							Name:      testJobName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
					},
				},
			},
			want: want{
				nodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodNameOfJob,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "recycle cgroup old(pod+dep)-new(pod)=dep part",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testPodOfDep,
					testPodOfJob,
					testJob,
					testDep,
					testRs,
				},
				originNodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodName,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
											{
												PodNamespace: testNs,
												PodName:      testPodNameOfDep,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				oldCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupName,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
						DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
							Name:      testDepName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
					},
				},
				newCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupName,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
					},
				},
			},
			want: want{
				nodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodName,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "recycle cgroup old(dep+job)-new(dep)=job part",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testPodOfDep,
					testPodOfJob,
					testJob,
					testDep,
					testRs,
				},
				originNodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodNameOfDep,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
											{
												PodNamespace: testNs,
												PodName:      testPodNameOfJob,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			args: args{
				oldCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupName,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
							Name:      testDepName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
						JobInfo: resourcesv1alpha1.JobInfoSpec{
							Name:      testJobName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
					},
				},
				newCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupName,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						DeploymentInfo: resourcesv1alpha1.DeploymentInfoSpec{
							Name:      testDepName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroups,
							},
						},
					},
				},
			},
			want: want{
				nodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodNameOfDep,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		//stopCh := make(chan struct{}, 1)
		clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.obj...)
		client := clientBuilder.Build()
		t.Run(tt.name, func(t *testing.T) {
			client.Create(context.TODO(), tt.fields.originNodeSLO)
			r := NewCgroupsInfoRecycler(client)
			r.AddCgroups(cgroupsSub(tt.args.oldCgroup, tt.args.newCgroup))
			r.recycleCgroups()
			if equal, msg := getAndCompareNodeSLOCgroup(r.Client, testNodeName, tt.want.nodeSLO); !equal {
				t.Errorf("NodeSLO not equal, name %v, msg: %v", tt.name, msg)
			}
		})
	}
}

func TestCgroupsInfoRecycler_recyclePods(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupName := "test-cgroup-name"
	testPodNameDelete := "test-pod-name-delete"

	testContainerCgroups := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container-name",
		CPUSet: "2,3",
	}
	testCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testCgroupName,
		},
		Spec: resourcesv1alpha1.CgroupsSpec{
			PodInfo: resourcesv1alpha1.PodInfoSpec{
				Name:      testPodNameDelete,
				Namespace: testNs,
				Containers: []resourcesv1alpha1.ContainerInfoSpec{
					testContainerCgroups,
				},
			},
		},
	}

	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodNameDelete,
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testOriginNodeSLO := &slov1alpha1.NodeSLO{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNodeName,
		},
		Spec: slov1alpha1.NodeSLOSpec{
			Extensions: &slov1alpha1.ExtensionsMap{
				Object: map[string]interface{}{
					slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
						{
							CgroupsNamespace: testNs,
							CgroupsName:      testCgroupName,
							PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
								{
									PodNamespace: testNs,
									PodName:      testPodNameDelete,
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroups,
									},
								},
								{
									PodNamespace: testNs,
									PodName:      "another-pod",
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroups,
									},
								},
							},
						},
						{
							CgroupsNamespace: testNs,
							CgroupsName:      "another-cgroups",
							PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
								{
									PodNamespace: testNs,
									PodName:      testPodNameDelete,
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroups,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	type fields struct {
		obj []runtime.Object
	}
	type args struct {
		req controllerruntime.Request
	}
	type want struct {
		nodeSLO *slov1alpha1.NodeSLO
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "recycle deleted pod",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testOriginNodeSLO,
					testCgroups,
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testCgroupName,
					},
				},
			},
			want: want{
				nodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      "another-pod",
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.obj...)
		client := clientBuilder.Build()
		t.Run(tt.name, func(t *testing.T) {
			r := NewCgroupsInfoRecycler(client)
			r.Client.Delete(context.TODO(), testPod)
			r.AddPod(testNs, testPodNameDelete, testNodeName)
			r.recyclePods()
			if equal, msg := getAndCompareNodeSLOCgroup(r.Client, testNodeName, tt.want.nodeSLO); !equal {
				t.Errorf("NodeSLO not equal, msg: %v", msg)
			}
		})
	}
}

func TestCgroupsInfoRecycler_recycleDanglings(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupName := "test-cgroup-name"
	testDanglingCgroupName := "test-dangling-cgroup-name"
	testPodName := "test-pod-name"
	testDanglingPodName := "test-dangling-pod-name"

	testContainerCgroups := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container-name",
		CPUSet: "2,3",
	}
	testCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testCgroupName,
		},
		Spec: resourcesv1alpha1.CgroupsSpec{
			PodInfo: resourcesv1alpha1.PodInfoSpec{
				Name:      testPodName,
				Namespace: testNs,
				Containers: []resourcesv1alpha1.ContainerInfoSpec{
					testContainerCgroups,
				},
			},
		},
	}

	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodName,
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testOriginNodeSLO := &slov1alpha1.NodeSLO{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNodeName,
		},
		Spec: slov1alpha1.NodeSLOSpec{
			Extensions: &slov1alpha1.ExtensionsMap{
				Object: map[string]interface{}{
					slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
						{
							CgroupsNamespace: testNs,
							CgroupsName:      testCgroupName,
							PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
								{
									PodNamespace: testNs,
									PodName:      testPodName,
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroups,
									},
								},
								{
									PodNamespace: testNs,
									PodName:      testDanglingPodName,
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroups,
									},
								},
							},
						},
						{
							CgroupsNamespace: testNs,
							CgroupsName:      testDanglingCgroupName,
							PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
								{
									PodNamespace: testNs,
									PodName:      "another-pod",
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroups,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	type fields struct {
		obj []runtime.Object
	}
	type args struct {
		req controllerruntime.Request
	}
	type want struct {
		nodeSLO *slov1alpha1.NodeSLO
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "recycle danglings",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testOriginNodeSLO,
					testCgroups,
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testCgroupName,
					},
				},
			},
			want: want{
				nodeSLO: &slov1alpha1.NodeSLO{
					ObjectMeta: metav1.ObjectMeta{
						Name: testNodeName,
					},
					Spec: slov1alpha1.NodeSLOSpec{
						Extensions: &slov1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupName,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodName,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroups,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.fields.obj...)
		client := clientBuilder.Build()
		t.Run(tt.name, func(t *testing.T) {
			r := NewCgroupsInfoRecycler(client)
			r.recycleDanglings()
			if equal, msg := getAndCompareNodeSLOCgroup(r.Client, testNodeName, tt.want.nodeSLO); !equal {
				t.Errorf("NodeSLO not equal, msg: %v", msg)
			}
		})
	}
}

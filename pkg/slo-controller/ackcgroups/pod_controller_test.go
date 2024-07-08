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
	"reflect"
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

func TestPodCgroupsReconciler_Reconcile_pod_create(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupName := "test-cgroup-name"
	testPodName := "test-pod-name"
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
	}
	type fields struct {
		obj []runtime.Object
	}
	type args struct {
		req controllerruntime.Request
	}
	type want struct {
		result    ctrl.Result
		resultErr bool
		nodeSLO   *slov1alpha1.NodeSLO
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "cgroups crd with pod info",
			fields: fields{
				obj: []runtime.Object{
					testCgroups,
					testOriginNodeSLO,
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testPodName,
					},
				},
			},
			want: want{
				result:    ctrl.Result{},
				resultErr: false,
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
			r := &PodCgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), testPod)
			got, err := r.Reconcile(context.TODO(), tt.args.req)
			if (err != nil) != tt.want.resultErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.want.resultErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Reconcile() got = %v, want %v", got, tt.want.result)
			}
			if equal, msg := getAndCompareNodeSLOCgroup(r.Client, testNodeName, tt.want.nodeSLO); !equal {
				t.Errorf("NodeSLO not equal, msg: %v", msg)
			}
		})
	}
}

func TestPodCgroupsReconciler_Reconcile_pod_of_dep_create(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupName := "test-cgroup-name"
	testPodName := "test-pod-name"
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
	testCgroups := &resourcesv1alpha1.Cgroups{
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
					Name: testDepName,
					UID:  testDepUid,
				},
			},
		},
	}
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodName,
			Labels:    selectorLabels,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: ownerKindReplicaSet,
					Name: testRsName,
					UID:  testRsUid,
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testOriginNodeSLO := &slov1alpha1.NodeSLO{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNodeName,
		},
	}
	type fields struct {
		obj []runtime.Object
	}
	type args struct {
		req controllerruntime.Request
	}
	type want struct {
		result    ctrl.Result
		resultErr bool
		nodeSLO   *slov1alpha1.NodeSLO
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "cgroups crd with deployment info",
			fields: fields{
				obj: []runtime.Object{
					testCgroups,
					testDep,
					testRs,
					testOriginNodeSLO,
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testPodName,
					},
				},
			},
			want: want{
				result:    ctrl.Result{},
				resultErr: false,
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
			r := &PodCgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), testPod)
			got, err := r.Reconcile(context.TODO(), tt.args.req)
			if (err != nil) != tt.want.resultErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.want.resultErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Reconcile() got = %v, want %v", got, tt.want.result)
			}
			if equal, msg := getAndCompareNodeSLOCgroup(r.Client, testNodeName, tt.want.nodeSLO); !equal {
				t.Errorf("NodeSLO not equal, msg: %v", msg)
			}
		})
	}
}

func TestPodCgroupsReconciler_Reconcile_pod_of_job_create(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupName := "test-cgroup-name"
	testPodName := "test-pod-name"
	testJobName := "test-job-name"

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
			JobInfo: resourcesv1alpha1.JobInfoSpec{
				Name:      testJobName,
				Namespace: testNs,
				Containers: []resourcesv1alpha1.ContainerInfoSpec{
					testContainerCgroups,
				},
			},
		},
	}
	testJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testJobName,
		},
	}
	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testPodName,
			Labels: map[string]string{
				PodLabelKeyJobName: testJobName,
			},
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testOriginNodeSLO := &slov1alpha1.NodeSLO{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNodeName,
		},
	}
	type fields struct {
		obj []runtime.Object
	}
	type args struct {
		req controllerruntime.Request
	}
	type want struct {
		result    ctrl.Result
		resultErr bool
		nodeSLO   *slov1alpha1.NodeSLO
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   want
	}{
		{
			name: "cgroups crd with job info",
			fields: fields{
				obj: []runtime.Object{
					testCgroups,
					testJob,
					testOriginNodeSLO,
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testPodName,
					},
				},
			},
			want: want{
				result:    ctrl.Result{},
				resultErr: false,
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
			r := &PodCgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), testPod)
			got, err := r.Reconcile(context.TODO(), tt.args.req)
			if (err != nil) != tt.want.resultErr {
				t.Errorf("Reconcile() error = %v, wantErr %v", err, tt.want.resultErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want.result) {
				t.Errorf("Reconcile() got = %v, want %v", got, tt.want.result)
			}
			if equal, msg := getAndCompareNodeSLOCgroup(r.Client, testNodeName, tt.want.nodeSLO); !equal {
				t.Errorf("NodeSLO not equal, msg: %v", msg)
			}
		})
	}
}

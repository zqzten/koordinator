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
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

func getAndCompareNodeSLOCgroup(c client.Client, nodeName string, want *slov1alpha1.NodeSLO) (bool, string) {
	nodeSLO := &slov1alpha1.NodeSLO{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: nodeName}, nodeSLO)
	if err != nil {
		return false, fmt.Sprintf("get node slo %v failed, error %v", nodeName, err)
	}
	gotNodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLO.Spec)
	if err != nil {
		return false, fmt.Sprintf("get cgroups from node slo %v failed, error %v", nodeName, err)
	}
	wantNodeCgroups, err := slov1alpha1.GetNodeCgroups(&want.Spec)
	if err != nil {
		return false, fmt.Sprintf("get cgroups from node slo %v failed, error %v", nodeName, err)
	}
	if equal, msg := nodeCgroupsListEqual(gotNodeCgroups, wantNodeCgroups); !equal {
		return false, msg
	}
	return true, ""
}

func TestCgroupsReconciler_Reconcile_cgroup_pod(t *testing.T) {
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
					testPod,
					testOriginNodeSLO,
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
			r := &CgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), testCgroups)
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

func TestCgroupsReconciler_Reconcile_cgroup_deployment(t *testing.T) {
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
					UID:  testRsUid,
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: testNodeName,
		},
	}
	testOriginNodeSLOCreate := &slov1alpha1.NodeSLO{
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
					testPod,
					testRs,
					testDep,
					testOriginNodeSLOCreate,
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
			r := &CgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), testCgroups)
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

func TestCgroupsReconciler_Reconcile_cgroup_job(t *testing.T) {
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
					testPod,
					testJob,
					testOriginNodeSLO,
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
			r := &CgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), testCgroups)
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

func TestCgroupsReconciler_Reconcile_cgroup_delete(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupNameDeleted := "test-cgroup-name-deleted"
	testPodName := "test-pod-name"

	testContainerCgroups := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container-name",
		CPUSet: "2,3",
	}
	testCgroups := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNs,
			Name:      testCgroupNameDeleted,
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
							CgroupsName:      testCgroupNameDeleted,
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
						{
							CgroupsNamespace: testNs,
							CgroupsName:      "another-cgroups",
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
			name: "cgroups crd delete",
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
						Name:      testCgroupNameDeleted,
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
										CgroupsName:      "another-cgroups",
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
			r := &CgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Delete(context.TODO(), testCgroups)
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

func TestCgroupsReconciler_Reconcile_cgroup_pod_already_exist(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	slov1alpha1.AddToScheme(scheme)
	resourcesv1alpha1.AddToScheme(scheme)

	testNs := "test-ns"
	testNodeName := "test-node"
	testCgroupNameOld := "test-cgroup-old-name"
	testCgroupNameNew := "test-cgroup-new-name"
	testPodName := "test-pod-name"
	testContainerCgroupsOld := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container-name",
		CPUSet: "2,3",
	}
	testContainerCgroupsNew := resourcesv1alpha1.ContainerInfoSpec{
		Name:   "test-container-name",
		CPUSet: "5,6",
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
							CgroupsName:      testCgroupNameOld,
							PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
								{
									PodNamespace: testNs,
									PodName:      testPodName,
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroupsOld,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	testOriginNodeSLO2 := &slov1alpha1.NodeSLO{
		ObjectMeta: metav1.ObjectMeta{
			Name: testNodeName,
		},
		Spec: slov1alpha1.NodeSLOSpec{
			Extensions: &slov1alpha1.ExtensionsMap{
				Object: map[string]interface{}{
					slov1alpha1.CgroupExtKey: []*resourcesv1alpha1.NodeCgroupsInfo{
						{
							CgroupsNamespace: testNs,
							CgroupsName:      testCgroupNameOld,
							PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
								{
									PodNamespace: testNs,
									PodName:      testPodName,
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroupsOld,
									},
								},
								{
									PodNamespace: testNs,
									PodName:      "another-pod-name",
									Containers: []*resourcesv1alpha1.ContainerInfoSpec{
										&testContainerCgroupsOld,
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
		obj       []runtime.Object
		newCgroup *resourcesv1alpha1.Cgroups
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
			name: "cgroups crd with pod info already exist",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testOriginNodeSLO,
				},
				newCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupNameNew,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroupsNew,
							},
						},
					},
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testCgroupNameNew,
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
										CgroupsName:      testCgroupNameNew,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodName,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroupsNew,
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
			name: "cgroups crd with pod info already exist and other pod",
			fields: fields{
				obj: []runtime.Object{
					testPod,
					testOriginNodeSLO2,
				},
				newCgroup: &resourcesv1alpha1.Cgroups{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNs,
						Name:      testCgroupNameNew,
					},
					Spec: resourcesv1alpha1.CgroupsSpec{
						PodInfo: resourcesv1alpha1.PodInfoSpec{
							Name:      testPodName,
							Namespace: testNs,
							Containers: []resourcesv1alpha1.ContainerInfoSpec{
								testContainerCgroupsNew,
							},
						},
					},
				},
			},
			args: args{
				req: ctrl.Request{
					NamespacedName: types.NamespacedName{
						Namespace: testNs,
						Name:      testCgroupNameNew,
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
										CgroupsName:      testCgroupNameOld,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      "another-pod-name",
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroupsOld,
												},
											},
										},
									},
									{
										CgroupsNamespace: testNs,
										CgroupsName:      testCgroupNameNew,
										PodCgroups: []*resourcesv1alpha1.PodCgroupsInfo{
											{
												PodNamespace: testNs,
												PodName:      testPodName,
												Containers: []*resourcesv1alpha1.ContainerInfoSpec{
													&testContainerCgroupsNew,
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
			r := &CgroupsReconciler{
				Client: client,
				Log:    ctrl.Log.WithName("controllers").WithName("NodeMetric"),
				Scheme: scheme,
			}
			r.Client.Create(context.TODO(), tt.fields.newCgroup)
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

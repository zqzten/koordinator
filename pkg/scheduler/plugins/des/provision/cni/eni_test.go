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

package cni

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/defaultbinder"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/queuesort"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
	schedulertesting "k8s.io/kubernetes/pkg/scheduler/testing"

	invsdkclient "gitlab.alibaba-inc.com/dbpaas/Inventory/inventory-sdk-go/common/client"
	invsdk "gitlab.alibaba-inc.com/dbpaas/Inventory/inventory-sdk-go/sdk"
)

func newClient(t *testing.T) ReserveNetworkInterfaceProvisioner {
	cs := kubefake.NewSimpleClientset()
	informerFactory := informers.NewSharedInformerFactory(cs, 0)
	registeredPlugins := []schedulertesting.RegisterPluginFunc{
		schedulertesting.RegisterBindPlugin(defaultbinder.Name, defaultbinder.New),
		schedulertesting.RegisterQueueSortPlugin(queuesort.Name, queuesort.New),
	}
	fh, err := schedulertesting.NewFramework(context.TODO(), registeredPlugins, "koord-scheduler",
		frameworkruntime.WithClientSet(cs),
		frameworkruntime.WithInformerFactory(informerFactory),
	)
	assert.Nil(t, err)

	podInformer := fh.SharedInformerFactory().Core().V1().Pods()
	nodeInformer := fh.SharedInformerFactory().Core().V1().Nodes()
	pvcInformer := fh.SharedInformerFactory().Core().V1().PersistentVolumeClaims()
	pvInformer := fh.SharedInformerFactory().Core().V1().PersistentVolumes()
	storageClassInformer := fh.SharedInformerFactory().Storage().V1().StorageClasses()
	kubeClient := fh.ClientSet()
	invClient := invsdk.Client{
		Client: &invsdkclient.Client{},
	}

	vSwitches := map[string]map[string]string{
		"cn-zhangjiakou-a": {
			"vswitch": "test",
			"cidr":    "33.58.0.0/19",
		},
		"cn-zhangjiakou-b": {
			"vswitch": "vsw-8vbvdhush3rsb3lcefo58",
			"cidr":    "33.56.80.0/20",
		},
	}

	vb, _ := json.Marshal(&vSwitches)
	accountMap := &corev1.ConfigMap{
		Data: map[string]string{
			"account":               "test",
			"biztype":               "aliyun",
			"uid":                   "test",
			"aliGroupramRole":       "test",
			"aliGroupsecurityGroup": "test",
			"aligroupVswitches":     string(vb),
		},
	}

	networkInterfaceProvisioner := NewNetworkInterfaceProvisioner(kubeClient, podInformer, nodeInformer, pvcInformer, pvInformer, storageClassInformer, invClient, accountMap)

	return networkInterfaceProvisioner
}
func TestCreateENI(t *testing.T) {
	nodeName := "test-node-1"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:         uuid.NewUUID(),
			Namespace:   "default",
			Name:        "test-pod-1",
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nodeName,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("96"),
				corev1.ResourceMemory: resource.MustParse("512Gi"),
			},
		},
	}

	client := newClient(t)
	_, _ = client.CreateEni(pod, node)
}

func TestDeleteENI(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-pod-1",
		},
	}

	client := newClient(t)
	_ = client.DeleteEni(context.TODO(), pod, "xxxxxxx")
}

func TestPatchENI(t *testing.T) {
	nodeName := "test-node-1"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-pod-1",
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("96"),
				corev1.ResourceMemory: resource.MustParse("512Gi"),
			},
		},
	}

	client := newClient(t)
	enis := make(map[string]invsdk.CreateEniInDesResponse)
	enis["eth0"] = invsdk.CreateEniInDesResponse{}
	enis["eth1"] = invsdk.CreateEniInDesResponse{}
	_ = client.PatchEni(context.TODO(), pod, node, enis)
}

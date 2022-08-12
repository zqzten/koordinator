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

package cpusetallocator

import (
	"context"
	"encoding/json"
	"runtime"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

var (
	defaultNumWorker = runtime.NumCPU()
)

type getAvailableCPUs func(nodeName string) (nodenumaresource.CPUSet, error)

type cpuSharePoolUpdater struct {
	queue                workqueue.RateLimitingInterface
	getAvailableCPUsFunc getAvailableCPUs
	handle               framework.Handle
}

func (u *cpuSharePoolUpdater) asyncUpdate(nodeName string) {
	node, err := u.handle.SharedInformerFactory().Core().V1().Nodes().Lister().Get(nodeName)
	if err != nil || node == nil || extunified.IsVirtualKubeletNode(node) {
		return
	}
	u.queue.Add(nodeName)
}

func (u *cpuSharePoolUpdater) start() {
	for i := 0; i < defaultNumWorker; i++ {
		go u.updateWorker()
	}
}

func (u *cpuSharePoolUpdater) updateWorker() {
	for u.processNextItem() {
	}
}

func (u *cpuSharePoolUpdater) processNextItem() bool {
	item, quit := u.queue.Get()
	if quit {
		return false
	}
	defer u.queue.Done(item)

	err := u.update(item.(string))
	u.handleError(item, err)
	return true
}

func (u *cpuSharePoolUpdater) handleError(item interface{}, err error) {
	if err == nil {
		u.queue.Forget(item)
		return
	}
	u.queue.AddRateLimited(item)
}

func (u *cpuSharePoolUpdater) update(nodeName string) error {
	node, err := u.handle.SharedInformerFactory().Core().V1().Nodes().Lister().Get(nodeName)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	return u.updateNodeCPUSharePool(node)
}

func (u *cpuSharePoolUpdater) updateNodeCPUSharePool(node *corev1.Node) error {
	preCPUSharePool, err := getCPUSharePool(node.Annotations)
	if err != nil {
		return err
	}
	availableCPUs, err := u.getAvailableCPUsFunc(node.Name)
	if err != nil {
		return err
	}
	if availableCPUs.Equals(preCPUSharePool) {
		klog.V(5).Infof("availableCPUs %s equals preCPUSharePool %s", availableCPUs, preCPUSharePool)
		return nil
	}

	oldNode := node
	node = node.DeepCopy()
	if err = setCPUSharePool(node, availableCPUs); err != nil {
		return err
	}

	patchBytes, err := generateNodePatch(oldNode, node)
	if err != nil {
		return err
	}
	if string(patchBytes) == "{}" {
		return nil
	}
	err = retry.OnError(
		retry.DefaultRetry,
		errors.IsTooManyRequests,
		func() error {
			_, err := u.handle.ClientSet().CoreV1().Nodes().
				Patch(context.TODO(), node.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			if err != nil {
				klog.Error("Failed to patch node %s, patch: %v, err: %v", node.Name, string(patchBytes), err)
			}
			return err
		})
	if err != nil {
		klog.Error("Failed to update Node %q CPUSharePool from %v to %v", node.Name, preCPUSharePool, availableCPUs)
		return err
	}
	klog.Infof("Succeed to Update Node %q CPUSharePool from %v to %v", node.Name, preCPUSharePool, availableCPUs)
	return nil
}

func getCPUSharePool(annotations map[string]string) (nodenumaresource.CPUSet, error) {
	cpuSetBuilder := nodenumaresource.NewCPUSetBuilder()
	rawCPUSharePool, ok := annotations[extunified.AnnotationNodeCPUSharePool]
	if !ok {
		return cpuSetBuilder.Result(), nil
	}
	var cpuSharePool extunified.CPUSharePool
	if err := json.Unmarshal([]byte(rawCPUSharePool), &cpuSharePool); err != nil {
		return cpuSetBuilder.Result(), err
	}
	cpuSetBuilder.Add(cpuSharePool.CPUIDs...)
	return cpuSetBuilder.Result(), nil
}

func setCPUSharePool(node *corev1.Node, cpusInCpuSharePool nodenumaresource.CPUSet) error {
	cpuSharePool := extunified.CPUSharePool{
		CPUIDs: cpusInCpuSharePool.ToSlice(),
	}
	cpuSharePoolData, err := json.Marshal(cpuSharePool)
	if err != nil {
		return err
	}
	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}
	node.Annotations[extunified.AnnotationNodeCPUSharePool] = string(cpuSharePoolData)
	return nil

}

func generateNodePatch(oldNode, newNode *corev1.Node) ([]byte, error) {
	oldData, err := json.Marshal(oldNode)
	if err != nil {
		return nil, err
	}

	newData, err := json.Marshal(newNode)
	if err != nil {
		return nil, err
	}
	return strategicpatch.CreateTwoWayMergePatch(oldData, newData, &corev1.Node{})
}

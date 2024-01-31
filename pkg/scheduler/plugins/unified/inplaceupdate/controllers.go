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

package inplaceupdate

import (
	"context"
	"fmt"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	ControllerName = "inplaceUpdate"
)

var (
	_ frameworkext.ControllerProvider = &Plugin{}
	_ frameworkext.Controller         = &controller{}
)

func (p *Plugin) NewControllers() ([]frameworkext.Controller, error) {
	c := &controller{
		queue:    workqueue.NewRateLimitingQueue(workqueue.DefaultItemBasedRateLimiter()),
		addInQFn: p.addInQFn,
		handle:   p.handle,
	}
	return []frameworkext.Controller{c}, nil
}

type controller struct {
	queue    workqueue.RateLimitingInterface
	addInQFn addPodInQueueFn
	handle   framework.Handle
}

func (c *controller) asyncAddToQ(podNamespacedName apimachinerytypes.NamespacedName) {
	klog.Infof("InplaceUpdate: receive an inplace update request for target pod %s/%s", podNamespacedName.Namespace, podNamespacedName.Name)
	c.queue.Add(podNamespacedName)
}

func (c *controller) Name() string {
	return ControllerName
}

func (c *controller) Start() {
	registersPodEventHandler(c.handle, c)
	go c.updateWorker()
}

func (c *controller) updateWorker() {
	for c.processNextItem() {
	}
}

func (c *controller) processNextItem() bool {
	item, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(item)

	err := c.update(item.(apimachinerytypes.NamespacedName))
	c.handleError(item, err)
	return true
}

func (c *controller) handleError(item interface{}, err error) {
	c.queue.Forget(item)
	if err == nil {
		return
	}
	namespacedName := item.(apimachinerytypes.NamespacedName)
	klog.Errorf("InplaceUpdate: handle inplace update request error: %s, pod: %s/%s", err.Error(), namespacedName.Namespace, namespacedName.Name)
}

func (c *controller) update(podNamespacedName apimachinerytypes.NamespacedName) error {
	pod, err := c.handle.SharedInformerFactory().Core().V1().Pods().Lister().Pods(podNamespacedName.Namespace).Get(podNamespacedName.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			klog.Errorf("InplaceUpdate: unexpect error: %s when get pod %s/%s from informer", err.Error(), podNamespacedName.Namespace, podNamespacedName.Name)
		}
		return nil
	}
	podAnnotations := pod.Annotations
	if podAnnotations == nil {
		podAnnotations = map[string]string{}
	}
	if !podHasUnhandledUpdateReq(pod) {
		return nil
	}
	resourceUpdateSpec, err := uniext.GetResourceUpdateSpec(podAnnotations)
	if err != nil {
		// unmarshal error
		c.handle.EventRecorder().Eventf(pod, nil, corev1.EventTypeWarning, "FailedInplaceUpdate", "InplaceUpdate", fmt.Sprintf("Invalid ResourceUpdateSpec: %s", err.Error()))
		return nil
	}
	inplaceUpdatePod := makeInplaceUpdatePod(pod, resourceUpdateSpec)
	err = c.addInQFn(inplaceUpdatePod)
	if err != nil {
		// 解析 key error，不太可能发生, 如果发生，需要将错误信息 patch 到 pod 上，无需重试
		return reject(c.handle.ClientSet().CoreV1().Pods(pod.Namespace), pod, resourceUpdateSpec.Version, err)
	}
	return nil
}

func reject(podClient clientv1.PodInterface, targetPod *corev1.Pod, resourceUpdateVersion string, err error) error {
	modifiedPod := targetPod.DeepCopy()
	if modifiedPod.Annotations == nil {
		modifiedPod.Annotations = map[string]string{}
	}
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	resourceUpdateState := &uniext.ResourceUpdateSchedulerState{
		Version: resourceUpdateVersion,
		Status:  uniext.ResourceUpdateStateRejected,
		Reason:  errStr,
	}
	err = uniext.SetResourceUpdateSchedulerState(modifiedPod.Annotations, resourceUpdateState)
	if err != nil {
		return err
	}
	err = util.RetryOnConflictOrTooManyRequests(func() error {
		// generate patch bytes for the update
		patchBytes, err := util.GeneratePodPatch(targetPod, modifiedPod)
		if err != nil {
			klog.V(5).InfoS("failed to generate pod patch", "pod", klog.KObj(targetPod), "err", err)
			return err
		}
		if string(patchBytes) == "{}" { // nothing to patch
			return nil
		}
		// patch with pod client
		patched, err := podClient.Patch(context.TODO(), targetPod.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			klog.V(5).InfoS("failed to patch pod", "pod", klog.KObj(targetPod), "patch", string(patchBytes), "err", err)
			return err
		}
		klog.V(6).InfoS("successfully patch pod", "pod", klog.KObj(patched), "patch", string(patchBytes))
		return nil
	})
	if err != nil {
		klog.ErrorS(err, "Failed reject pod resource update", "pod", klog.KObj(targetPod), "resourceUpdateVersion", resourceUpdateVersion)
		return err
	}
	klog.V(4).InfoS("Successfully reject pod resource update", "pod", klog.KObj(targetPod), "resourceUpdateVersion", resourceUpdateVersion)
	return err
}

func makeInplaceUpdatePod(pod *corev1.Pod, resourceUpdateSpec *uniext.ResourceUpdateSpec) *corev1.Pod {
	inplaceUpdatePod := pod.DeepCopy()
	inplaceUpdatePod.UID = uuid.NewUUID()
	inplaceUpdatePod.Name = string(pod.UID) + "-" + resourceUpdateSpec.Version
	if inplaceUpdatePod.Annotations == nil {
		inplaceUpdatePod.Annotations = map[string]string{}
	}
	inplaceUpdatePod.Annotations[extunified.AnnotationResourceUpdateTargetPodUID] = string(pod.UID)
	inplaceUpdatePod.Annotations[extunified.AnnotationResourceUpdateTargetPodName] = pod.Name
	// 1. 在 Prefilter 阶段，如果不去掉的话 Volume，VolumeRestriction 对于 ReadWriteOncePod 的 PVC PreFilter 阶段可能会失败
	// 2. 原地升级不会变动 Volume，涉及 Volume 的调度流程就没必要走了
	inplaceUpdatePod.Spec.Volumes = nil
	containersUpdateSpec := map[string]*uniext.ContainerResource{}
	for i := range resourceUpdateSpec.Containers {
		containerUpdateSpec := &resourceUpdateSpec.Containers[i]
		containersUpdateSpec[containerUpdateSpec.Name] = containerUpdateSpec
	}
	for i := range inplaceUpdatePod.Spec.Containers {
		containerToUpdate := &inplaceUpdatePod.Spec.Containers[i]
		containerUpdateSpec, ok := containersUpdateSpec[containerToUpdate.Name]
		if !ok {
			continue
		}
		// 1. 当 InplaceUpdate Complete 之后，InplaceUpdate 插件会从 Cache 中 Forget InplaceUpdatePod，这样其实会把 TargetPod使用的端口 从 Cache 的 NodeInfo.UsedPorts 中错误去掉
		// 2. 原地升级不会变动端口，涉及端口的更改就毫无必要了
		containerToUpdate.Ports = nil
		if !quotav1.Equals(containerToUpdate.Resources.Requests, containerUpdateSpec.Resources.Requests) {
			if containerToUpdate.Resources.Requests == nil {
				containerToUpdate.Resources.Requests = containerUpdateSpec.Resources.Requests.DeepCopy()
			} else {
				for resourceName, quantity := range containerUpdateSpec.Resources.Requests {
					if resourceName == corev1.ResourceCPU {
						klog.V(5).Infof("[Inplace Update] pod: %s/%s, request resource name: %v, value: %v", pod.Namespace, pod.Name, resourceName, quantity.MilliValue())
					} else {
						klog.V(5).Infof("[Inplace Update] pod: %s/%s, request resource name: %v, value: %v", pod.Namespace, pod.Name, resourceName, quantity.Value())
					}
					containerToUpdate.Resources.Requests[resourceName] = quantity
				}
			}
		}
		if !quotav1.Equals(containerToUpdate.Resources.Limits, containerUpdateSpec.Resources.Limits) {
			if containerToUpdate.Resources.Limits == nil {
				containerToUpdate.Resources.Limits = containerUpdateSpec.Resources.Limits.DeepCopy()
			} else {
				for resourceName, quantity := range containerUpdateSpec.Resources.Limits {
					if resourceName == corev1.ResourceCPU {
						klog.V(5).Infof("[Inplace Update] pod: %s/%s, limit resource name: %v, value: %v", pod.Namespace, pod.Name, resourceName, quantity.MilliValue())
					} else {
						klog.V(5).Infof("[Inplace Update] pod: %s/%s, limit resource name: %v, value: %v", pod.Namespace, pod.Name, resourceName, quantity.Value())
					}
					containerToUpdate.Resources.Limits[resourceName] = quantity
				}
			}
		}

	}
	return inplaceUpdatePod
}

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

package hijack

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

// NOTE:
// 比如一个 grpc 请求转换的fake pod或者inplace update的fake pod，走一次调度后，拿到了可以分配的资源和分配结果，例如绑核信息、设备信息等。
// 这些信息都需要写到真实的pod上（这里还包括了调整pod的container resources）。这个在apply patch阶段做（也就是prebind）；
// 真实的Pod上还会带着 fake pod的 uid，在真实Pod update event到达后，再把fake pod已经占住的资源释放掉。
// fake pod 对应到 hijacked pod, 真实 pod 对应到 target pod.

const (
	Name = "Hijack"
)

const (
	AnnotationHijackedPod          = apiext.SchedulingDomainPrefix + "/hijacked-pod"
	AnnotationContainerNameMapping = apiext.SchedulingDomainPrefix + "/container-name-mapping"
)

var (
	_ framework.PreBindPlugin = &Plugin{}
	_ framework.BindPlugin    = &Plugin{}

	_ frameworkext.PreBindExtensions = &Plugin{}
)

// Plugin hijacks the assumed Pod to reserve pod or inplace update pod.
type Plugin struct {
	extendedHandle frameworkext.ExtendedHandle
	lock           sync.Mutex
	hijackedPods   map[types.UID]*corev1.Pod
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	extendedHandle := handle.(frameworkext.ExtendedHandle)
	pl := &Plugin{
		extendedHandle: extendedHandle,
		hijackedPods:   map[types.UID]*corev1.Pod{},
	}

	handle.SharedInformerFactory().Core().V1().Pods().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: pl.onPodUpdate,
		DeleteFunc: pl.onPodDelete,
	})

	return pl, nil
}

func (pl *Plugin) Name() string {
	return Name
}

func (pl *Plugin) onPodUpdate(oldObj, newObj interface{}) {
	pod, _ := newObj.(*corev1.Pod)
	if pod != nil {
		pl.forgetHijackedPodFromSchedulerCache(pod)
	}
}

func (pl *Plugin) onPodDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = obj.(*corev1.Pod)
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*corev1.Pod)
		if !ok {
			return
		}
	default:
		return
	}
	pl.forgetHijackedPodFromSchedulerCache(pod)
}

type stateData struct {
	targetPod *corev1.Pod
}

func (s *stateData) Clone() framework.StateData {
	return s
}

func (pl *Plugin) Unreserve(ctx context.Context, cycleState *framework.CycleState, assumedPod *corev1.Pod, nodeName string) {

}

func (pl *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, assumedPod *corev1.Pod, nodeName string) *framework.Status {
	return nil
}

func (pl *Plugin) ApplyPatch(ctx context.Context, cycleState *framework.CycleState, originalObj, modifiedObj metav1.Object) *framework.Status {
	originalTargetPod := GetTargetPod(cycleState)
	if originalTargetPod == nil {
		return framework.NewStatus(framework.Skip)
	}

	originalHijackedPod, ok := originalObj.(*corev1.Pod)
	if !ok {
		return framework.NewStatus(framework.Skip)
	}
	hijackedPod := modifiedObj.(*corev1.Pod)
	targetPod := originalTargetPod.DeepCopy()
	if err := applyHijackedPodPatch(originalHijackedPod, hijackedPod, targetPod); err != nil {
		klog.ErrorS(err, "Failed to apply hijacked pod patch", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod))
		return framework.AsStatus(err)
	}

	if err := pl.updatePodContainers(targetPod, hijackedPod); err != nil {
		klog.ErrorS(err, "Failed to update containers from hijacked to target", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod))
		return framework.AsStatus(err)
	}

	if err := pl.updateOperatingPod(targetPod, hijackedPod); err != nil {
		klog.ErrorS(err, "Failed to update operating Pod", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod))
		return framework.AsStatus(err)
	}

	patchBytes, err := util.GeneratePodPatch(originalTargetPod, targetPod)
	if err != nil {
		klog.ErrorS(err, "Failed to generate pod patch", "targetPod", klog.KObj(targetPod))
		return framework.AsStatus(err)
	}
	if string(patchBytes) == "{}" {
		return nil
	}

	if err := pl.assumeHijackedPod(targetPod, hijackedPod); err != nil {
		return framework.AsStatus(err)
	}

	updated := false
	err = util.RetryOnConflictOrTooManyRequests(func() error {
		patchedPod, err := util.PatchPod(ctx, pl.extendedHandle.ClientSet(), originalTargetPod, targetPod)
		if err != nil {
			klog.ErrorS(err, "Failed to patch pod", "targetPod", klog.KObj(originalTargetPod))
			return err
		}
		updated = patchedPod != originalTargetPod
		return nil
	})
	if err != nil {
		klog.ErrorS(err, "Failed to apply patches from hijacked pod to target pod", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod))
		pl.forgetHijackedPod(hijackedPod)
		return framework.AsStatus(err)
	}
	if !updated {
		pl.forgetHijackedPod(hijackedPod)
	}
	klog.InfoS("Successfully apply patches from hijacked pod to target pod", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod))
	return nil
}

func applyHijackedPodPatch(originalHijackedPod, hijackedPod, targetPod *corev1.Pod) error {
	delete(hijackedPod.Annotations, apiext.AnnotationReservationAllocated)
	patchBytes, err := util.GeneratePodPatch(originalHijackedPod, hijackedPod)
	if err != nil {
		return err
	}

	if string(patchBytes) != "{}" {
		cloneBytes, _ := json.Marshal(targetPod)
		modified, err := strategicpatch.StrategicMergePatch(cloneBytes, patchBytes, &corev1.Pod{})
		if err != nil {
			return err
		}
		err = json.Unmarshal(modified, targetPod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (pl *Plugin) updatePodContainers(pod, modifiedPod *corev1.Pod) error {
	containerNameMapping, err := getContainerNameMapping(modifiedPod.Annotations)
	if err != nil {
		return fmt.Errorf("failed to parse contaienrNameMapping, err: %w", err)
	}

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		containerName := containerNameMapping[container.Name]
		if containerName == "" {
			containerName = container.Name
		}
		var targetContainer *corev1.Container
		for j := range modifiedPod.Spec.Containers {
			if modifiedPod.Spec.Containers[j].Name == containerName {
				targetContainer = &modifiedPod.Spec.Containers[i]
				break
			}
		}
		if targetContainer == nil {
			continue
		}
		if !equality.Semantic.DeepEqual(container.Resources, targetContainer.Resources) {
			container.Resources = *targetContainer.Resources.DeepCopy()
		}
	}
	return nil
}

func (pl *Plugin) updateOperatingPod(operatingPod, pod *corev1.Pod) error {
	if apiext.IsReservationOperatingMode(operatingPod) {
		if operatingPod.Annotations == nil {
			operatingPod.Annotations = map[string]string{}
		}
		objRef := &corev1.ObjectReference{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       pod.UID,
		}
		err := apiext.SetReservationCurrentOwner(operatingPod.Annotations, objRef)
		if err != nil {
			klog.ErrorS(err, "Failed to SetReservationCurrentOwner for operating Pod", "operatingPod", klog.KObj(operatingPod))
			return err
		}
	}
	return nil
}

func (pl *Plugin) assumeHijackedPod(pod, hijackedPod *corev1.Pod) error {
	objRef := &corev1.ObjectReference{
		UID:       hijackedPod.UID,
		Name:      hijackedPod.Name,
		Namespace: hijackedPod.Namespace,
	}
	data, err := json.Marshal(&objRef)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[AnnotationHijackedPod] = string(data)

	pl.lock.Lock()
	defer pl.lock.Unlock()
	pl.hijackedPods[hijackedPod.UID] = hijackedPod
	return nil
}

func (pl *Plugin) forgetHijackedPod(hijackedPod *corev1.Pod) {
	pl.lock.Lock()
	defer pl.lock.Unlock()
	delete(pl.hijackedPods, hijackedPod.UID)
}

func (pl *Plugin) forgetHijackedPodFromSchedulerCache(targetPod *corev1.Pod) {
	data := targetPod.Annotations[AnnotationHijackedPod]
	if data == "" {
		return
	}
	var objRef corev1.ObjectReference
	if err := json.Unmarshal([]byte(data), &objRef); err != nil {
		return
	}

	pl.lock.Lock()
	defer pl.lock.Unlock()
	hijackedPod := pl.hijackedPods[objRef.UID]
	if hijackedPod != nil {
		err := pl.extendedHandle.ForgetPod(hijackedPod)
		if err != nil {
			klog.ErrorS(err, "failed to forget hijacked pod", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod), "hijacked", data)
		} else {
			delete(pl.hijackedPods, objRef.UID)
			klog.InfoS("Successfully forget hijacked pod", "targetPod", klog.KObj(targetPod), "hijacked", klog.KObj(hijackedPod))
		}
	}
}

func (pl *Plugin) Bind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	targetPod := GetTargetPod(cycleState)
	if targetPod == nil {
		return framework.NewStatus(framework.Skip)
	}
	return framework.NewStatus(framework.Success)
}

func getContainerNameMapping(annotations map[string]string) (map[string]string, error) {
	var mapping map[string]string
	if s := annotations[AnnotationContainerNameMapping]; s != "" {
		mapping = make(map[string]string)
		err := json.Unmarshal([]byte(s), &mapping)
		if err != nil {
			return nil, err
		}
	}
	return mapping, nil
}

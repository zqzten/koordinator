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

package scheduleresult

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

var (
	_ framework.PreBindPlugin = &Plugin{}
)

const (
	// Name is the name of the plugin used in the plugin registry and configurations.
	Name = "UnifiedScheduleResult"
)

type Plugin struct {
	handle framework.Handle
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &Plugin{
		handle: handle,
	}, nil
}

func (p *Plugin) Name() string { return Name }

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	podOriginal := pod
	pod = pod.DeepCopy()
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[extunified.K8sLabelScheduleNodeName] = nodeName
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	updateTime := time.Now().In(time.FixedZone("CST", 8*3600)).Format(time.RFC3339Nano)
	pod.Annotations[extunified.AnnotationSchedulerUpdateTime] = updateTime
	pod.Annotations[extunified.AnnotationSchedulerBindTime] = updateTime
	patchBytes, err := util.GeneratePodPatch(podOriginal, pod)
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}
	if string(patchBytes) == "{}" {
		return nil
	}
	err = retry.OnError(
		retry.DefaultRetry,
		errors.IsTooManyRequests,
		func() error {
			_, err := p.handle.ClientSet().CoreV1().Pods(pod.Namespace).
				Patch(ctx, pod.Name, apimachinerytypes.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})
			if err != nil {
				klog.Error("Failed to patch Pod %s/%s, patch: %v, err: %v", pod.Namespace, pod.Name, string(patchBytes), err)
			}
			return err
		})
	if err != nil {
		return framework.NewStatus(framework.Error, err.Error())
	}

	klog.V(4).Infof("Successfully patch Pod %s/%s, patch: %v", pod.Namespace, pod.Name, string(patchBytes))
	return nil
}

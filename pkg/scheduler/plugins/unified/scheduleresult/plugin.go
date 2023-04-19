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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
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
	registersPodEventHandler(handle)
	return &Plugin{
		handle: handle,
	}, nil
}

func (p *Plugin) Name() string { return Name }

func (p *Plugin) PreBind(ctx context.Context, cycleState *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	return p.preBindObject(ctx, cycleState, pod, nodeName)
}

func (p *Plugin) PreBindReservation(ctx context.Context, cycleState *framework.CycleState, reservation *schedulingv1alpha1.Reservation, nodeName string) *framework.Status {
	return p.preBindObject(ctx, cycleState, reservation, nodeName)
}

func (p *Plugin) preBindObject(ctx context.Context, cycleState *framework.CycleState, obj metav1.Object, nodeName string) *framework.Status {
	labels := map[string]string{extunified.K8sLabelScheduleNodeName: nodeName}
	annotations := map[string]string{}
	updateTime := time.Now().In(time.FixedZone("CST", 8*3600)).Format(time.RFC3339Nano)
	annotations[extunified.AnnotationSchedulerUpdateTime] = updateTime
	annotations[extunified.AnnotationSchedulerBindTime] = updateTime
	// patch pod or reservation with new annotations and new labels
	err := util.RetryOnConflictOrTooManyRequests(func() error {
		_, err1 := util.NewPatch().WithHandle(p.handle).AddAnnotations(annotations).AddLabels(labels).Patch(ctx, obj)
		return err1
	})
	if err != nil {
		klog.V(3).ErrorS(err, "Failed to preBind", "object", klog.KObj(obj))
		return framework.NewStatus(framework.Error, err.Error())
	}
	klog.V(4).Infof("Successfully preBind Object %v(%T) with schedule result", klog.KObj(obj), obj)
	return nil
}

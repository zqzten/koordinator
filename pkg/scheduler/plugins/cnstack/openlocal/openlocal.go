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

package openlocal

import (
	"context"
	"fmt"

	openlocalfk "github.com/alibaba/open-local/pkg/scheduling-framework"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/reservation"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	Name = openlocalfk.PluginName
)

type openlocal struct {
	*openlocalfk.LocalPlugin
}

var _ = framework.PreFilterPlugin(&openlocal{})
var _ = framework.FilterPlugin(&openlocal{})
var _ = framework.ScorePlugin(&openlocal{})
var _ = framework.ReservePlugin(&openlocal{})
var _ = framework.PreBindPlugin(&openlocal{})

func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.V(1).Info("openlocal start")
	kubeConfig := handle.KubeConfig()
	kubeConfig.ContentType = runtime.ContentTypeJSON
	kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
	p, err := openlocalfk.NewLocalPluginWithKubeconfig(obj, handle, kubeConfig)
	if err != nil {
		return nil, err
	}
	local, ok := p.(*openlocalfk.LocalPlugin)
	if !ok {
		return nil, fmt.Errorf("LocalPlugin convert error")
	}
	ol := &openlocal{
		LocalPlugin: local,
	}
	extendedHandle, ok := handle.(frameworkext.ExtendedHandle)
	if !ok {
		return nil, fmt.Errorf("want handle to be of type frameworkext.ExtendedHandle, got %T", handle)
	}
	reservationInformer := extendedHandle.KoordinatorSharedInformerFactory().Scheduling().V1alpha1().Reservations().Informer()
	reservationInformer.AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1alpha1.Reservation:
					return scheduledReservation(t)
				case cache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1alpha1.Reservation); ok {
						return scheduledReservation(pod)
					}
					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    ol.addReservation,
				UpdateFunc: ol.updateReservation,
				DeleteFunc: ol.deleteReservation,
			},
		},
	)
	return ol, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (ol *openlocal) Name() string {
	return Name
}

func (ol *openlocal) addReservation(obj interface{}) {
	r := obj.(*v1alpha1.Reservation)
	if util.IsReservationActive(r) {
		pod := util.NewReservePod(r)
		pod.Status.Phase = corev1.PodRunning
		ol.OnPodAdd(pod)
	}
}

func (ol *openlocal) updateReservation(oldObj, newObj interface{}) {
	oldR := oldObj.(*v1alpha1.Reservation)
	newR := newObj.(*v1alpha1.Reservation)
	if util.IsReservationActive(oldR) && !util.IsReservationActive(newR) {
		pod := util.NewReservePod(newR)
		pod.Status.Phase = corev1.PodRunning
		ol.OnPodDelete(pod)
	}
}

func (ol *openlocal) deleteReservation(obj interface{}) {
	var r *v1alpha1.Reservation
	switch t := obj.(type) {
	case *v1alpha1.Reservation:
		r = t
	case cache.DeletedFinalStateUnknown:
		obj, ok := t.Obj.(*v1alpha1.Reservation)
		if ok {
			r = obj
		}
	}
	if r == nil {
		return
	}
	pod := util.NewReservePod(r)
	ol.OnPodDelete(pod)
}

func (plugin *openlocal) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	if state != nil && reservation.CycleStateMatchReservation(state, nodeInfo.Node().Name) {
		return framework.NewStatus(framework.Success)
	}
	return plugin.LocalPlugin.Filter(ctx, state, pod, nodeInfo)
}

func (plugin *openlocal) Score(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) (int64, *framework.Status) {
	if state != nil && reservation.CycleStateMatchReservation(state, nodeName) {
		return framework.MaxNodeScore, framework.NewStatus(framework.Success)
	}
	return plugin.LocalPlugin.Score(ctx, state, pod, nodeName)
}

func (plugin *openlocal) Reserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) *framework.Status {
	if state != nil && reservation.CycleStateMatchReservation(state, nodeName) {
		reservationPod := pod.DeepCopy()
		reservationPod.UID = reservation.CycleStateMatchReservationUID(state)
		return plugin.LocalPlugin.ReserveReservation(ctx, state, pod, reservationPod, nodeName)
	}
	return plugin.LocalPlugin.Reserve(ctx, state, pod, nodeName)
}

func (plugin *openlocal) Unreserve(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeName string) {
	if state != nil && reservation.CycleStateMatchReservation(state, nodeName) {
		reservationPod := pod.DeepCopy()
		reservationPod.UID = reservation.CycleStateMatchReservationUID(state)
		plugin.LocalPlugin.UnreserveReservation(ctx, state, pod, reservationPod, nodeName)
		return
	}
	plugin.LocalPlugin.Unreserve(ctx, state, pod, nodeName)
}

func scheduledReservation(r *v1alpha1.Reservation) bool {
	return r != nil && len(r.Status.NodeName) > 0
}

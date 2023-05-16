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

package utils

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PatchFunc func(client.Object) client.Object

func Patch(c client.Client, obj client.Object, funcs ...PatchFunc) (bool, error) {
	o := obj.DeepCopyObject()
	newObj, ok := o.(client.Object)
	if !ok {
		return false, fmt.Errorf("convert client.Object error")
	}
	for _, f := range funcs {
		newObj = f(newObj)
	}
	if reflect.DeepEqual(obj, newObj) {
		return false, nil
	}
	if err := c.Patch(context.Background(), newObj, client.MergeFrom(obj)); err != nil {
		return false, err
	}
	return true, nil
}

func PatchStatus(c client.Client, obj client.Object, funcs ...PatchFunc) (bool, error) {
	o := obj.DeepCopyObject()
	newObj, ok := o.(client.Object)
	if !ok {
		return false, fmt.Errorf("convert client.Object error")
	}
	for _, f := range funcs {
		newObj = f(newObj)
	}
	if reflect.DeepEqual(obj, newObj) {
		return false, nil
	}
	if err := c.Status().Patch(context.Background(), newObj, client.MergeFrom(obj)); err != nil {
		return false, err
	}
	return true, nil
}

func ToggleDrainNodeState(c client.Client, eventRecorder events.EventRecorder, dn *v1alpha1.DrainNode, phase v1alpha1.DrainNodePhase, podMigrations []v1alpha1.PodMigration, podMigrationSummary map[v1alpha1.PodMigrationPhase]int32, msg string) error {
	ok, err := PatchStatus(c, dn, func(o client.Object) client.Object {
		newDn := o.(*v1alpha1.DrainNode)
		newDn.Status.Phase = phase
		if len(podMigrations) > 0 {
			sort.SliceStable(podMigrations, func(i, j int) bool {
				if podMigrations[i].Namespace == podMigrations[j].Namespace {
					return podMigrations[i].PodName < podMigrations[j].PodName
				}
				return podMigrations[i].Namespace < podMigrations[j].Namespace
			})
			newDn.Status.PodMigrations = podMigrations
		}
		drainNodeCondition := v1alpha1.DrainNodeCondition{
			Type:    v1alpha1.DrainNodeConditionType(phase),
			Status:  metav1.ConditionTrue,
			Reason:  string(phase),
			Message: msg,
		}
		newDn.Status.PodMigrationSummary = podMigrationSummary
		UpdateCondition(&newDn.Status, &drainNodeCondition)
		return newDn
	})
	if ok {
		klog.Infof("Update DrainNode %v podMigrations %v, msg %v", dn.Name, phase, msg)
		eventRecorder.Eventf(dn, nil, v1.EventTypeNormal, string(phase), string(phase), msg)
		return nil
	}
	return err
}

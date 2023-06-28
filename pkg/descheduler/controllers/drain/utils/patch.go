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

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
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

func ToggleDrainNodeState(c client.Client, eventRecorder events.EventRecorder, dn *v1alpha1.DrainNode, status *v1alpha1.DrainNodeStatus, msg string) error {
	ok, err := PatchStatus(c, dn, func(o client.Object) client.Object {
		newDn := o.(*v1alpha1.DrainNode)
		newDn.Status.Phase = status.Phase
		if len(status.PodMigrations) > 0 {
			sort.SliceStable(status.PodMigrations, func(i, j int) bool {
				if status.PodMigrations[i].Namespace == status.PodMigrations[j].Namespace {
					return status.PodMigrations[i].PodName < status.PodMigrations[j].PodName
				}
				return status.PodMigrations[i].Namespace < status.PodMigrations[j].Namespace
			})
			newDn.Status.PodMigrations = status.PodMigrations
		}
		newDn.Status.Conditions = status.Conditions
		newDn.Status.PodMigrationSummary = status.PodMigrationSummary
		return newDn
	})
	if ok {
		klog.Infof("Update DrainNode %v podMigrations %v, msg %v", dn.Name, status.Phase, msg)
		eventRecorder.Eventf(dn, nil, v1.EventTypeNormal, string(status.Phase), string(status.Phase), msg)
		return nil
	}
	return err
}

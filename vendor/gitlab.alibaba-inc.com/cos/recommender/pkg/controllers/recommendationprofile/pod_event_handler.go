package recommendationprofile

import (
	"reflect"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ predicate.Predicate = &podPredicate{}

type podPredicate struct {
}

func (p podPredicate) Create(event event.CreateEvent) bool {
	_, ok := event.Object.(*v1.Pod)

	return ok
}

func (p podPredicate) Delete(event event.DeleteEvent) bool {
	return false
}

func (p podPredicate) Update(event event.UpdateEvent) bool {
	newPod, ok := event.ObjectNew.(*v1.Pod)
	if !ok {
		return false
	}
	oldPod, ok := event.ObjectOld.(*v1.Pod)
	if !ok {
		return false
	}

	// 如果pod标签发生改变
	if !reflect.DeepEqual(newPod.Labels, oldPod.Labels) {
		return true
	}

	return false
}

func (p podPredicate) Generic(event event.GenericEvent) bool {
	return false
}

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

package resourceflavor

import (
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

var _ handler.EventHandler = &enqueueForEvent{}

type enqueueForEvent struct {
}

func (e *enqueueForEvent) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
}

func (e *enqueueForEvent) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
}

func (e *enqueueForEvent) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
}

func (e *enqueueForEvent) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
}

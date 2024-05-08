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

package expectations

import (
	"strconv"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
)

const (
	AllObjectsKey = ""
)

type ResourceVersionExpectation interface {
	Expect(obj metav1.Object, keys ...string)
	Observe(obj metav1.Object)
	IsSatisfied(obj metav1.Object) (bool, time.Duration)
	IsAllSatisfied(key string) (bool, []string)
	Delete(obj metav1.Object)
}

func NewResourceVersionExpectation() ResourceVersionExpectation {
	return &realResourceVersionExpectation{objectVersions: make(map[types.UID]*objectCacheVersions, 100)}
}

type realResourceVersionExpectation struct {
	sync.Mutex
	objectVersions map[types.UID]*objectCacheVersions
}

type objectCacheVersions struct {
	version                   string
	name                      string
	keys                      sets.String
	firstUnsatisfiedTimestamp time.Time
}

func (r *realResourceVersionExpectation) Expect(obj metav1.Object, keys ...string) {
	r.Lock()
	defer r.Unlock()

	objVersion := r.objectVersions[obj.GetUID()]
	if objVersion == nil {
		objVersion = &objectCacheVersions{}
		r.objectVersions[obj.GetUID()] = objVersion
	}
	if isResourceVersionNewer(objVersion.version, obj.GetResourceVersion()) {
		objVersion.version = obj.GetResourceVersion()
		objVersion.name = obj.GetName()
		objVersion.keys = sets.NewString(keys...)
	}
}

func (r *realResourceVersionExpectation) Observe(obj metav1.Object) {
	r.Lock()
	defer r.Unlock()

	objVersion := r.objectVersions[obj.GetUID()]
	if objVersion == nil {
		return
	}
	if isResourceVersionNewer(objVersion.version, obj.GetResourceVersion()) {
		delete(r.objectVersions, obj.GetUID())
	}
}

func (r *realResourceVersionExpectation) IsSatisfied(obj metav1.Object) (bool, time.Duration) {
	r.Lock()
	defer r.Unlock()

	objVersion := r.objectVersions[obj.GetUID()]
	if objVersion == nil {
		return true, 0
	}

	if isResourceVersionNewer(objVersion.version, obj.GetResourceVersion()) {
		delete(r.objectVersions, obj.GetUID())
		return true, 0
	}

	if objVersion.firstUnsatisfiedTimestamp.IsZero() {
		objVersion.firstUnsatisfiedTimestamp = time.Now()
	}
	return false, time.Since(objVersion.firstUnsatisfiedTimestamp)
}

func (r *realResourceVersionExpectation) IsAllSatisfied(key string) (bool, []string) {
	r.Lock()
	defer r.Unlock()

	if len(r.objectVersions) == 0 {
		return true, nil
	}

	var names, allNames []string
	for _, ocv := range r.objectVersions {
		allNames = append(allNames, ocv.name)
		if key == AllObjectsKey || ocv.keys.Has(key) {
			names = append(names, ocv.name)
		}
	}
	klog.Warningf("Currently unsatisfied objects: %v", allNames)

	if len(names) > 0 {
		return false, names
	}
	return true, nil
}

func (r *realResourceVersionExpectation) Delete(obj metav1.Object) {
	r.Lock()
	defer r.Unlock()
	delete(r.objectVersions, obj.GetUID())
}

func isResourceVersionNewer(old, new string) bool {
	if len(old) == 0 {
		return true
	}

	oldCount, err := strconv.ParseUint(old, 10, 64)
	if err != nil {
		return true
	}

	newCount, err := strconv.ParseUint(new, 10, 64)
	if err != nil {
		return false
	}

	return newCount >= oldCount
}

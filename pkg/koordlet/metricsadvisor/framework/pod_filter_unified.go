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

package framework

import (
	"fmt"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
)

// DefaultRuntimePodFilter allows pods whose runtime classes are runc or not specified.
var DefaultRuntimePodFilter = NewRuntimeClassPodFilter([]string{extunified.PodRuntimeTypeRunc}, []string{extunified.PodRuntimeTypeRund}, true)

// RundPodFilter filters out pods whose runtime classes are not rund.
var RundPodFilter = NewRuntimeClassPodFilter([]string{extunified.PodRuntimeTypeRund}, []string{extunified.PodRuntimeTypeRunc}, false)

// RuntimeClassPodFilter filters the pod according to the pod runtime class.
// 1. The filter rejects the invalid pod meta.
// 2. The filter allows the pod whose runtime class is in the white list.
// 3. The filter rejects the pod whose runtime class is in the black list.
// 4. The filter allows or disallows the pod according to AllowByDefault.
type RuntimeClassPodFilter struct {
	Whitelist      map[string]struct{}
	Blacklist      map[string]struct{}
	AllowByDefault bool
}

func NewRuntimeClassPodFilter(whitelist, blacklist []string, allowByDefault bool) *RuntimeClassPodFilter {
	w := map[string]struct{}{}
	for _, r := range whitelist {
		w[r] = struct{}{}
	}
	b := map[string]struct{}{}
	for _, r := range blacklist {
		b[r] = struct{}{}
	}
	return &RuntimeClassPodFilter{
		Whitelist:      w,
		Blacklist:      b,
		AllowByDefault: allowByDefault,
	}
}

func (f *RuntimeClassPodFilter) FilterPod(podMeta *statesinformer.PodMeta) (bool, string) {
	if podMeta == nil || podMeta.Pod == nil {
		return true, "invalid pod meta"
	}

	runtimeClass := extunified.GetPodRuntimeType(podMeta.Pod)

	if _, inWhitelist := f.Whitelist[runtimeClass]; inWhitelist {
		return false, fmt.Sprintf("pod runtime class [%s] belongs to the whitelist", runtimeClass)
	}

	if _, inBlacklist := f.Blacklist[runtimeClass]; inBlacklist {
		return true, fmt.Sprintf("pod runtime class [%s] belongs to the blacklist", runtimeClass)
	}

	return !f.AllowByDefault, fmt.Sprintf("pod runtime class [%s] unmatched, AllowByDefault=%v",
		runtimeClass, f.AllowByDefault)
}

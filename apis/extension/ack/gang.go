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

package ack

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	LabelGangPrefix = "pod-group.scheduling.sigs.k8s.io"
	LabelGangName   = LabelGangPrefix + "/name"
	LabelGangMinNum = LabelGangPrefix + "/min-available"
)

func init() {
	extension.GetMinNum = GetMinNum
	extension.GetGangName = GetGangName
}

func GetMinNum(pod *v1.Pod) (int, error) {
	if pod.Annotations[extension.AnnotationGangMinNum] != "" {
		minNumber, err := strconv.ParseInt(pod.Annotations[extension.AnnotationGangMinNum], 10, 32)
		if err != nil {
			return 0, err
		}
		return int(minNumber), nil
	}
	if pod.Labels[LabelGangMinNum] != "" {
		minNumber, err := strconv.ParseInt(pod.Labels[LabelGangMinNum], 10, 32)
		if err != nil {
			return 0, err
		}
		return int(minNumber), nil
	}
	return 0, fmt.Errorf("gang min number is not set")
}

func GetGangName(pod *v1.Pod) string {
	gangName := pod.Annotations[extension.AnnotationGangName]
	if gangName == "" {
		gangName = pod.Labels[LabelGangName]
	}
	gangName = strings.ToLower(gangName)
	return gangName
}

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

package unified

import (
	"encoding/json"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

const (
	AnnotationMatchLabelKeysInPodTopologySpread = extension.SchedulingDomainPrefix + "/match-label-keys-in-topology-spread"
)

func GetMatchLabelKeysInPodTopologySpread(annotations map[string]string) ([]string, error) {
	if rawMatchLabelKeys, ok := annotations[AnnotationMatchLabelKeysInPodTopologySpread]; ok {
		var matchLabelKeys []string
		if err := json.Unmarshal([]byte(rawMatchLabelKeys), &matchLabelKeys); err != nil {
			return nil, err
		}
		return matchLabelKeys, nil
	}
	return nil, nil
}

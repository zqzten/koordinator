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
	"fmt"
	"strings"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	LabelAppName      = "app.hippo.io/app-name"
	SigmaLabelAppName = "sigma.ali/app-name"
)

func GetCustomPodAffinity(pod *corev1.Pod) (maxInstancePerHost int, podSpreadInfo *PodSpreadInfo) {
	if pod == nil {
		return
	}
	maxInstancePerHost, podSpreadInfo = getUnifiedCustomAffinity(pod.Annotations)
	if podSpreadInfo != nil {
		return
	}
	return getASICustomAffinity(pod.Labels, pod.Annotations)
}

func getUnifiedCustomAffinity(annotations map[string]string) (maxInstancePerHost int, podSpreadInfo *PodSpreadInfo) {
	if annotations == nil {
		return
	}
	if _, ok := annotations[uniext.AnnotationPodSpreadPolicy]; !ok {
		return
	}
	podSpreadPolicy, err := uniext.GetSchedulerPodSpreadPolicy(annotations)
	if err != nil {
		return
	}
	if podSpreadPolicy == nil {
		return
	}
	if podSpreadPolicy.MaxInstancePerHost <= 0 || podSpreadPolicy.LabelSelector == nil {
		return
	}
	// support specify matchLabels
	appName, appNameExists := podSpreadPolicy.LabelSelector.MatchLabels[uniext.LabelAppName]
	if !appNameExists {
		appName, appNameExists = podSpreadPolicy.LabelSelector.MatchLabels[LabelAppName]
		if !appNameExists {
			appName = podSpreadPolicy.LabelSelector.MatchLabels[SigmaLabelAppName]
		}
	}

	serviceUnit, ok := podSpreadPolicy.LabelSelector.MatchLabels[uniext.LabelDeployUnit]
	if !ok {
		if serviceUnit, ok = podSpreadPolicy.LabelSelector.MatchLabels[uniext.LabelHippoRoleName]; !ok {
			serviceUnit = podSpreadPolicy.LabelSelector.MatchLabels[SigmaLabelServiceUnitName]
		}
	}
	if appName == "" && serviceUnit == "" {
		return
	}
	podSpreadInfo = &PodSpreadInfo{
		AppName:     appName,
		ServiceUnit: serviceUnit,
	}
	maxInstancePerHost = int(podSpreadPolicy.MaxInstancePerHost)
	return
}

type PodSpreadInfo struct {
	AppName     string
	ServiceUnit string
}

func (p PodSpreadInfo) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s/%s", p.AppName, p.ServiceUnit)), nil
}

func (p *PodSpreadInfo) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		return nil
	}
	parts := strings.Split(string(text), "/")
	if len(parts) == 2 {
		p.AppName = parts[0]
		p.ServiceUnit = parts[1]
	} else {
		p.AppName = ""
		p.ServiceUnit = ""
	}
	return nil
}

func getASICustomAffinity(labels, annotations map[string]string) (maxInstancePerHost int, podSpreadInfo *PodSpreadInfo) {
	if labels == nil || annotations == nil {
		return
	}

	appName := getAppName(labels)
	serviceUnit := getServiceUnit(labels)
	if appName == "" && serviceUnit == "" {
		return
	}

	allocSpec, err := GetASIAllocSpec(annotations)
	if err != nil || allocSpec == nil {
		return
	}
	var terms []PodAffinityTerm
	if allocSpec.Affinity != nil && allocSpec.Affinity.PodAntiAffinity != nil {
		terms = allocSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	}
	for _, term := range terms {
		selectorMap, err := metav1.LabelSelectorAsMap(term.PodAffinityTerm.LabelSelector)
		if err != nil {
			continue
		}
		if value, ok := selectorMap[SigmaLabelServiceUnitName]; ok && term.MaxCount > 0 {
			if value == serviceUnit {
				maxInstancePerHost = int(term.MaxCount)
				podSpreadInfo = &PodSpreadInfo{
					ServiceUnit: serviceUnit,
					AppName:     appName,
				}
				break
			}
		} else if value, ok := selectorMap[SigmaLabelAppName]; ok && term.MaxCount > 0 {
			if value == appName {
				maxInstancePerHost = int(term.MaxCount)
				podSpreadInfo = &PodSpreadInfo{
					AppName: appName,
				}
				break
			}
		}
	}
	return
}
func getAppName(label map[string]string) string {
	_, value := getAppNameKeyValue(label)
	return value
}

func getAppNameKeyValue(label map[string]string) (string, string) {
	if value, ok := label[uniext.LabelAppName]; ok {
		return uniext.LabelAppName, value
	} else if value, ok = label[uniext.LabelAppName]; ok {
		return uniext.LabelAppName, value
	} else if value, ok = label[SigmaLabelAppName]; ok {
		return SigmaLabelAppName, value
	}
	return "", ""
}

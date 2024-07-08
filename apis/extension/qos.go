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

package extension

import (
	uniext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

type QoSClass string

// https://koordinator.sh/docs/architecture/qos/
const (
	QoSLSE    QoSClass = "LSE"
	QoSLSR    QoSClass = "LSR"
	QoSLS     QoSClass = "LS"
	QoSBE     QoSClass = "BE"
	QoSSystem QoSClass = "SYSTEM"
	QoSNone   QoSClass = ""
)

func GetPodQoSClassByName(qos string) QoSClass {
	q := QoSClass(qos)

	switch q {
	case QoSLSE, QoSLSR, QoSLS, QoSBE, QoSSystem:
		return q
	}

	return QoSNone
}

// TODO consider using different file for compiling in the future
func getQoSClassAttr(podLabels, podAnnotations map[string]string) (string, bool) {
	if podLabels != nil {
		if qosLabel, exist := podLabels[LabelPodQoS]; exist {
			return qosLabel, true
		}
	}
	if podAnnotations != nil {
		if qosAnnotation, exist := podAnnotations[uniext.AnnotationPodQOSClass]; exist {
			return qosAnnotation, true
		}
	}

	return "", false
}

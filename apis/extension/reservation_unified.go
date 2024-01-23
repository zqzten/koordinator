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
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AnnotationVolumeReservationSpec represents volume-related reservation specification
	AnnotationVolumeReservationSpec = SchedulingDomainPrefix + "/volume-reservation-spec"
)

type VolumeReservationSpec struct {
	VolumeReservations []VolumeReservation `json:"volumeReservations,omitempty"`
}

type VolumeReservation struct {
	StorageClassName string `json:"storageClassName,omitempty"`
	VolumeCount      int    `json:"volumeCount,omitempty"`
}

func SetVolumeReservationSpec(obj metav1.Object, spec *VolumeReservationSpec) error {
	data, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[AnnotationVolumeReservationSpec] = string(data)
	obj.SetAnnotations(annotations)
	return nil
}

func GetVolumeReservationSpec(annotations map[string]string) (*VolumeReservationSpec, error) {
	if s := annotations[AnnotationVolumeReservationSpec]; s != "" {
		var volumeReservationSpec VolumeReservationSpec
		if err := json.Unmarshal([]byte(s), &volumeReservationSpec); err != nil {
			return nil, err
		}
		return &volumeReservationSpec, nil
	}
	return nil, nil
}

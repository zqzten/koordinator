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

package util

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// MultiplyResourceList scales quantity by factor
func MultiplyResourceList(a corev1.ResourceList, factor float64) corev1.ResourceList {
	result := corev1.ResourceList{}
	for key, quantity := range a {
		if key != corev1.ResourceCPU {
			value := quantity.Value()
			newValue := int64(float64(value) * factor)
			newQuant := resource.NewQuantity(newValue, quantity.Format)
			result[key] = *newQuant
		} else {
			value := quantity.MilliValue()
			newValue := int64(float64(value) * factor)
			newQuant := resource.NewMilliQuantity(newValue, quantity.Format)
			result[key] = *newQuant
		}
	}
	return result
}

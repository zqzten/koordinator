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

package validating

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func (h *PodValidatingHandler) gpuResourceValidatingPod(ctx context.Context, req admission.Request) (bool, string, error) {
	pod := &corev1.Pod{}
	if err := h.Decoder.DecodeRaw(req.Object, pod); err != nil {
		return false, "", err
	}
	allErrs := validateGPUResources(pod)
	err := allErrs.ToAggregate()
	allowed := true
	reason := ""
	if err != nil {
		allowed = false
		reason = err.Error()
	}
	return allowed, reason, nil
}

func validateGPUResources(pod *corev1.Pod) field.ErrorList {
	allErrs := field.ErrorList{}
	requests := util.GetPodRequest(pod)
	gpuShared := requests[extension.ResourceGPUShared]
	if pointer.StringDeref(pod.Spec.RuntimeClassName, "") == "rund" && gpuShared.Cmp(resource.MustParse("1")) > 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("pod.spec.containers[*].resources.requests"), gpuShared.String(), "the requested gpu.shared of rund pod should be greater than one"))
	}
	return allErrs
}

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
	"encoding/json"
	"fmt"
	"net/http"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	lrnutil "github.com/koordinator-sh/koordinator/pkg/util/logicalresourcenode"

	unifiedresourceext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	apiscore "k8s.io/kubernetes/pkg/apis/core"
	apiscorev1 "k8s.io/kubernetes/pkg/apis/core/v1"
	corevalidation "k8s.io/kubernetes/pkg/apis/core/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// LogicalResourceNodeValidatingHandler handles Pod
type LogicalResourceNodeValidatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder
}

var _ admission.Handler = &LogicalResourceNodeValidatingHandler{}

// Handle handles admission requests.
func (h *LogicalResourceNodeValidatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {

	obj := &schedulingv1alpha1.LogicalResourceNode{}
	if err := h.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusUnprocessableEntity, err)
	}

	if err := validateLRN(obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if req.Operation == admissionv1.Update {
		oldObj := &schedulingv1alpha1.LogicalResourceNode{}
		if err := h.Decoder.DecodeRaw(req.OldObject, oldObj); err != nil {
			return admission.Errored(http.StatusUnprocessableEntity, err)
		}

		if !apiequality.Semantic.DeepEqual(obj.Spec.Requirements, oldObj.Spec.Requirements) {
			return admission.Errored(http.StatusBadRequest, fmt.Errorf("spec.requirements of LogicalResourceNode can not be changed"))
		}
	}

	return admission.ValidationResponse(true, "")
}

func validateLRN(lrn *schedulingv1alpha1.LogicalResourceNode) error {
	if owners, err := lrnutil.GetReservationOwners(lrn); err != nil {
		return err
	} else if len(owners) == 0 {
		return fmt.Errorf("owners(pod-label-selector) can not be empty")
	}

	if err := validateRequirements(&lrn.Spec.Requirements); err != nil {
		return err
	}

	if str, ok := lrn.Annotations[schedulingv1alpha1.AnnotationVPCQoSThreshold]; ok {
		threshold := schedulingv1alpha1.LRNVPCQoSThreshold{}
		if err := json.Unmarshal([]byte(str), &threshold); err != nil {
			return fmt.Errorf("failed to unmarshal %s: %v", str, err)
		}

		if _, err := resource.ParseQuantity(threshold.Tx); err != nil {
			return fmt.Errorf("invalid qos threshold Tx %s: %v", threshold.Tx, err)
		}
		if _, err := resource.ParseQuantity(threshold.Rx); err != nil {
			return fmt.Errorf("invalid qos threshold Rx %s: %v", threshold.Rx, err)
		}
		if _, err := resource.ParseQuantity(threshold.TxPps); err != nil {
			return fmt.Errorf("invalid qos threshold TxPps %s: %v", threshold.TxPps, err)
		}
		if _, err := resource.ParseQuantity(threshold.RxPps); err != nil {
			return fmt.Errorf("invalid qos threshold RxPps %s: %v", threshold.RxPps, err)
		}
	}

	return nil
}

func validateRequirements(requirements *schedulingv1alpha1.LogicalResourceNodeRequirements) error {
	if err := validateResources(requirements.Resources); err != nil {
		return fmt.Errorf("failed to validate requirements.resources: %s", err)
	}

	mockPodSpec := lrnutil.RequirementsToPodSpec(requirements)

	apisMockPodSpec := &apiscore.PodSpec{}
	if err := apiscorev1.Convert_v1_PodSpec_To_core_PodSpec(mockPodSpec, apisMockPodSpec, nil); err != nil {
		return fmt.Errorf("failed to convert pod spec for validation: %s", err)
	}

	allErrs := corevalidation.ValidatePodSpec(apisMockPodSpec, &metav1.ObjectMeta{}, field.NewPath("spec"), corevalidation.PodValidationOptions{})
	if len(allErrs) > 0 {
		return fmt.Errorf("invalid requirements if it be scheduled as a pod: %s", allErrs.ToAggregate())
	}
	return nil
}

func validateResources(resources corev1.ResourceList) error {
	for name, quantity := range resources {
		switch name {
		case corev1.ResourceCPU, apiext.ResourceNvidiaGPU:
			if err := quantityMustUint(name, quantity); err != nil {
				return err
			}
		case apiext.ResourceGPU, apiext.ResourceGPUCore, apiext.ResourceGPUMemoryRatio,
			unifiedresourceext.GPUResourceAlibaba, unifiedresourceext.GPUResourceCore, unifiedresourceext.GPUResourceMemRatio:
			if err := quantityMustUint100(name, quantity); err != nil {
				return err
			}
		}
	}
	return nil
}

func quantityMustUint(name corev1.ResourceName, quantity resource.Quantity) error {
	val, ok := quantity.AsInt64()
	if !ok {
		return fmt.Errorf("%s %v invalid", name, quantity)
	}
	if val <= 0 {
		return fmt.Errorf("%s %v invalid", name, quantity)
	}
	return nil
}

func quantityMustUint100(name corev1.ResourceName, quantity resource.Quantity) error {
	val, ok := quantity.AsInt64()
	if !ok {
		return fmt.Errorf("%s %v invalid", name, quantity)
	}
	if val <= 0 || val%100 != 0 {
		return fmt.Errorf("%s %v invalid", name, quantity)
	}
	return nil
}

// var _ inject.Client = &LogicalResourceNodeValidatingHandler{}

// InjectClient injects the client into the LogicalResourceNodeValidatingHandler
func (h *LogicalResourceNodeValidatingHandler) InjectClient(c client.Client) error {
	h.Client = c
	return nil
}

// var _ admission.DecoderInjector = &LogicalResourceNodeValidatingHandler{}

// InjectDecoder injects the decoder into the LogicalResourceNodeValidatingHandler
func (h *LogicalResourceNodeValidatingHandler) InjectDecoder(d *admission.Decoder) error {
	h.Decoder = d
	return nil
}

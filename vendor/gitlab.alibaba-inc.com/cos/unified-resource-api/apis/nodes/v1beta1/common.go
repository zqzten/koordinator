package v1beta1

import corev1 "k8s.io/api/core/v1"

type ResourceMap struct {
	corev1.ResourceList `json:"resources,omitempty"`
	// extend disks, gpus in the future
}

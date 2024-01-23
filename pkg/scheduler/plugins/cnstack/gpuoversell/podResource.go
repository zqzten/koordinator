package gpuoversell

import (
	v1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
)

type PodResource struct {
	PodUID             apitypes.UID
	PodName            string
	PodNamespace       string
	AllocatedResources map[int]int
}

func NewPodResource(pod *v1.Pod) *PodResource {
	return &PodResource{
		PodUID:             pod.UID,
		PodName:            pod.Name,
		PodNamespace:       pod.Namespace,
		AllocatedResources: map[int]int{},
	}
}

func (pr *PodResource) Clone() *PodResource {
	clone := &PodResource{
		PodUID:             pr.PodUID,
		PodName:            pr.PodName,
		PodNamespace:       pr.PodNamespace,
		AllocatedResources: make(map[int]int, len(pr.AllocatedResources)),
	}
	for k, v := range pr.AllocatedResources {
		clone.AllocatedResources[k] = v
	}
	return clone
}

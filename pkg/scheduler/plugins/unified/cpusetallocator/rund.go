package cpusetallocator

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

const (
	AnnotationRundNUMAAware = "securecontainer.alibabacloud.com/numa-aware"
	AnnoPassThroughCPUs     = "io.alibaba.pouch.vm.passthru.cpus"
	AnnoSecureContainerCPUs = "securecontainer.alibabacloud.com/cpus"
)

type MBindPolicy string

const (
	MPolBindPolicy      MBindPolicy = "MPOL_BIND"
	MPolPreferredPolicy MBindPolicy = "MPOL_PREFERRED"
)

type RundNUMANodeResult struct {
	MBindPolicy MBindPolicy           `json:"mbind_policy,omitempty"`
	NodeConfig  map[int32]*NodeConfig `json:"node_config"`
}

type NodeConfig struct {
	CPUSet string `json:"cpuset,omitempty"`
	Memory string `json:"memory,omitempty"`
	VCPU   uint64 `json:"vcpu,omitempty"`
}

func SetRundNUMAAwareResult(cpuDetails nodenumaresource.CPUDetails, pod *corev1.Pod, status *extension.ResourceStatus) error {
	if pod.Spec.RuntimeClassName == nil || *pod.Spec.RuntimeClassName != "rund" {
		return nil
	}
	cpus, err := cpuset.Parse(status.CPUSet)
	if err != nil {
		return err
	}
	if !cpus.IsEmpty() {
		cpuDetails = cpuDetails.KeepOnly(cpus)
	}
	var rawNumberOfCPUs string
	var exists bool
	if rawNumberOfCPUs, exists = pod.Annotations[AnnoSecureContainerCPUs]; !exists {
		if rawNumberOfCPUs, exists = pod.Annotations[AnnoPassThroughCPUs]; !exists {
			return fmt.Errorf("impossible, number of cpus not found")
		}
	}
	numberOfCPUs, err := strconv.Atoi(rawNumberOfCPUs)
	if err != nil {
		return err
	}
	roundCPUs(status, int64(numberOfCPUs))
	numaNodeConfig := make(map[int32]*NodeConfig, len(status.NUMANodeResources))
	var sumOfMemoryBytes int64
	for _, numaNodeResource := range status.NUMANodeResources {
		sumOfMemoryBytes += numaNodeResource.Resources.Memory().Value()
	}
	for _, numaNodeResource := range status.NUMANodeResources {
		nodeConfig := &NodeConfig{
			CPUSet: cpuDetails.CPUsInNUMANodes(int(numaNodeResource.Node)).String(),
			VCPU:   uint64(numaNodeResource.Resources.Cpu().Value()),
		}
		if sumOfMemoryBytes != 0 {
			nodeConfig.Memory = strconv.FormatInt(numaNodeResource.Resources.Memory().Value()*100/sumOfMemoryBytes, 10)
		}
		numaNodeConfig[numaNodeResource.Node] = nodeConfig
	}
	result := &RundNUMANodeResult{
		MBindPolicy: MPolPreferredPolicy,
		NodeConfig:  numaNodeConfig,
	}
	rawResult, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pod.Annotations[AnnotationRundNUMAAware] = string(rawResult)
	return nil
}

func roundCPUs(status *extension.ResourceStatus, numberOfCPUs int64) {
	sort.Slice(status.NUMANodeResources, func(i, j int) bool {
		return status.NUMANodeResources[i].Resources.Cpu().MilliValue() < status.NUMANodeResources[j].Resources.Cpu().MilliValue()
	})
	var sumOfMilliCPU int64
	for _, numaNodeResource := range status.NUMANodeResources {
		sumOfMilliCPU += numaNodeResource.Resources.Cpu().MilliValue()
	}
	if sumOfMilliCPU == 0 {
		return
	}
	var alreadyRoundedMilliCPU int64
	var alreadyRoundedCPU int64
	for i, numaNodeResource := range status.NUMANodeResources {
		sumOfMilliCPU -= alreadyRoundedMilliCPU
		numberOfCPUs -= alreadyRoundedCPU
		roundedCPU := int64(math.Ceil(float64(numaNodeResource.Resources.Cpu().MilliValue()) * float64(numberOfCPUs) / float64(sumOfMilliCPU)))
		alreadyRoundedMilliCPU += numaNodeResource.Resources.Cpu().MilliValue()
		alreadyRoundedCPU += roundedCPU
		status.NUMANodeResources[i].Resources[corev1.ResourceCPU] = *resource.NewQuantity(roundedCPU, resource.DecimalSI)
	}
}

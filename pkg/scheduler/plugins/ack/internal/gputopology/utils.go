package gputopology

import (
	"fmt"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
)

const GpuGroup = "topology.kubernetes.io/gpu-group"
const Visible = "topology.kubernetes.io/gpu-visible"

// this function will get all gpu combinations by given gpus
// for example ,if given gpus [1,2,3],we can get gpu combinations
// [1],[2],[3],[1,2],[1,3],[2,3],[1,2,3]
func iterateGPUS(gpus []int, callback func([]int)) {
	var iterate func(gpus, accum []int, size int)
	iterate = func(gpus, accum []int, size int) {
		if len(accum) == size {
			callback(accum)
			return
		}
		for i := range gpus {
			iterate(gpus[i+1:], append(accum, gpus[i]), size)
		}
	}
	for i := 1; i <= len(gpus); i++ {
		iterate(gpus, []int{}, i)
	}
}

// this function is used to get the min bandwidth of given gpu combination
// for example,if given gpus [1,2,3]
// we will calculate the bandwidth of [1,2],[1,3],[2,3]
// and pick the min bandwidth
func getGPULinkMinBandwidth(gpus []int, callback func([]int)) {
	var iterate func(gpus, accum []int, size int)
	iterate = func(gpus, accum []int, size int) {
		if len(accum) == size {
			callback(accum)
			return
		}
		for i := range gpus {
			iterate(gpus[i+1:], append(accum, gpus[i]), size)
		}
	}
	iterate(gpus, []int{}, 2)
}

// check matrix is a square matrix
func checkBandwidthMatrixIsValid(matrix [][]float32) error {
	if matrix == nil {
		return fmt.Errorf("matrix is null")
	}
	rowLength := len(matrix)
	for _, row := range matrix {
		if len(row) != rowLength {
			return fmt.Errorf("matrix is not a square matrix")
		}
	}
	return nil
}

func getPodAnnotation(pod *v1.Pod, key string) (string, bool) {
	if pod.ObjectMeta.Annotations == nil {
		return "", false
	}
	value, ok := pod.ObjectMeta.Annotations[key]
	return value, ok
}

func GetResourceIDList(pod *v1.Pod) []int {
	var result = []int{}
	gpuVisible, exit := getPodAnnotation(pod, Visible)
	if !exit {
		return result
	}
	ids := strings.Split(gpuVisible, ",")
	for _, i := range ids {
		j, err := strconv.Atoi(i)
		if err != nil {
			panic(err)
		}
		result = append(result, j)
	}
	return result
}

func GetGPUsAndMinBandwidth(availableGPUs []string, hints []GPUTopologyHint, requestGPUs int) (TopologyGPUs, float32, error) {
	allocation := []string{}
	minBandwidth := float32(9999999999)
	// if not available gpus,return nil
	if len(availableGPUs) < requestGPUs {
		return allocation, minBandwidth, ErrNotAvailableGPUAllocation
	}
	if len(hints) == 0 {
		return availableGPUs[0:requestGPUs], minBandwidth, nil
	}
	// convert gpu index string to int
	gpus := []int{}
	for _, gpuIndex := range availableGPUs {
		value, err := strconv.Atoi(gpuIndex)
		if err != nil {
			return allocation, minBandwidth, fmt.Errorf("failed to parse gpu index string (%v) to int: %v", gpuIndex, err)
		}
		gpus = append(gpus, value)
	}
	availableAffinity, err := bitmask.NewBitMask(gpus...)
	if err != nil {
		return allocation, minBandwidth, err
	}
	var result *GPUTopologyHint
	for _, topo := range hints {
		// check the length of gpu combination is equal to request gpus number
		if topo.Affinity.Count() != requestGPUs {
			continue
		}
		// check the gpu combination is subset of available gpus
		mergedAffinity := bitmask.And(availableAffinity, topo.Affinity)
		if mergedAffinity.Count() != requestGPUs {
			continue
		}
		// found the topology hint whose value of min bandwidth is max
		if result == nil || result.MinBandwidth < topo.MinBandwidth {
			result = &GPUTopologyHint{Affinity: mergedAffinity, MinBandwidth: topo.MinBandwidth}
		}
	}
	if result == nil {
		return allocation, minBandwidth, ErrNotAvailableGPUAllocation
	}
	for _, bit := range result.Affinity.GetBits() {
		allocation = append(allocation, fmt.Sprintf("%v", bit))
	}
	return allocation, result.MinBandwidth, nil
}

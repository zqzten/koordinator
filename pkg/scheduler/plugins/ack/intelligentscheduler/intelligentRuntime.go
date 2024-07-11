package intelligentscheduler

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"reflect"
	"strconv"
)

const (
	IntelligentSchedulerNodeLabel = "alipay/intelligent-schedule"
)

// score的计算可以放在这里
type IntelligentSchedulerRuntime struct {
	name            string
	nodeScorePolicy string
	gpuScorePolicy  string
}

func NewIntelligentSchedulerRuntime(name string, nodeScorePolicy string, gpuScorePolicy string) *IntelligentSchedulerRuntime {
	return &IntelligentSchedulerRuntime{
		name:            name,
		nodeScorePolicy: nodeScorePolicy,
		gpuScorePolicy:  gpuScorePolicy,
	}
}
func (r *IntelligentSchedulerRuntime) Name() string {
	return r.name
}

func (r *IntelligentSchedulerRuntime) getNodeScorePolicy() string {
	return r.nodeScorePolicy
}

func (r *IntelligentSchedulerRuntime) getGPUScorePolicy() string {
	return r.gpuScorePolicy
}

func (r *IntelligentSchedulerRuntime) Init() error {
	//TODO
	return nil
}

func validateGPUSchedulerArgs(args IntelligentSchedulerArgs) error {
	if reflect.DeepEqual(args, IntelligentSchedulerArgs{}) {
		return fmt.Errorf("intelligent scheduler plugin requires at least one argument")
	}
	gpuMemoryScoreWeight := *args.GPUMemoryScoreWeight
	gpuUtilizationScoreWeight := *args.GPUUtilizationScoreWeight
	if !(gpuMemoryScoreWeight >= 0 && gpuMemoryScoreWeight <= 100 && gpuUtilizationScoreWeight >= 0 && gpuUtilizationScoreWeight <= 100 && gpuMemoryScoreWeight+gpuUtilizationScoreWeight == 100) {
		return fmt.Errorf("invalid GPU score weight")
	}
	if !(args.GpuSelectorPolicy == "spread" || args.GpuSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid GPU selector policy. It should be 'spread' or 'binpack'")
	}
	if !(args.NodeSelectorPolicy == "spread" || args.NodeSelectorPolicy == "binpack") {
		return fmt.Errorf("invalid node selector policy. It should be 'spread' or 'binpack'")
	}
	return nil
}

func isIntelligentNode(node *v1.Node) bool {
	// TODO 这个label需要在设置node gpu调度方式时设置
	if node.Labels[IntelligentSchedulerNodeLabel] == "true" {
		return true
	}
	return false
}

func isIntelligentPod(pod *v1.Pod) bool {
	keyFound := false
	for _, container := range pod.Spec.Containers {
		for resourceName, _ := range container.Resources.Limits {
			if resourceName == VirtualGpuSpecificationKey {
				keyFound = true
				break
			}
		}
	}
	if keyFound {
		klog.Infof("Pod with name [%v] needs to be handled cause annotation key %v is found", pod.Name, VirtualGpuSpecificationKey)
	} else {
		klog.Infof("Pod with name [%v] will be ignored cause annotation key %v is not found", pod.Name, VirtualGpuSpecificationKey)
	}
	return keyFound
}

func GetVirtualGPUCountAndSpec(pod *v1.Pod, cache *intelligentCache) (int, string, error) {
	totalGPUCount := 0
	vGpuSpecName := ""
	for _, container := range pod.Spec.Containers {
		for resourceName, r := range container.Resources.Limits {
			if resourceName == VirtualGpuSpecificationKey {
				if vGpuSpecName == "" {
					vGpuSpecName = r.String()
				} else {
					if vGpuSpecName != r.String() {
						return 0, "", fmt.Errorf("multiple virtual GPU specification found for one pod [%v]", pod.Name)
					}
				}
				vGpuSpec, ok := cache.virtualGpuSpecifications[vGpuSpecName]
				if !ok {
					return 0, "", fmt.Errorf("virtual gpu specification not found for pod %v", pod.Name)
				}
				if !vGpuSpec.isActive {
					return 0, "", fmt.Errorf("virtual gpu specification %s is not active", vGpuSpecName)
				}
			}
			if resourceName == VirtualGpuCountKey {
				count, err := strconv.Atoi(r.String())
				if err != nil {
					return 0, "", fmt.Errorf("unable to parse %v %v into integer", VirtualGpuCountKey, r.String())
				}
				totalGPUCount += count
			}
		}
	}
	klog.Infof("Pod with name [%v] should be allocated with [%v] virtual gpu", pod.Name, totalGPUCount)
	return totalGPUCount, vGpuSpecName, nil
}

func nodeAvailableForPod(nodeResources *NodeInfo, vGpuPodState *VirtualGpuPodState) bool {
	panic("implement me")
}

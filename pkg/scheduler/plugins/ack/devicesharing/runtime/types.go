package runtime

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/noderesource"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/policy"
)

// PolicyInstance is an instance of policy with node label
type PolicyInstance struct {
	policy.Policy
	Config    PolicyConfig
	IsDefault bool
}

// PolicyConfig describes Policy configuration
type PolicyConfig struct {
	// PolicyName holds policy name
	PolicyName string `json:"-"`
	// AlgorithmName holds algorithm name
	AlgorithmName string `json:"algorithm,omitempty"`
	// NodeSelectors represents that which nodes should use this policy,these nodes has the same label
	NodeSelectors map[string]string `json:"nodeSelectors,omitempty"`
}

type SearchNodePolicy func(pod *v1.Pod, node *v1.Node, policyInstances []PolicyInstance) (PolicyInstance, error)

type FitlerNodeByPolicy func(state *framework.CycleState, pod *v1.Pod, node *v1.Node, policyInstance PolicyInstance) (bool, error)

type AllocateDeviceResources func(state *framework.CycleState, pod *v1.Pod, node *v1.Node, policyInstances []PolicyInstance) (map[v1.ResourceName]noderesource.PodResource, int, error)

type BuildPodAnnotations func(*v1.Pod, map[v1.ResourceName]noderesource.PodResource, string) (map[string]string, error)

type AddOrUpdateNode func(node *v1.Node)

type AddOrUpdatePod func(pod *v1.Pod)

type FilterNodeArgs struct {
	Node                   *v1.Node
	SchedulePod            *v1.Pod
	GetNodeResourceManager func(state *framework.CycleState, nodeName string) (*noderesource.NodeResourceManager, error)
	SearchNodePolicy       func(pod *v1.Pod, node *v1.Node, policyInstances []PolicyInstance) (PolicyInstance, error)
	FitlerNodeByPolicy     func(state *framework.CycleState, pod *v1.Pod, node *v1.Node, policyInstance PolicyInstance, nrm *noderesource.NodeResourceManager) (bool, error)
}

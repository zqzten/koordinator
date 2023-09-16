package nodeaffinity

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	temporaryNodeAffinityStateKey = "koordinator.sh/temporary-node-affinity"
)

type TemporaryNodeAffinity struct {
	NodeSelector *corev1.NodeSelector
}

func (t *TemporaryNodeAffinity) Clone() framework.StateData {
	return t
}

func SetTemporaryNodeAffinity(cycleState *framework.CycleState, affinity *TemporaryNodeAffinity) {
	cycleState.Write(temporaryNodeAffinityStateKey, affinity)
}

func GetTemporaryNodeAffinity(cycleState *framework.CycleState) RequiredNodeSelectorAndAffinity {
	s, _ := cycleState.Read(temporaryNodeAffinityStateKey)
	pod := &corev1.Pod{}
	if temporaryNodeAffinity, _ := s.(*TemporaryNodeAffinity); temporaryNodeAffinity != nil {
		pod.Spec.Affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: temporaryNodeAffinity.NodeSelector,
			},
		}
	}
	return GetRequiredNodeAffinity(pod)
}

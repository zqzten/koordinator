package cpusetallocator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/nodenumaresource"
)

func TestSetRundNUMAAwareResult(t *testing.T) {
	tests := []struct {
		name                      string
		pod                       *corev1.Pod
		status                    *extension.ResourceStatus
		expectRundNUMAAwareResult *RundNUMANodeResult
		wantErr                   assert.ErrorAssertionFunc
	}{
		{
			name: "runc pod",
			pod: &corev1.Pod{Spec: corev1.PodSpec{
				RuntimeClassName: pointer.String("runc"),
			}},
			status: &extension.ResourceStatus{
				CPUSet: "0-1",
				NUMANodeResources: []extension.NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2"),
						},
					},
				},
			},
			expectRundNUMAAwareResult: &RundNUMANodeResult{},
			wantErr:                   assert.NoError,
		},
		{
			name: "rund pod, cpuset parse error",
			pod: &corev1.Pod{Spec: corev1.PodSpec{
				RuntimeClassName: pointer.String("rund"),
			}},
			status: &extension.ResourceStatus{
				CPUSet: "0-",
				NUMANodeResources: []extension.NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2"),
						},
					},
				},
			},
			expectRundNUMAAwareResult: &RundNUMANodeResult{},
			wantErr:                   assert.Error,
		},
		{
			name: "rund pod, cpu nums not found",
			pod: &corev1.Pod{Spec: corev1.PodSpec{
				RuntimeClassName: pointer.String("rund"),
			}},
			status: &extension.ResourceStatus{
				CPUSet: "0-1",
				NUMANodeResources: []extension.NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2"),
						},
					},
				},
			},
			expectRundNUMAAwareResult: &RundNUMANodeResult{},
			wantErr:                   assert.Error,
		},
		{
			name: "rund pod, cpuset",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnoSecureContainerCPUs: "2",
					},
				},
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
				},
			},
			status: &extension.ResourceStatus{
				CPUSet: "0-1",
				NUMANodeResources: []extension.NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2"),
						},
					},
				},
			},
			expectRundNUMAAwareResult: &RundNUMANodeResult{
				MBindPolicy: MPolPreferredPolicy,
				NodeConfig: map[int32]*NodeConfig{
					0: {
						CPUSet: "0-1",
						Memory: "",
						VCPU:   2,
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "rund pod, cpushare, numa 0 3.5, numa 1 5.5",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnoSecureContainerCPUs: "9",
					},
				},
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
				},
			},
			status: &extension.ResourceStatus{
				NUMANodeResources: []extension.NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("3500m"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("5500m"),
						},
					},
				},
			},
			expectRundNUMAAwareResult: &RundNUMANodeResult{
				MBindPolicy: MPolPreferredPolicy,
				NodeConfig: map[int32]*NodeConfig{
					0: {
						CPUSet: "0-95",
						VCPU:   4,
					},
					1: {
						CPUSet: "96-191",
						VCPU:   5,
					},
				},
			},
			wantErr: assert.NoError,
		},
		{
			name: "rund pod, cpushare, numa 0 5.5, numa 1 3.5",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnoSecureContainerCPUs: "9",
					},
				},
				Spec: corev1.PodSpec{
					RuntimeClassName: pointer.String("rund"),
				},
			},
			status: &extension.ResourceStatus{
				NUMANodeResources: []extension.NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("5500m"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("3500m"),
						},
					},
				},
			},
			expectRundNUMAAwareResult: &RundNUMANodeResult{
				MBindPolicy: MPolPreferredPolicy,
				NodeConfig: map[int32]*NodeConfig{
					0: {
						CPUSet: "0-95",
						VCPU:   5,
					},
					1: {
						CPUSet: "96-191",
						VCPU:   4,
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuTopology := buildCPUTopologyForTest(1, 2, 48, 2)
			tt.wantErr(t, SetRundNUMAAwareResult(cpuTopology.CPUDetails, tt.pod, tt.status))
			numaAwareResult := &RundNUMANodeResult{}
			rawNUMAAwareResult, ok := tt.pod.Annotations[AnnotationRundNUMAAware]
			if ok {
				err := json.Unmarshal([]byte(rawNUMAAwareResult), &numaAwareResult)
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectRundNUMAAwareResult, numaAwareResult)
		})
	}
}

func buildCPUTopologyForTest(numSockets, nodesPerSocket, coresPerNode, cpusPerCore int) *nodenumaresource.CPUTopology {
	topo := &nodenumaresource.CPUTopology{
		NumSockets: numSockets,
		NumNodes:   nodesPerSocket * numSockets,
		NumCores:   coresPerNode * nodesPerSocket * numSockets,
		NumCPUs:    cpusPerCore * coresPerNode * nodesPerSocket * numSockets,
		CPUDetails: make(map[int]nodenumaresource.CPUInfo),
	}
	var nodeID, coreID, cpuID int
	for s := 0; s < numSockets; s++ {
		for n := 0; n < nodesPerSocket; n++ {
			for c := 0; c < coresPerNode; c++ {
				for p := 0; p < cpusPerCore; p++ {
					topo.CPUDetails[cpuID] = nodenumaresource.CPUInfo{
						SocketID: s,
						NodeID:   nodeID,
						CoreID:   coreID,
						CPUID:    cpuID,
					}
					cpuID++
				}
				coreID++
			}
			nodeID++
		}
	}
	return topo
}

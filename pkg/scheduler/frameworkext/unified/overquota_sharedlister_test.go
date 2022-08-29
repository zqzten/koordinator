package unified

import (
	"fmt"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

var _ framework.SharedLister = &testSharedLister{}

type testSharedLister struct {
	nodeInfos   []*framework.NodeInfo
	nodeInfoMap map[string]*framework.NodeInfo
}

func newTestSharedLister(nodeNum, podNumPerNode int) *testSharedLister {
	nodeInfoMap := make(map[string]*framework.NodeInfo)
	for i := 0; i < nodeNum; i++ {
		var pods []*corev1.Pod
		for j := 1; j <= podNumPerNode; j++ {
			pods = append(
				pods,
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      fmt.Sprintf("test-pod-%d-%d", i, j),
						Labels: map[string]string{
							extunified.AnnotationDisableOverQuotaFilter: "true",
						},
						Annotations: map[string]string{
							extunified.AnnotationDisableOverQuotaFilter: "true",
						},
					},
					Spec: corev1.PodSpec{
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      extunified.LabelEnableOverQuota,
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"false"},
												},
												{
													Key:      "dummy-label",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"test"},
												},
											},
										},
									},
								},
							},
							PodAntiAffinity: &corev1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												extunified.AnnotationDisableOverQuotaFilter: "true",
											},
										},
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:              resource.MustParse("1"),
										corev1.ResourceMemory:           resource.MustParse("1Gi"),
										corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
										apiext.KoordBatchCPU:            resource.MustParse("1"),
										apiext.KoordBatchMemory:         resource.MustParse("1Gi"),
									},
								},
								Ports: []corev1.ContainerPort{
									{
										HostPort:      int32(i) + 8000,
										ContainerPort: int32(i) + 8000,
									},
									{
										HostPort:      int32(i) + 9000,
										ContainerPort: int32(i) + 9000,
									},
								},
							},
						},
					},
				},
			)
		}
		nodeInfo := framework.NewNodeInfo(pods...)
		nodeName := "test-node" + strconv.Itoa(i)
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					extunified.LabelEnableOverQuota: "true",
				},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("1000"),
					corev1.ResourceMemory:           resource.MustParse("1000Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("1000Gi"),
					corev1.ResourcePods:             resource.MustParse("110"),
					apiext.KoordBatchCPU:            resource.MustParse("1"),
					apiext.KoordBatchMemory:         resource.MustParse("1Gi"),
				},
			},
		}
		nodeInfo.SetNode(node)
		nodeInfoMap[nodeName] = nodeInfo
	}

	nodeInfos := make([]*framework.NodeInfo, 0)
	for _, v := range nodeInfoMap {
		nodeInfos = append(nodeInfos, v)
	}

	return &testSharedLister{
		nodeInfos:   nodeInfos,
		nodeInfoMap: nodeInfoMap,
	}
}

func (f *testSharedLister) NodeInfos() framework.NodeInfoLister {
	return f
}

func (f *testSharedLister) List() ([]*framework.NodeInfo, error) {
	return f.nodeInfos, nil
}

func (f *testSharedLister) HavePodsWithAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) HavePodsWithRequiredAntiAffinityList() ([]*framework.NodeInfo, error) {
	return nil, nil
}

func (f *testSharedLister) Get(nodeName string) (*framework.NodeInfo, error) {
	return f.nodeInfoMap[nodeName], nil
}

func BenchmarkHookNodeInfoWithOverQuota(b *testing.B) {
	sharedLister := NewOverQuotaSharedLister(newTestSharedLister(10000, 60))
	b.Run("clone", func(b *testing.B) {
		_, err := sharedLister.NodeInfos().List()
		if err != nil {
			b.Fatal(err)
		}
	})
}

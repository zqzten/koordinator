package resourcesummary

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

func Test_statisticsPodUsedResource(t *testing.T) {
	priorities := []int32{
		uniext.PriorityProdValueMax, uniext.PriorityBatchValueMax, uniext.PriorityBatchValueMax,
	}
	podStatistics := []v1beta1.PodStatistics{
		{
			Name: "test",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}
	var pods []corev1.Pod
	for i := 0; i < 3; i++ {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-0",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		pod.Name = fmt.Sprintf("pod-%d-%d", 0, 3)
		pod.Spec.Priority = &priorities[i]
		pods = append(pods, pod)
	}
	type args struct {
		candidateNodes *corev1.NodeList
		nodeOwnedPods  map[string]*corev1.PodList
		podStatistics  []v1beta1.PodStatistics
	}
	tests := []struct {
		name    string
		args    args
		want    []map[uniext.PriorityClass]corev1.ResourceList
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "normal",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10000Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string]*corev1.PodList{
					"node-0": {
						Items: pods,
					},
				},
				podStatistics: podStatistics,
			},
			want: []map[uniext.PriorityClass]corev1.ResourceList{
				{
					uniext.PriorityProd: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					uniext.PriorityBatch: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := statisticsPodUsedResource(tt.args.candidateNodes, tt.args.nodeOwnedPods, tt.args.podStatistics)
			tt.wantErr(t, err)
			for priorityClassType, resourceList := range tt.want[0] {
				assert.True(t, quotav1.Equals(resourceList, got[0][priorityClassType]))
			}
		})
	}
}

func Test_statisticsNodeResource(t *testing.T) {
	priorities := []int32{
		uniext.PriorityProdValueMax, uniext.PriorityBatchValueMax, uniext.PriorityBatchValueMax,
	}
	var pods []corev1.Pod
	for i := 0; i < 3; i++ {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test",
				Labels: map[string]string{
					"app": "test",
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-0",
				Containers: []corev1.Container{
					{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		}
		pod.Name = fmt.Sprintf("pod-%d-%d", 0, 3)
		pod.Spec.Priority = &priorities[i]
		pods = append(pods, pod)
	}
	type args struct {
		candidateNodes *corev1.NodeList
		nodeOwnedPods  map[string]*corev1.PodList
	}
	tests := []struct {
		name          string
		args          args
		wantCapacity  map[uniext.PriorityClass]corev1.ResourceList
		wantRequested map[uniext.PriorityClass]corev1.ResourceList
		wantFree      map[uniext.PriorityClass]corev1.ResourceList
	}{
		{
			name: "normal",
			args: args{
				candidateNodes: &corev1.NodeList{
					Items: []corev1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-0",
							},
							Spec: corev1.NodeSpec{
								Taints: []corev1.Taint{
									{
										Key:    "sigma.ali/resource-pool",
										Value:  "sigma_public",
										Effect: corev1.TaintEffectNoSchedule,
									},
								},
							},
							Status: corev1.NodeStatus{
								Allocatable: corev1.ResourceList{
									corev1.ResourceCPU:              resource.MustParse("110"),
									corev1.ResourceMemory:           resource.MustParse("100Gi"),
									corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
									extension.BatchCPU:              resource.MustParse("50000"),
									extension.BatchMemory:           resource.MustParse("10000Gi"),
								},
								Conditions: []corev1.NodeCondition{{
									Type:   corev1.NodeReady,
									Status: corev1.ConditionTrue,
								}},
							},
						},
					},
				},
				nodeOwnedPods: map[string]*corev1.PodList{
					"node-0": {
						Items: pods,
					},
				},
			},
			wantCapacity: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:              resource.MustParse("110"),
					corev1.ResourceMemory:           resource.MustParse("100Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:              resource.MustParse("50"),
					corev1.ResourceMemory:           resource.MustParse("10Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
			},
			wantRequested: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				uniext.PriorityMid:  {},
				uniext.PriorityFree: {},
			},
			wantFree: map[uniext.PriorityClass]corev1.ResourceList{
				uniext.PriorityProd: {
					corev1.ResourceCPU:              resource.MustParse("109"),
					corev1.ResourceMemory:           resource.MustParse("88Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				uniext.PriorityBatch: {
					corev1.ResourceCPU:              resource.MustParse("48"),
					corev1.ResourceMemory:           resource.MustParse("2Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				uniext.PriorityMid: {
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				uniext.PriorityFree: {
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCapacity, gotRequested, gotFree := statisticsNodeResource(tt.args.candidateNodes, tt.args.nodeOwnedPods)
			for _, priorityClassType := range priorityClassTypes {
				assert.True(t, quotav1.Equals(tt.wantCapacity[priorityClassType], gotCapacity[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantRequested[priorityClassType], gotRequested[priorityClassType]))
				assert.True(t, quotav1.Equals(tt.wantFree[priorityClassType], gotFree[priorityClassType]))
			}
		})
	}
}

func TestResourceSummaryReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)
	err = v1beta1.AddToScheme(scheme)
	assert.NoError(t, err)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{
		Client: client,
	}

	//testEnv := envtest.Environment{}
	//cfg, err := testEnv.Start()
	//assert.NoError(t, err)
	//
	//_, err = ctrclient.New(cfg, ctrclient.Options{Scheme: scheme})
	//assert.NoError(t, err)

	nodeName := fmt.Sprintf("node-%d", 0)
	err = r.Client.Create(context.Background(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{
					Key:    "sigma.ali/resource-pool",
					Value:  "sigma_public",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:              resource.MustParse("110"),
				corev1.ResourceMemory:           resource.MustParse("100Gi"),
				corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				extension.BatchCPU:              resource.MustParse("50000"),
				extension.BatchMemory:           resource.MustParse("10000Gi"),
			},
			Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	assert.NoError(t, err)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod.Name = fmt.Sprintf("pod-%d-%d", 0, 1)
	priority := uniext.PriorityProdValueMax
	pod.Spec.Priority = &priority
	err = r.Client.Create(context.Background(), pod)
	assert.NoError(t, err)

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod.Name = fmt.Sprintf("pod-%d-%d", 0, 2)
	priority = uniext.PriorityBatchValueMax
	pod.Spec.Priority = &priority
	err = r.Client.Create(context.Background(), pod)
	assert.NoError(t, err)

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	pod.Name = fmt.Sprintf("pod-%d-%d", 0, 3)
	priority = uniext.PriorityBatchValueMax
	pod.Spec.Priority = &priority
	err = r.Client.Create(context.Background(), pod)
	assert.NoError(t, err)

	resourceSummary := &v1beta1.ResourceSummary{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-summary",
			Namespace: "test",
		},
		Spec: v1beta1.ResourceSummarySpec{
			Tolerations: []corev1.Toleration{
				{
					Key:      "sigma.ali/resource-pool",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			PodStatistics: []v1beta1.PodStatistics{
				{
					Name: "test",
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			ResourceSpecs: []v1beta1.ResourceSpec{
				{
					Name: "50C50Gi",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50"),
						corev1.ResourceMemory: resource.MustParse("50Gi"),
					},
				},
				{
					Name: "500C500Gi",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500"),
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
				{
					Name: "10C1Gi",
					Resources: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		},
	}
	key := types.NamespacedName{
		Namespace: resourceSummary.Namespace,
		Name:      resourceSummary.Name,
	}
	err = r.Create(context.Background(), resourceSummary)
	assert.NoError(t, err)

	_, err = r.Reconcile(context.TODO(), ctrl.Request{NamespacedName: key})
	assert.NoError(t, err)

	newSummary := &v1beta1.ResourceSummary{}
	err = r.Client.Get(context.Background(), key, newSummary)
	assert.NoError(t, err)
	sort.Slice(newSummary.Status.Resources, func(i, j int) bool {
		return newSummary.Status.Resources[i].PriorityClass < newSummary.Status.Resources[j].PriorityClass
	})
	sort.Slice(newSummary.Status.ResourceSpecStats, func(i, j int) bool {
		return newSummary.Status.ResourceSpecStats[i].Name < newSummary.Status.ResourceSpecStats[j].Name
	})
	for _, resourceSpecStat := range newSummary.Status.ResourceSpecStats {
		sort.Slice(resourceSpecStat.Allocatable, func(i, j int) bool {
			return resourceSpecStat.Allocatable[i].PriorityClass < resourceSpecStat.Allocatable[j].PriorityClass
		})
	}
	newPodUsedStatistics := newSummary.Status.PodUsedStatistics[0]
	sort.Slice(newPodUsedStatistics.Allocated, func(i, j int) bool {
		return newPodUsedStatistics.Allocated[i].PriorityClass < newPodUsedStatistics.Allocated[j].PriorityClass
	})

	expectedSummary := newSummary.DeepCopy()
	expectedSummary.Status = v1beta1.ResourceSummaryStatus{
		UpdateTimestamp: newSummary.Status.UpdateTimestamp,
		Phase:           v1beta1.ResourceSummarySucceeded,
		NumNodes:        1,
		Resources: []*v1beta1.NodeResourceSummary{
			{
				PriorityClass: uniext.PriorityProd,
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("110"),
					corev1.ResourceMemory:           resource.MustParse("100Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("109"),
					corev1.ResourceMemory:           resource.MustParse("88Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
			{
				PriorityClass: uniext.PriorityBatch,
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("50"),
					corev1.ResourceMemory:           resource.MustParse("10Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("48"),
					corev1.ResourceMemory:           resource.MustParse("2Gi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
			{
				PriorityClass: uniext.PriorityMid,
				Capacity: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{},
				Allocatable: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
			},
			{
				PriorityClass: uniext.PriorityFree,
				Capacity: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
				Allocated: corev1.ResourceList{},
				Allocatable: corev1.ResourceList{
					corev1.ResourceEphemeralStorage: resource.MustParse("200Gi"),
				},
			},
		},
		ResourceSpecStats: []*v1beta1.ResourceSpecStat{
			{
				Name: "50C50Gi",
				Allocatable: []*v1beta1.ResourceSpecStateAllocatable{
					{
						PriorityClass: uniext.PriorityProd,
						Count:         int32(1),
					},
					{
						PriorityClass: uniext.PriorityBatch,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityMid,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityFree,
						Count:         int32(0),
					},
				},
			},
			{
				Name: "500C500Gi",
				Allocatable: []*v1beta1.ResourceSpecStateAllocatable{
					{
						PriorityClass: uniext.PriorityProd,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityBatch,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityMid,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityFree,
						Count:         int32(0),
					},
				},
			},
			{
				Name: "10C1Gi",
				Allocatable: []*v1beta1.ResourceSpecStateAllocatable{
					{
						PriorityClass: uniext.PriorityProd,
						Count:         int32(10),
					},
					{
						PriorityClass: uniext.PriorityBatch,
						Count:         int32(2),
					},
					{
						PriorityClass: uniext.PriorityMid,
						Count:         int32(0),
					},
					{
						PriorityClass: uniext.PriorityFree,
						Count:         int32(0),
					},
				},
			},
		},
		PodUsedStatistics: []*v1beta1.PodUsedStatistics{
			{
				Name: "test",
				Allocated: []*v1beta1.PodPriorityUsed{
					{
						PriorityClass: uniext.PriorityFree,
						Allocated:     corev1.ResourceList{},
					},
					{
						PriorityClass: uniext.PriorityMid,
						Allocated:     corev1.ResourceList{},
					},
					{
						PriorityClass: uniext.PriorityProd,
						Allocated: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					{
						PriorityClass: uniext.PriorityBatch,
						Allocated: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("8Gi"),
						},
					},
				},
			},
		},
	}
	sort.Slice(expectedSummary.Status.Resources, func(i, j int) bool {
		return expectedSummary.Status.Resources[i].PriorityClass < expectedSummary.Status.Resources[j].PriorityClass
	})
	sort.Slice(expectedSummary.Status.ResourceSpecStats, func(i, j int) bool {
		return expectedSummary.Status.ResourceSpecStats[i].Name < expectedSummary.Status.ResourceSpecStats[j].Name
	})
	for _, resourceSpecStat := range expectedSummary.Status.ResourceSpecStats {
		sort.Slice(resourceSpecStat.Allocatable, func(i, j int) bool {
			return resourceSpecStat.Allocatable[i].PriorityClass < resourceSpecStat.Allocatable[j].PriorityClass
		})
	}
	expectedPodUsedStatistics := expectedSummary.Status.PodUsedStatistics[0]
	sort.Slice(expectedPodUsedStatistics.Allocated, func(i, j int) bool {
		return expectedPodUsedStatistics.Allocated[i].PriorityClass < expectedPodUsedStatistics.Allocated[j].PriorityClass
	})

	assert.Equal(t, expectedSummary.Status.Phase, newSummary.Status.Phase)
	assert.Equal(t, expectedSummary.Status.NumNodes, newSummary.Status.NumNodes)
	assert.Len(t, newSummary.Status.Resources, len(expectedSummary.Status.Resources))
	for i := 0; i < len(expectedSummary.Status.Resources); i++ {
		left := expectedSummary.Status.Resources[i]
		right := newSummary.Status.Resources[i]
		for k, v := range left.Capacity {
			assert.True(t, v.Equal(right.Capacity[k]))
		}
		for k, v := range left.Allocatable {
			if !v.Equal(right.Allocatable[k]) {
				klog.Info(k, " ", v, right.Allocatable[k], "\n")
			}
			assert.True(t, v.Equal(right.Allocatable[k]))
		}
		for k, v := range left.Allocated {
			assert.True(t, v.Equal(right.Allocated[k]))
		}
	}

	for i := 0; i < len(expectedSummary.Status.ResourceSpecStats); i++ {
		left := expectedSummary.Status.ResourceSpecStats[i]
		right := newSummary.Status.ResourceSpecStats[i]
		assert.Equal(t, left.Name, right.Name)
		for k, v := range left.Allocatable {
			assert.Equal(t, v.PriorityClass, right.Allocatable[k].PriorityClass)
			assert.Equal(t, v.Count, right.Allocatable[k].Count)
		}
	}

	assert.Equal(t, expectedPodUsedStatistics.Name, newPodUsedStatistics.Name)
	for i := 0; i < len(expectedPodUsedStatistics.Allocated); i++ {
		left := expectedPodUsedStatistics.Allocated[i]
		right := newPodUsedStatistics.Allocated[i]
		for k, v := range left.Allocated {
			assert.True(t, v.Equal(right.Allocated[k]))
		}
	}
}

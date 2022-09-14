package orchestratingslo

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	unifiedsev1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	koordsev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	deschedulerconfig "github.com/koordinator-sh/koordinator/pkg/descheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/apis/config/v1alpha2"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/evictor"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/unified"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/framework"
)

func init() {
	_ = unifiedsev1.AddToScheme(scheme.Scheme)
	_ = koordsev1alpha1.AddToScheme(scheme.Scheme)
}

type fakeEvictor struct {
	client.Client
	framework.Handle
}

func (f fakeEvictor) Evictor() framework.Evictor {
	return f
}

func (f fakeEvictor) Name() string { return "fakeEvictor" }

func (f fakeEvictor) Filter(pod *v1.Pod) bool { return true }

func (f fakeEvictor) Evict(ctx context.Context, pod *v1.Pod, evictionOptions framework.EvictOptions) bool {
	var v1beta2args v1alpha2.MigrationControllerArgs
	v1alpha2.SetDefaults_MigrationControllerArgs(&v1beta2args)
	var args deschedulerconfig.MigrationControllerArgs
	err := v1alpha2.Convert_v1alpha2_MigrationControllerArgs_To_config_MigrationControllerArgs(&v1beta2args, &args, nil)
	if err != nil {
		panic(err)
	}

	return migration.CreatePodMigrationJob(ctx, pod, evictionOptions, f.Client, &args) == nil
}

func makeUnhealthyPods(t *testing.T, namespace, name string, context *ProblemContext, labels map[string]string, annotations map[string]string) *v1.Pod {
	var data []byte
	if context != nil {
		var err error
		data, err = json.Marshal(context)
		if err != nil {
			t.Fatal(err)
		}
	}

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				LabelOrchestratingSLOHealthy: "false",
			},
			Annotations: map[string]string{
				AnnotationOrchestratingSLOProblems: string(data),
			},
		},
	}
	for k, v := range labels {
		pod.Labels[k] = v
	}
	for k, v := range annotations {
		pod.Annotations[k] = v
	}
	return pod
}

func makeHealthyPod(namespace, name string, labels map[string]string, annotations map[string]string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   namespace,
			Name:        name,
			Labels:      labels,
			Annotations: annotations,
		},
	}
}

func Test_parseUnhealthySloes(t *testing.T) {
	tests := []struct {
		name               string
		orchestratingSLO   *unifiedsev1.OrchestratingSLO
		wantUnhealthySloes []unifiedsev1.OrchestratingSLODefinitionStatus
		wantExpectSloes    map[unifiedsev1.OrchestratingSLOType]float64
		wantErr            bool
	}{
		{
			name: "all sloes healthy",
			orchestratingSLO: &unifiedsev1.OrchestratingSLO{
				Spec: unifiedsev1.OrchestratingSLOSpec{
					Sloes: []unifiedsev1.OrchestratingSLODefinition{
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
							Value: "99.99%",
						},
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresMutex,
							Value: "99.99%",
						},
					},
				},
				Status: unifiedsev1.OrchestratingSLOStatus{
					Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
							Value: "99.99%",
						},
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresMutex,
							Value: "99.99%",
						},
					},
				},
			},
			wantUnhealthySloes: nil,
			wantExpectSloes: map[unifiedsev1.OrchestratingSLOType]float64{
				unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA: 99.99,
				unifiedsev1.OrchestratingSLOGuaranteeCoresMutex:   99.99,
			},
		},
		{
			name: "missing sloes status",
			orchestratingSLO: &unifiedsev1.OrchestratingSLO{
				Spec: unifiedsev1.OrchestratingSLOSpec{
					Sloes: []unifiedsev1.OrchestratingSLODefinition{
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
							Value: "99.99%",
						},
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresMutex,
							Value: "99.99%",
						},
					},
				},
				Status: unifiedsev1.OrchestratingSLOStatus{},
			},
			wantUnhealthySloes: nil,
			wantExpectSloes:    nil,
		},
		{
			name: "has sloes unhealthy",
			orchestratingSLO: &unifiedsev1.OrchestratingSLO{
				Spec: unifiedsev1.OrchestratingSLOSpec{
					Sloes: []unifiedsev1.OrchestratingSLODefinition{
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
							Value: "99.99%",
						},
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresMutex,
							Value: "99.99%",
						},
					},
				},
				Status: unifiedsev1.OrchestratingSLOStatus{
					Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
						{
							Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
							Value:             "99%",
							UnhealthyReplicas: 10,
						},
						{
							Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresMutex,
							Value: "99.99%",
						},
					},
				},
			},
			wantUnhealthySloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "99%",
					UnhealthyReplicas: 10,
				},
			},
			wantExpectSloes: map[unifiedsev1.OrchestratingSLOType]float64{
				unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA: 99.99,
				unifiedsev1.OrchestratingSLOGuaranteeCoresMutex:   99.99,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := parseUnhealthySloes(tt.orchestratingSLO)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUnhealthySloes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.wantUnhealthySloes) {
				t.Errorf("parseUnhealthySloes() got = %v, wantUnhealthySloes %v", got, tt.wantUnhealthySloes)
			}
			if !reflect.DeepEqual(got1, tt.wantExpectSloes) {
				t.Errorf("parseUnhealthySloes() got1 = %v, wantUnhealthySloes %v", got1, tt.wantExpectSloes)
			}
		})
	}
}

func Test_listUnhealthyPods(t *testing.T) {
	assert := assert.New(t)
	client := fake.NewClientBuilder().Build()
	unhealthyPod1 := makeUnhealthyPods(t, "default", "test-1", &ProblemContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Problems: []Problem{
			{
				SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
				Reason:  "just for test",
			},
		},
	}, map[string]string{"labelA": "1"}, nil)
	unhealthyPod1.Status.Phase = v1.PodRunning
	unhealthyPod1.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}
	unhealthyPod2 := makeUnhealthyPods(t, "default", "test-2", &ProblemContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Problems: []Problem{
			{
				SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
				Reason:  "just for test",
			},
		},
	}, map[string]string{"labelA": "1"}, nil)
	unhealthyPod2.Status.Phase = v1.PodRunning
	unhealthyPod2.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}
	healthyPod1 := makeHealthyPod("default", "test-3", map[string]string{"labelA": "1"}, nil)
	healthyPod1.Status.Phase = v1.PodRunning
	healthyPod1.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}
	err := client.Create(context.TODO(), unhealthyPod1)
	assert.NoError(err)
	err = client.Create(context.TODO(), unhealthyPod2)
	assert.NoError(err)
	err = client.Create(context.TODO(), healthyPod1)
	assert.NoError(err)

	r := &Reconciler{
		Client:        client,
		eventRecorder: nil,
		podCache:      NewPodAssumeCache(nil),
	}

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"labelA": "1",
				},
			},
		},
	}
	unhealthySloes := []unifiedsev1.OrchestratingSLODefinitionStatus{
		{
			Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
			Value:             "0%",
			UnhealthyReplicas: 2,
		},
	}
	unhealthyPods, err := r.listUnhealthyPods(orchestratingSLO, unhealthySloes)
	assert.NoError(err)
	assert.NotEmpty(unhealthyPods)
	for _, pods := range unhealthyPods {
		sort.Slice(pods, func(i, j int) bool {
			return pods[i].Name < pods[j].Name
		})
	}
	expectedPods := map[unifiedsev1.OrchestratingSLOType][]*v1.Pod{
		unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA: {unhealthyPod1, unhealthyPod2},
	}
	assert.Equal(expectedPods, unhealthyPods)
}

func Test_groupUnhealthyPodsByOperation(t *testing.T) {
	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type: unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.RebindCPUOrchestratingSLOOperation,
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
				{
					Type: unifiedsev1.OrchestratingSLOGuaranteeCoresMutex,
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.RebindCPUOrchestratingSLOOperation,
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
			},
		},
	}

	unhealthyPod1 := makeUnhealthyPods(t, "default", "test-1", nil, map[string]string{"labelA": "1"}, nil)
	unhealthyPod2 := makeUnhealthyPods(t, "default", "test-2", nil, map[string]string{"labelA": "1"}, nil)

	pods := map[unifiedsev1.OrchestratingSLOType][]*v1.Pod{
		unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA: {unhealthyPod1, unhealthyPod2},
		unifiedsev1.OrchestratingSLOGuaranteeCoresMutex:   {unhealthyPod1},
	}
	groupResult := groupUnhealthyPodsByOperation(orchestratingSLO, pods)
	for _, v := range groupResult {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Name < v[j].Name
		})
	}
	expectedGroupResult := map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.RebindCPUOrchestratingSLOOperation: {unhealthyPod1, unhealthyPod2},
		unifiedsev1.MigrateOrchestratingSLOOperation:   {unhealthyPod1, unhealthyPod2},
	}
	assert.Equal(t, expectedGroupResult, groupResult)
}

func Test_removePodsInNextOperationGroups(t *testing.T) {
	unhealthyPods := []*v1.Pod{
		makeUnhealthyPods(t, "default", "test-1", nil, nil, nil),
		makeUnhealthyPods(t, "default", "test-2", nil, nil, nil),
		makeUnhealthyPods(t, "default", "test-3", nil, nil, nil),
		makeUnhealthyPods(t, "default", "test-4", nil, nil, nil),
	}
	clones := make([]*v1.Pod, len(unhealthyPods))
	copy(clones, unhealthyPods)
	groups := map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.RebindCPUOrchestratingSLOOperation: unhealthyPods,
	}
	removePodsInNextOperationGroups([]unifiedsev1.OrchestratingSLOOperationType{unifiedsev1.RebindCPUOrchestratingSLOOperation}, groups, []*v1.Pod{unhealthyPods[1]})
	for _, v := range groups {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Name < v[j].Name
		})
	}
	expectedGroups := map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.RebindCPUOrchestratingSLOOperation: {clones[0], clones[2], clones[3]},
	}
	assert.Equal(t, expectedGroups, groups)
}

func TestRebindCPUFlow(t *testing.T) {
	assert := assert.New(t)

	constantUID := uuid.NewUUID()

	oldUUIDGenerateFn := migration.UUIDGenerateFn
	migration.UUIDGenerateFn = func() types.UID {
		return constantUID
	}
	defer func() {
		migration.UUIDGenerateFn = oldUUIDGenerateFn
	}()

	fakeClient := fake.NewClientBuilder().Build()
	unhealthyPod := makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Problems: []Problem{
			{
				SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
				Reason:  "just for test",
			},
		},
	}, map[string]string{"labelA": "1"}, nil)
	unhealthyPod.Spec.NodeName = "test-node-1"
	unhealthyPod.Status.Phase = v1.PodRunning
	unhealthyPod.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}

	err := fakeClient.Create(context.TODO(), unhealthyPod)
	assert.NoError(err)

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				RebindCPU: &unifiedsev1.OrchestratingSLORebindCPUParams{
					DryRun: false,
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.RebindCPUOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          1,
			HealthyReplicas:   0,
			UnhealthyReplicas: 1,
			AvailableReplicas: 1,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 1,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)
	assumeCache.add(unhealthyPod)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.RebindCPUOrchestratingSLOOperation: {unhealthyPod},
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	operationStatus, err := r.getPodOperationStatus(unhealthyPod)
	assert.NoError(err)
	assert.True(operationStatus == SLOOperationStatusPending)

	pod := &v1.Pod{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-pod-1"}, pod)
	assert.NoError(err)
	opsCtx, err := GetOperationContext(pod, unifiedsev1.RebindCPUOrchestratingSLOOperation)
	assert.NoError(err)
	assert.NotNil(opsCtx)
	assert.NotNil(opsCtx.LastUpdateTime)
	opsCtx.LastUpdateTime = nil
	expectOpsCtx := &OperationContext{
		LastUpdateTime: nil,
		Operation:      string(unifiedsev1.RebindCPUOrchestratingSLOOperation),
		Count:          1,
	}
	assert.Equal(expectOpsCtx, opsCtx)

	rebindResources := &unifiedsev1.RebindResourceList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"sigma.ali/node-sn": "test-node-1",
		}),
	}
	err = r.Client.List(context.TODO(), rebindResources, listOpts)
	assert.NoError(err)
	assert.Len(rebindResources.Items, 1)

	rebindResourceCtx := &extension.RebindResourceContext{
		ID:      string(constantUID),
		Reason:  "Violating the SLO of CPU Orchestration",
		Trigger: "descheduler",
	}
	rebindResourceCtxBytes, err := json.Marshal(rebindResourceCtx)
	assert.NoError(err)

	deadlineSeconds := int64(180)
	expectRebindResource := &unifiedsev1.RebindResource{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "1",
			Name:            string(constantUID),
			Annotations: map[string]string{
				extension.AnnotationRebindResourceContext: string(rebindResourceCtxBytes),
			},
			Labels: map[string]string{
				"sigma.ali/node-sn": "test-node-1",
			},
		},
		Spec: unifiedsev1.RebindResourceSpec{
			NodeName:              "test-node-1",
			RebindAction:          unifiedsev1.RebindCPU,
			ActiveDeadlineSeconds: &deadlineSeconds,
		},
	}
	assert.Equal(expectRebindResource, &rebindResources.Items[0])
}

func TestRebindCPUFlowWithDryRun(t *testing.T) {
	assert := assert.New(t)

	constantUID := uuid.NewUUID()

	oldUUIDGenerateFn := migration.UUIDGenerateFn
	migration.UUIDGenerateFn = func() types.UID {
		return constantUID
	}
	defer func() {
		migration.UUIDGenerateFn = oldUUIDGenerateFn
	}()

	fakeClient := fake.NewClientBuilder().Build()
	unhealthyPod := makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Problems: []Problem{
			{
				SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
				Reason:  "just for test",
			},
		},
	}, map[string]string{"labelA": "1"}, nil)
	unhealthyPod.Spec.NodeName = "test-node-1"
	unhealthyPod.Status.Phase = v1.PodRunning
	unhealthyPod.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}

	err := fakeClient.Create(context.TODO(), unhealthyPod)
	assert.NoError(err)

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				RebindCPU: &unifiedsev1.OrchestratingSLORebindCPUParams{
					DryRun: true,
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.RebindCPUOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          1,
			HealthyReplicas:   0,
			UnhealthyReplicas: 1,
			AvailableReplicas: 1,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 1,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)
	assumeCache.add(unhealthyPod)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.RebindCPUOrchestratingSLOOperation: {unhealthyPod},
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	operationStatus, err := r.getPodOperationStatus(unhealthyPod)
	assert.NoError(err)
	assert.True(operationStatus == SLOOperationStatusPending)

	pod := &v1.Pod{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-pod-1"}, pod)
	assert.NoError(err)
	opsCtx, err := GetOperationContext(pod, unifiedsev1.RebindCPUOrchestratingSLOOperation)
	assert.NoError(err)
	assert.NotNil(opsCtx)
	assert.NotNil(opsCtx.LastUpdateTime)
	opsCtx.LastUpdateTime = nil
	expectOpsCtx := &OperationContext{
		LastUpdateTime: nil,
		Operation:      string(unifiedsev1.RebindCPUOrchestratingSLOOperation),
		Count:          1,
	}
	assert.Equal(expectOpsCtx, opsCtx)

	rebindResources := &unifiedsev1.RebindResourceList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"sigma.ali/node-sn": "test-node-1",
		}),
	}
	err = r.Client.List(context.TODO(), rebindResources, listOpts)
	assert.NoError(err)
	assert.Len(rebindResources.Items, 0)
}

func TestReconciler_filterNominatedPods(t *testing.T) {
	assert := assert.New(t)
	tests := []struct {
		name                string
		unhealthyPods       []*v1.Pod
		operationContexts   map[string][]OperationContext
		unavailablePods     sets.String
		retryPeriod         time.Duration
		stabilizationWindow time.Duration
		maxRetryCount       int
		operationType       unifiedsev1.OrchestratingSLOOperationType
		wantNominated       []types.NamespacedName
		wantSkipped         []types.NamespacedName
		wantIncurable       map[types.NamespacedName]string
		wantNextRetryCounts map[types.NamespacedName]int
		wantErr             bool
	}{
		{
			name: "first cured",
			unhealthyPods: []*v1.Pod{
				makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
				makeUnhealthyPods(t, "default", "test-pod-2", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
			},
			operationContexts:   nil,
			retryPeriod:         defaultRebindCPURetryPeriod,
			stabilizationWindow: defaultRebindCPUStabilizationWindow,
			maxRetryCount:       defaultRebindCPUMaxRetryCount,
			operationType:       unifiedsev1.RebindCPUOrchestratingSLOOperation,
			wantNominated: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-1"},
				{Namespace: "default", Name: "test-pod-2"},
			},
			wantNextRetryCounts: map[types.NamespacedName]int{
				{Namespace: "default", Name: "test-pod-1"}: 1,
				{Namespace: "default", Name: "test-pod-2"}: 1,
			},
		},
		{
			name: "skip unavailable pods",
			unhealthyPods: []*v1.Pod{
				makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
				makeUnhealthyPods(t, "default", "test-pod-2", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
			},
			unavailablePods: map[string]sets.Empty{
				"default/test-pod-1": {},
			},
			operationContexts:   nil,
			retryPeriod:         defaultRebindCPURetryPeriod,
			stabilizationWindow: defaultRebindCPUStabilizationWindow,
			maxRetryCount:       defaultRebindCPUMaxRetryCount,
			operationType:       unifiedsev1.RebindCPUOrchestratingSLOOperation,
			wantNominated: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-2"},
			},
			wantSkipped: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-1"},
			},
			wantNextRetryCounts: map[types.NamespacedName]int{
				{Namespace: "default", Name: "test-pod-2"}: 1,
			},
		},
		{
			name: "skip operation Pending pods",
			unhealthyPods: []*v1.Pod{
				makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, map[string]string{LabelOrchestratingSLOOperationStatus: string(SLOOperationStatusPending)}, nil),
				makeUnhealthyPods(t, "default", "test-pod-2", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
			},
			retryPeriod:         defaultRebindCPURetryPeriod,
			stabilizationWindow: defaultRebindCPUStabilizationWindow,
			maxRetryCount:       defaultRebindCPUMaxRetryCount,
			operationType:       unifiedsev1.RebindCPUOrchestratingSLOOperation,
			wantNominated: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-2"},
			},
			wantSkipped: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-1"},
			},
			wantNextRetryCounts: map[types.NamespacedName]int{
				{Namespace: "default", Name: "test-pod-2"}: 1,
			},
		},
		{
			name: "skip pods that in retryPeriod",
			unhealthyPods: []*v1.Pod{
				makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, map[string]string{LabelOrchestratingSLOOperationStatus: string(SLOOperationStatusRunning)}, nil),
				makeUnhealthyPods(t, "default", "test-pod-2", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
			},
			operationContexts: map[string][]OperationContext{
				"default/test-pod-1": {
					{
						LastUpdateTime: &metav1.Time{Time: time.Now()},
						Operation:      string(unifiedsev1.RebindCPUOrchestratingSLOOperation),
						Count:          1,
					},
				},
			},
			retryPeriod:         defaultRebindCPURetryPeriod,
			stabilizationWindow: defaultRebindCPUStabilizationWindow,
			maxRetryCount:       defaultRebindCPUMaxRetryCount,
			operationType:       unifiedsev1.RebindCPUOrchestratingSLOOperation,
			wantNominated: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-2"},
			},
			wantSkipped: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-1"},
			},
			wantNextRetryCounts: map[types.NamespacedName]int{
				{Namespace: "default", Name: "test-pod-2"}: 1,
			},
		},
		{
			name: "incurable pods that exceeded max retryCount",
			unhealthyPods: []*v1.Pod{
				makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, map[string]string{LabelOrchestratingSLOOperationStatus: string(SLOOperationStatusRunning)}, nil),
				makeUnhealthyPods(t, "default", "test-pod-2", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
			},
			operationContexts: map[string][]OperationContext{
				"default/test-pod-1": {
					{
						LastUpdateTime: &metav1.Time{Time: time.Now().Add(-defaultRebindCPURetryPeriod)},
						Operation:      string(unifiedsev1.RebindCPUOrchestratingSLOOperation),
						Count:          defaultRebindCPUMaxRetryCount,
					},
				},
			},
			retryPeriod:         defaultRebindCPURetryPeriod,
			stabilizationWindow: defaultRebindCPUStabilizationWindow,
			maxRetryCount:       defaultRebindCPUMaxRetryCount,
			operationType:       unifiedsev1.RebindCPUOrchestratingSLOOperation,
			wantNominated: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-2"},
			},
			wantIncurable: map[types.NamespacedName]string{
				{Namespace: "default", Name: "test-pod-1"}: "the Pod has try to RebindCPU 3 counts, it's incurable in stabilizationWindow(30m0s)",
			},
			wantNextRetryCounts: map[types.NamespacedName]int{
				{Namespace: "default", Name: "test-pod-2"}: 1,
			},
		},
		{
			name: "nominated pods that exceeded max retryCount and stabilization window",
			unhealthyPods: []*v1.Pod{
				makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, map[string]string{LabelOrchestratingSLOOperationStatus: string(SLOOperationStatusRunning)}, nil),
				makeUnhealthyPods(t, "default", "test-pod-2", &ProblemContext{
					Problems: []Problem{
						{
							SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
							Reason:  "just for test",
						},
					},
				}, nil, nil),
			},
			operationContexts: map[string][]OperationContext{
				"default/test-pod-1": {
					{
						LastUpdateTime: &metav1.Time{Time: time.Now().Add(-defaultRebindCPUStabilizationWindow)},
						Operation:      string(unifiedsev1.RebindCPUOrchestratingSLOOperation),
						Count:          defaultRebindCPUMaxRetryCount,
					},
				},
			},
			retryPeriod:         defaultRebindCPURetryPeriod,
			stabilizationWindow: defaultRebindCPUStabilizationWindow,
			maxRetryCount:       defaultRebindCPUMaxRetryCount,
			operationType:       unifiedsev1.RebindCPUOrchestratingSLOOperation,
			wantNominated: []types.NamespacedName{
				{Namespace: "default", Name: "test-pod-1"},
				{Namespace: "default", Name: "test-pod-2"},
			},
			wantNextRetryCounts: map[types.NamespacedName]int{
				{Namespace: "default", Name: "test-pod-1"}: 1,
				{Namespace: "default", Name: "test-pod-2"}: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().Build()
			r := &Reconciler{
				Client:        fakeClient,
				eventRecorder: record.NewFakeRecorder(1024),
				podCache:      NewPodAssumeCache(nil),
				rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
			}
			assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)
			for _, pod := range tt.unhealthyPods {
				name := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
				if !tt.unavailablePods.Has(name) {
					pod.Status.Phase = v1.PodRunning
					pod.Status.Conditions = []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
					}
				}

				contexts := tt.operationContexts[name]
				if contexts != nil {
					data, err := json.Marshal(contexts)
					assert.NoError(err)
					assert.NotEmpty(data)
					if pod.Annotations == nil {
						pod.Annotations = map[string]string{}
					}
					pod.Annotations[AnnotationOrchestratingSLOOperationContext] = string(data)
				}

				err := fakeClient.Create(context.TODO(), pod)
				assert.NoError(err)
				assumeCache.add(pod)
			}
			request := reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: "default",
				Name:      "test-slo-1",
			}}
			nominated, skipped, gotIncurable, nextRetryCount, err := r.filterNominatedPods(request, tt.unhealthyPods, tt.retryPeriod, tt.stabilizationWindow, tt.maxRetryCount, tt.operationType)
			if (err != nil) != tt.wantErr {
				t.Errorf("filterNominatedPods() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			var gotNominated []types.NamespacedName
			var gotSkipped []types.NamespacedName
			gotNextRetryCount := map[types.NamespacedName]int{}
			for _, v := range nominated {
				gotNominated = append(gotNominated, types.NamespacedName{
					Namespace: v.Namespace,
					Name:      v.Name,
				})
			}
			for _, v := range skipped {
				gotSkipped = append(gotSkipped, types.NamespacedName{
					Namespace: v.Namespace,
					Name:      v.Name,
				})
			}
			for uid, v := range nextRetryCount {
				found := false
				for _, vv := range tt.unhealthyPods {
					if uid == vv.UID {
						found = true
						gotNextRetryCount[types.NamespacedName{
							Namespace: vv.Namespace,
							Name:      vv.Name,
						}] = v
						break
					}
				}
				if found {
					continue
				}
			}

			if !reflect.DeepEqual(gotNominated, tt.wantNominated) {
				t.Errorf("filterNominatedPods() gotNominated = %v, want %v", gotNominated, tt.wantNominated)
			}
			if !reflect.DeepEqual(gotSkipped, tt.wantSkipped) {
				t.Errorf("filterNominatedPods() gotSkipped = %v, want %v", gotSkipped, tt.wantSkipped)
			}
			if !reflect.DeepEqual(gotIncurable, tt.wantIncurable) {
				t.Errorf("filterNominatedPods() gotIncurable = %v, want %v", gotIncurable, tt.wantIncurable)
			}
			if !reflect.DeepEqual(gotNextRetryCount, tt.wantNextRetryCounts) {
				t.Errorf("filterNominatedPods() gotNextRetryCounts = %v, want %v", gotNextRetryCount, tt.wantNextRetryCounts)
			}
		})
	}
}

func TestMigrateFlow(t *testing.T) {
	assert := assert.New(t)

	constantUID := uuid.NewUUID()

	oldUUIDGenerateFn := migration.UUIDGenerateFn
	migration.UUIDGenerateFn = func() types.UID {
		return constantUID
	}
	defer func() {
		migration.UUIDGenerateFn = oldUUIDGenerateFn
	}()

	fakeClient := fake.NewClientBuilder().Build()
	unhealthyPod := makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Problems: []Problem{
			{
				SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
				Reason:  "just for test",
			},
		},
	}, map[string]string{"labelA": "1"}, nil)
	unhealthyPod.Spec.NodeName = "test-node-1"
	unhealthyPod.Status.Phase = v1.PodRunning
	unhealthyPod.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}

	err := fakeClient.Create(context.TODO(), unhealthyPod)
	assert.NoError(err)

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				Migrate: &unifiedsev1.OrchestratingSLOMigrateParams{
					DryRun: false,
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          1,
			HealthyReplicas:   0,
			UnhealthyReplicas: 1,
			AvailableReplicas: 1,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 1,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		handle:        fakeEvictor{Client: fakeClient},
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)
	assumeCache.add(unhealthyPod)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.MigrateOrchestratingSLOOperation: {unhealthyPod},
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	operationStatus, err := r.getPodOperationStatus(unhealthyPod)
	assert.NoError(err)
	assert.True(operationStatus == SLOOperationStatusPending)

	pod := &v1.Pod{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-pod-1"}, pod)
	assert.NoError(err)
	opsCtx, err := GetOperationContext(pod, unifiedsev1.MigrateOrchestratingSLOOperation)
	assert.NoError(err)
	assert.NotNil(opsCtx)
	assert.NotNil(opsCtx.LastUpdateTime)
	opsCtx.LastUpdateTime = nil
	expectOpsCtx := &OperationContext{
		LastUpdateTime: nil,
		Operation:      string(unifiedsev1.MigrateOrchestratingSLOOperation),
		Count:          1,
	}
	assert.Equal(expectOpsCtx, opsCtx)

	migrateJobList := &koordsev1alpha1.PodMigrationJobList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			LabelOrchestratingSLOName: orchestratingSLO.Name,
		}),
	}
	err = r.Client.List(context.TODO(), migrateJobList, listOpts)
	assert.NoError(err)
	assert.Len(migrateJobList.Items, 1)

	expectMigrateJob := &koordsev1alpha1.PodMigrationJob{
		ObjectMeta: metav1.ObjectMeta{
			ResourceVersion: "1",
			Name:            string(constantUID),
			Labels: map[string]string{
				unified.LabelPodMigrateFromNode: "test-node-1",
				unified.LabelPodMigrateTicket:   string(constantUID),
				unified.LabelPodMigrateTrigger:  "descheduler",
				LabelOrchestratingSLONamespace:  orchestratingSLO.Namespace,
				LabelOrchestratingSLOName:       orchestratingSLO.Name,
				evictor.LabelEvictPolicy:        "LabelBasedEvictor",
			},
			Annotations: map[string]string{
				unified.AnnotationPodMigrateReason: "Violating the SLO of Orchestration",
			},
		},
		Spec: koordsev1alpha1.PodMigrationJobSpec{
			PodRef: &v1.ObjectReference{
				Namespace: pod.Namespace,
				Name:      pod.Name,
				UID:       pod.UID,
			},
			Mode: koordsev1alpha1.PodMigrationJobModeReservationFirst,
			TTL: &metav1.Duration{
				Duration: defaultMigrateJobTimeout,
			},
		},
		Status: koordsev1alpha1.PodMigrationJobStatus{
			Phase: koordsev1alpha1.PodMigrationJobPending,
		},
	}
	assert.Equal(expectMigrateJob, &migrateJobList.Items[0])
}

func TestMigrateFlowWithDryRun(t *testing.T) {
	assert := assert.New(t)

	constantUID := uuid.NewUUID()

	oldUUIDGenerateFn := migration.UUIDGenerateFn
	migration.UUIDGenerateFn = func() types.UID {
		return constantUID
	}
	defer func() {
		migration.UUIDGenerateFn = oldUUIDGenerateFn
	}()

	fakeClient := fake.NewClientBuilder().Build()
	unhealthyPod := makeUnhealthyPods(t, "default", "test-pod-1", &ProblemContext{
		LastUpdateTime: &metav1.Time{Time: time.Now()},
		Problems: []Problem{
			{
				SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
				Reason:  "just for test",
			},
		},
	}, map[string]string{"labelA": "1"}, nil)
	unhealthyPod.Spec.NodeName = "test-node-1"
	unhealthyPod.Status.Phase = v1.PodRunning
	unhealthyPod.Status.Conditions = []v1.PodCondition{
		{
			Type:   v1.PodReady,
			Status: v1.ConditionTrue,
		},
	}

	err := fakeClient.Create(context.TODO(), unhealthyPod)
	assert.NoError(err)

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				Migrate: &unifiedsev1.OrchestratingSLOMigrateParams{
					DryRun: true,
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          1,
			HealthyReplicas:   0,
			UnhealthyReplicas: 1,
			AvailableReplicas: 1,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 1,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)
	assumeCache.add(unhealthyPod)

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.MigrateOrchestratingSLOOperation: {unhealthyPod},
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	operationStatus, err := r.getPodOperationStatus(unhealthyPod)
	assert.NoError(err)
	assert.True(operationStatus == SLOOperationStatusPending)

	pod := &v1.Pod{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "test-pod-1"}, pod)
	assert.NoError(err)
	opsCtx, err := GetOperationContext(pod, unifiedsev1.MigrateOrchestratingSLOOperation)
	assert.NoError(err)
	assert.NotNil(opsCtx)
	assert.NotNil(opsCtx.LastUpdateTime)
	opsCtx.LastUpdateTime = nil
	expectOpsCtx := &OperationContext{
		LastUpdateTime: nil,
		Operation:      string(unifiedsev1.MigrateOrchestratingSLOOperation),
		Count:          1,
	}
	assert.Equal(expectOpsCtx, opsCtx)

	migrateJobList := &koordsev1alpha1.PodMigrationJobList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			LabelOrchestratingSLOName: orchestratingSLO.Name,
		}),
	}
	err = r.Client.List(context.TODO(), migrateJobList, listOpts)
	assert.NoError(err)
	assert.Empty(migrateJobList.Items)
}

func TestMigrateFlowWithMaxMigratingEqualMaxUnavailable(t *testing.T) {
	assert := assert.New(t)

	replicas := 3

	fakeClient := fake.NewClientBuilder().Build()
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		handle:        fakeEvictor{Client: fakeClient},
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)

	var unhealthyPods []*v1.Pod
	for i := 0; i < replicas; i++ {
		unhealthyPod := makeUnhealthyPods(t, "default", fmt.Sprintf("test-pod-%d", i), &ProblemContext{
			LastUpdateTime: &metav1.Time{Time: time.Now()},
			Problems: []Problem{
				{
					SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
					Reason:  "just for test",
				},
			},
		}, map[string]string{"labelA": "1"}, nil)
		unhealthyPod.Spec.NodeName = "test-node-1"
		unhealthyPod.Status.Phase = v1.PodRunning
		unhealthyPod.Status.Conditions = []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
		}

		err := fakeClient.Create(context.TODO(), unhealthyPod)
		assert.NoError(err)
		assumeCache.add(unhealthyPod)
		unhealthyPods = append(unhealthyPods, unhealthyPod)
	}

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				Migrate: &unifiedsev1.OrchestratingSLOMigrateParams{
					DryRun: false,
					FlowControlPolicy: &unifiedsev1.OrchestratingSLOFlowControlPolicy{
						QPS:   "1000",
						Burst: 100,
					},
					MaxMigrating:        fromInt(2),
					MaxUnavailable:      fromInt(2),
					MaxMigratingPerNode: pointer.Int32Ptr(2),
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          2,
			HealthyReplicas:   0,
			UnhealthyReplicas: 2,
			AvailableReplicas: 2,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 2,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-0",
						},
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.MigrateOrchestratingSLOOperation: unhealthyPods,
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	expectedOperationStatus := map[types.UID]SLOOperationStatus{
		unhealthyPods[0].UID: SLOOperationStatusPending,
		unhealthyPods[1].UID: SLOOperationStatusPending,
		unhealthyPods[2].UID: SLOOperationStatusUnknown,
	}

	expectedOperationContexts := map[types.UID]*OperationContext{
		unhealthyPods[0].UID: {
			LastUpdateTime: nil,
			Operation:      string(unifiedsev1.MigrateOrchestratingSLOOperation),
			Count:          1,
		},
		unhealthyPods[1].UID: {
			LastUpdateTime: nil,
			Operation:      string(unifiedsev1.MigrateOrchestratingSLOOperation),
			Count:          1,
		},
		unhealthyPods[2].UID: {},
	}

	for _, unhealthyPod := range unhealthyPods {
		operationStatus, err := r.getPodOperationStatus(unhealthyPod)
		assert.NoError(err)
		expectedStatus := expectedOperationStatus[unhealthyPod.UID]
		assert.True(operationStatus == expectedStatus)

		pod := &v1.Pod{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: unhealthyPod.Name}, pod)
		assert.NoError(err)
		opsCtx, err := GetOperationContext(pod, unifiedsev1.MigrateOrchestratingSLOOperation)
		assert.NoError(err)
		assert.NotNil(opsCtx)
		opsCtx.LastUpdateTime = nil

		expectOpsCtx := expectedOperationContexts[unhealthyPod.UID]
		assert.Equal(expectOpsCtx, opsCtx)
	}

	migrateJobList := &koordsev1alpha1.PodMigrationJobList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			LabelOrchestratingSLOName: orchestratingSLO.Name,
		}),
	}
	err = r.Client.List(context.TODO(), migrateJobList, listOpts)
	assert.NoError(err)
	assert.Len(migrateJobList.Items, 2)

	sort.Slice(migrateJobList.Items, func(i, j int) bool {
		return migrateJobList.Items[i].Spec.PodRef.Name < migrateJobList.Items[j].Spec.PodRef.Name
	})

	for i, unhealthyPod := range unhealthyPods {
		if i+1 >= replicas {
			break
		}
		job := &migrateJobList.Items[i]
		expectMigrateJob := &koordsev1alpha1.PodMigrationJob{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
				Name:            job.Name,
				Labels: map[string]string{
					unified.LabelPodMigrateFromNode: "test-node-1",
					unified.LabelPodMigrateTicket:   job.Labels[unified.LabelPodMigrateTicket],
					unified.LabelPodMigrateTrigger:  "descheduler",
					LabelOrchestratingSLONamespace:  orchestratingSLO.Namespace,
					LabelOrchestratingSLOName:       orchestratingSLO.Name,
					evictor.LabelEvictPolicy:        "LabelBasedEvictor",
				},
				Annotations: map[string]string{
					unified.AnnotationPodMigrateReason: "Violating the SLO of Orchestration",
				},
			},
			Spec: koordsev1alpha1.PodMigrationJobSpec{
				PodRef: &v1.ObjectReference{
					Namespace: unhealthyPod.Namespace,
					Name:      unhealthyPod.Name,
					UID:       unhealthyPod.UID,
				},
				Mode: koordsev1alpha1.PodMigrationJobModeReservationFirst,
				TTL: &metav1.Duration{
					Duration: defaultMigrateJobTimeout,
				},
			},
			Status: koordsev1alpha1.PodMigrationJobStatus{
				Phase: koordsev1alpha1.PodMigrationJobPending,
			},
		}
		assert.Equal(expectMigrateJob, &migrateJobList.Items[i])
	}
}

func TestMigrateFlowWithMaxMigratingGreaterThanMaxUnavailable(t *testing.T) {
	assert := assert.New(t)

	replicas := 3

	fakeClient := fake.NewClientBuilder().Build()
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		handle:        fakeEvictor{Client: fakeClient},
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)

	var unhealthyPods []*v1.Pod
	for i := 0; i < replicas; i++ {
		unhealthyPod := makeUnhealthyPods(t, "default", fmt.Sprintf("test-pod-%d", i), &ProblemContext{
			LastUpdateTime: &metav1.Time{Time: time.Now()},
			Problems: []Problem{
				{
					SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
					Reason:  "just for test",
				},
			},
		}, map[string]string{"labelA": "1"}, nil)
		unhealthyPod.Spec.NodeName = "test-node-1"
		unhealthyPod.Status.Phase = v1.PodRunning
		unhealthyPod.Status.Conditions = []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
		}

		err := fakeClient.Create(context.TODO(), unhealthyPod)
		assert.NoError(err)
		assumeCache.add(unhealthyPod)
		unhealthyPods = append(unhealthyPods, unhealthyPod)
	}

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				Migrate: &unifiedsev1.OrchestratingSLOMigrateParams{
					DryRun: false,
					FlowControlPolicy: &unifiedsev1.OrchestratingSLOFlowControlPolicy{
						QPS:   "1000",
						Burst: 100,
					},
					MaxMigrating:        fromInt(1),
					MaxUnavailable:      fromInt(2),
					MaxMigratingPerNode: pointer.Int32Ptr(2),
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          2,
			HealthyReplicas:   0,
			UnhealthyReplicas: 2,
			AvailableReplicas: 2,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 2,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-0",
						},
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.MigrateOrchestratingSLOOperation: unhealthyPods,
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	expectedOperationStatus := map[types.UID]SLOOperationStatus{
		unhealthyPods[0].UID: SLOOperationStatusPending,
		unhealthyPods[1].UID: SLOOperationStatusUnknown,
		unhealthyPods[2].UID: SLOOperationStatusUnknown,
	}

	expectedOperationContexts := map[types.UID]*OperationContext{
		unhealthyPods[0].UID: {
			LastUpdateTime: nil,
			Operation:      string(unifiedsev1.MigrateOrchestratingSLOOperation),
			Count:          1,
		},
		unhealthyPods[1].UID: {},
		unhealthyPods[2].UID: {},
	}

	for _, unhealthyPod := range unhealthyPods {
		operationStatus, err := r.getPodOperationStatus(unhealthyPod)
		assert.NoError(err)
		expectedStatus := expectedOperationStatus[unhealthyPod.UID]
		assert.True(operationStatus == expectedStatus)

		pod := &v1.Pod{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: unhealthyPod.Name}, pod)
		assert.NoError(err)
		opsCtx, err := GetOperationContext(pod, unifiedsev1.MigrateOrchestratingSLOOperation)
		assert.NoError(err)
		assert.NotNil(opsCtx)
		opsCtx.LastUpdateTime = nil

		expectOpsCtx := expectedOperationContexts[unhealthyPod.UID]
		assert.Equal(expectOpsCtx, opsCtx)
	}

	migrateJobList := &koordsev1alpha1.PodMigrationJobList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			LabelOrchestratingSLOName: orchestratingSLO.Name,
		}),
	}
	err = r.Client.List(context.TODO(), migrateJobList, listOpts)
	assert.NoError(err)
	assert.Len(migrateJobList.Items, 1)

	for i, unhealthyPod := range unhealthyPods {
		if i+1 >= replicas-1 {
			break
		}
		job := &migrateJobList.Items[i]
		expectMigrateJob := &koordsev1alpha1.PodMigrationJob{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
				Name:            job.Name,
				Labels: map[string]string{
					unified.LabelPodMigrateFromNode: "test-node-1",
					unified.LabelPodMigrateTicket:   job.Labels[unified.LabelPodMigrateTicket],
					unified.LabelPodMigrateTrigger:  "descheduler",
					LabelOrchestratingSLONamespace:  orchestratingSLO.Namespace,
					LabelOrchestratingSLOName:       orchestratingSLO.Name,
					evictor.LabelEvictPolicy:        "LabelBasedEvictor",
				},
				Annotations: map[string]string{
					unified.AnnotationPodMigrateReason: "Violating the SLO of Orchestration",
				},
			},
			Spec: koordsev1alpha1.PodMigrationJobSpec{
				PodRef: &v1.ObjectReference{
					Namespace: unhealthyPod.Namespace,
					Name:      unhealthyPod.Name,
					UID:       unhealthyPod.UID,
				},
				Mode: koordsev1alpha1.PodMigrationJobModeReservationFirst,
				TTL: &metav1.Duration{
					Duration: defaultMigrateJobTimeout,
				},
			},
			Status: koordsev1alpha1.PodMigrationJobStatus{
				Phase: koordsev1alpha1.PodMigrationJobPending,
			},
		}
		assert.Equal(expectMigrateJob, &migrateJobList.Items[i])
	}
}

func TestMigrateFlowWithMaxMigratingPerNode(t *testing.T) {
	assert := assert.New(t)

	replicas := 3

	fakeClient := fake.NewClientBuilder().Build()
	fakeEventRecorder := record.NewFakeRecorder(1024)
	r := &Reconciler{
		Client:        fakeClient,
		handle:        fakeEvictor{Client: fakeClient},
		eventRecorder: fakeEventRecorder,
		podCache:      NewPodAssumeCache(nil),
		rateLimiters:  map[reconcile.Request]map[unifiedsev1.OrchestratingSLOOperationType]*RateLimiter{},
	}
	assumeCache := r.podCache.(*podAssumeCache).AssumeCache.(*assumeCache)

	var unhealthyPods []*v1.Pod
	for i := 0; i < replicas; i++ {
		unhealthyPod := makeUnhealthyPods(t, "default", fmt.Sprintf("test-pod-%d", i), &ProblemContext{
			LastUpdateTime: &metav1.Time{Time: time.Now()},
			Problems: []Problem{
				{
					SLOType: string(unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA),
					Reason:  "just for test",
				},
			},
		}, map[string]string{"labelA": "1"}, nil)
		unhealthyPod.Spec.NodeName = "test-node-1"
		unhealthyPod.Status.Phase = v1.PodRunning
		unhealthyPod.Status.Conditions = []v1.PodCondition{
			{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			},
		}

		err := fakeClient.Create(context.TODO(), unhealthyPod)
		assert.NoError(err)
		assumeCache.add(unhealthyPod)
		unhealthyPods = append(unhealthyPods, unhealthyPod)
	}

	orchestratingSLO := &unifiedsev1.OrchestratingSLO{
		ObjectMeta: metav1.ObjectMeta{
			UID:       uuid.NewUUID(),
			Namespace: "default",
			Name:      "test-slo-1",
		},
		Spec: unifiedsev1.OrchestratingSLOSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"labelA": "1"}},
			Operation: &unifiedsev1.OrchestratingSLOOperation{
				Migrate: &unifiedsev1.OrchestratingSLOMigrateParams{
					DryRun: false,
					FlowControlPolicy: &unifiedsev1.OrchestratingSLOFlowControlPolicy{
						QPS:   "1000",
						Burst: 100,
					},
					MaxMigrating:        fromInt(3),
					MaxUnavailable:      fromInt(3),
					MaxMigratingPerNode: pointer.Int32Ptr(1),
				},
			},
			Sloes: []unifiedsev1.OrchestratingSLODefinition{
				{
					Type:  unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value: "99.99%",
					Operations: []unifiedsev1.OrchestratingSLOOperationType{
						unifiedsev1.MigrateOrchestratingSLOOperation,
					},
				},
			},
		},
		Status: unifiedsev1.OrchestratingSLOStatus{
			Replicas:          2,
			HealthyReplicas:   0,
			UnhealthyReplicas: 2,
			AvailableReplicas: 2,
			MigratingReplicas: 0,
			Sloes: []unifiedsev1.OrchestratingSLODefinitionStatus{
				{
					Type:              unifiedsev1.OrchestratingSLOGuaranteeCoresFitNUMA,
					Value:             "0.00%",
					HealthyReplicas:   0,
					UnhealthyReplicas: 2,
					UnhealthyPods: []unifiedsev1.OrchestratingPodRecord{
						{
							Namespace: "default",
							Name:      "test-pod-0",
						},
						{
							Namespace: "default",
							Name:      "test-pod-1",
						},
					},
				},
			},
		},
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-slo-1",
		},
	}
	result, err := r.executeOperations(request, orchestratingSLO, map[unifiedsev1.OrchestratingSLOOperationType][]*v1.Pod{
		unifiedsev1.MigrateOrchestratingSLOOperation: unhealthyPods,
	})
	assert.NoError(err)
	assert.True(result.RequeueAfter > 0)

	expectedOperationStatus := map[types.UID]SLOOperationStatus{
		unhealthyPods[0].UID: SLOOperationStatusPending,
		unhealthyPods[1].UID: SLOOperationStatusUnknown,
		unhealthyPods[2].UID: SLOOperationStatusUnknown,
	}

	expectedOperationContexts := map[types.UID]*OperationContext{
		unhealthyPods[0].UID: {
			LastUpdateTime: nil,
			Operation:      string(unifiedsev1.MigrateOrchestratingSLOOperation),
			Count:          1,
		},
		unhealthyPods[1].UID: {},
		unhealthyPods[2].UID: {},
	}

	for _, unhealthyPod := range unhealthyPods {
		operationStatus, err := r.getPodOperationStatus(unhealthyPod)
		assert.NoError(err)
		expectedStatus := expectedOperationStatus[unhealthyPod.UID]
		assert.True(operationStatus == expectedStatus)

		pod := &v1.Pod{}
		err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: unhealthyPod.Name}, pod)
		assert.NoError(err)
		opsCtx, err := GetOperationContext(pod, unifiedsev1.MigrateOrchestratingSLOOperation)
		assert.NoError(err)
		assert.NotNil(opsCtx)
		opsCtx.LastUpdateTime = nil

		expectOpsCtx := expectedOperationContexts[unhealthyPod.UID]
		assert.Equal(expectOpsCtx, opsCtx)
	}

	migrateJobList := &koordsev1alpha1.PodMigrationJobList{}
	listOpts := &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(labels.Set{
			LabelOrchestratingSLOName: orchestratingSLO.Name,
		}),
	}
	err = r.Client.List(context.TODO(), migrateJobList, listOpts)
	assert.NoError(err)
	assert.Len(migrateJobList.Items, 1)

	for i, unhealthyPod := range unhealthyPods {
		if i+1 >= replicas-1 {
			break
		}
		job := &migrateJobList.Items[i]
		expectMigrateJob := &koordsev1alpha1.PodMigrationJob{
			ObjectMeta: metav1.ObjectMeta{
				ResourceVersion: "1",
				Name:            job.Name,
				Labels: map[string]string{
					unified.LabelPodMigrateFromNode: "test-node-1",
					unified.LabelPodMigrateTicket:   job.Labels[unified.LabelPodMigrateTicket],
					unified.LabelPodMigrateTrigger:  "descheduler",
					LabelOrchestratingSLONamespace:  orchestratingSLO.Namespace,
					LabelOrchestratingSLOName:       orchestratingSLO.Name,
					evictor.LabelEvictPolicy:        "LabelBasedEvictor",
				},
				Annotations: map[string]string{
					unified.AnnotationPodMigrateReason: "Violating the SLO of Orchestration",
				},
			},
			Spec: koordsev1alpha1.PodMigrationJobSpec{
				PodRef: &v1.ObjectReference{
					Namespace: unhealthyPod.Namespace,
					Name:      unhealthyPod.Name,
					UID:       unhealthyPod.UID,
				},
				Mode: koordsev1alpha1.PodMigrationJobModeReservationFirst,
				TTL: &metav1.Duration{
					Duration: defaultMigrateJobTimeout,
				},
			},
			Status: koordsev1alpha1.PodMigrationJobStatus{
				Phase: koordsev1alpha1.PodMigrationJobPending,
			},
		}
		assert.Equal(expectMigrateJob, &migrateJobList.Items[i])
	}
}

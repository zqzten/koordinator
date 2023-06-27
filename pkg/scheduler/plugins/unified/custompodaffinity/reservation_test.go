package custompodaffinity

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

func TestMaxInstancePerHostUniProtocolReservation(t *testing.T) {
	nodes := []*corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "test-node",
				Labels: map[string]string{},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("96"),
					corev1.ResourceMemory: resource.MustParse("512Gi"),
				},
			},
		},
	}

	assignedAppName := "app_1"
	assignedServiceUnitName := "role_1"

	tests := []struct {
		name             string
		assignedPods     []*corev1.Pod
		reservePods      []*corev1.Pod
		testSpreadPolicy *uniext.PodSpreadPolicy
		testPod          *corev1.Pod
		expectResult     bool
	}{
		{
			name:         "satisfy serviceUnit maxInstancePerHost",
			expectResult: true,
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			reservePods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "reserve-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName:         "app_2",
						extunified.SigmaLabelServiceUnitName: assignedServiceUnitName,
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
		{
			name:         "failed serviceUnit maxInstancePerHost",
			expectResult: false,
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			reservePods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "reserve-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName:         assignedAppName,
						extunified.SigmaLabelServiceUnitName: assignedServiceUnitName,
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
		{
			name:         "satisfy app maxInstancePerHost",
			expectResult: true,
			reservePods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "reserve-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName: "app_2",
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
		{
			name:         "failed app maxInstancePerHost",
			expectResult: false,
			assignedPods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "assigned-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			reservePods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "default",
						Name:      "reserve-pod-1",
					},
					Spec: corev1.PodSpec{
						NodeName: nodes[0].Name,
					},
				},
			},
			testSpreadPolicy: &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName: assignedAppName,
					},
				},
			},
			testPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "test-pod-1",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nodes)
			p, err := suit.proxyNew(suit.args, suit.Handle)
			assert.NotNil(t, p)
			assert.Nil(t, err)

			plg := p.(*Plugin)
			suit.start()

			nodeInfo, err := suit.Handle.SnapshotSharedLister().NodeInfos().Get("test-node")
			assert.NoError(t, err)
			assert.NotNil(t, nodeInfo)

			assignedPodSpreadPolicy := &uniext.PodSpreadPolicy{
				MaxInstancePerHost: 1,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						extunified.SigmaLabelAppName:         assignedAppName,
						extunified.SigmaLabelServiceUnitName: assignedServiceUnitName,
					},
				},
			}
			assignedData, err := json.Marshal(assignedPodSpreadPolicy)
			assert.NoError(t, err)
			for _, v := range tt.assignedPods {
				if v.Annotations == nil {
					v.Annotations = make(map[string]string)
				}
				v.Annotations[uniext.AnnotationPodSpreadPolicy] = string(assignedData)
				plg.cache.AddPod(v.Spec.NodeName, v)
			}

			if tt.testPod.Annotations == nil {
				tt.testPod.Annotations = make(map[string]string)
			}
			data, err := json.Marshal(tt.testSpreadPolicy)
			assert.NoError(t, err)
			tt.testPod.Annotations[uniext.AnnotationPodSpreadPolicy] = string(data)

			var matchedReservationInfo []*frameworkext.ReservationInfo
			expectedMatchedReservePods := sets.NewString()
			for _, v := range tt.reservePods {
				if v.Annotations == nil {
					v.Annotations = make(map[string]string)
				}
				v.Annotations[uniext.AnnotationPodSpreadPolicy] = string(data)
				plg.cache.AddPod(v.Spec.NodeName, v)
				matchedReservationInfo = append(matchedReservationInfo, &frameworkext.ReservationInfo{
					Pod: v,
				})
				expectedMatchedReservePods.Insert(getNamespacedName(v.Namespace, v.Name))
			}

			cycleState := framework.NewCycleState()
			status := plg.PreRestoreReservation(context.TODO(), cycleState, tt.testPod)
			assert.Nil(t, status)
			matchedReservePods, status := plg.RestoreReservation(context.TODO(), cycleState, tt.testPod, matchedReservationInfo, nil, nodeInfo)
			assert.Equal(t, expectedMatchedReservePods, matchedReservePods)
			assert.Nil(t, status)
			nodeToStates := map[string]interface{}{"test-node": matchedReservePods}
			status = plg.FinalRestoreReservation(context.TODO(), cycleState, tt.testPod, nodeToStates)
			assert.Nil(t, status)

			_, status = plg.PreFilter(context.TODO(), cycleState, tt.testPod)
			assert.Nil(t, status)
			status = plg.Filter(context.TODO(), cycleState, tt.testPod, nodeInfo)
			if status.IsSuccess() != tt.expectResult {
				t.Fatalf("unexpected status result, expect: %v, status: %v, statusMessage: %s", tt.expectResult, status.IsSuccess(), status.Message())
			}
		})
	}
}

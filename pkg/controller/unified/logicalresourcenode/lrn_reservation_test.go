/*
Copyright 2023 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logicalresourcenode

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	terwayapis "github.com/AliyunContainerService/terway-apis/network.alibabacloud.com/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	utilpointer "k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func TestReconcileWithReservation(t *testing.T) {
	cases := []struct {
		lrn                  *schedulingv1alpha1.LogicalResourceNode
		reservation          *schedulingv1alpha1.Reservation
		node                 *corev1.Node
		currentGeneration    int64
		expectedReservations []*schedulingv1alpha1.Reservation
		expectedStatus       *schedulingv1alpha1.LogicalResourceNodeStatus
	}{
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label": "val",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{
					Requirements: schedulingv1alpha1.LogicalResourceNodeRequirements{
						Resources: map[corev1.ResourceName]resource.Quantity{
							"cpu":            resource.MustParse("64"),
							"memory":         resource.MustParse("512Gi"),
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						NodeSelector: map[string]string{
							"node.koordinator.sh/gpu-model-series": "A800",
						},
					},
				},
			},
			reservation:       nil,
			node:              nil,
			currentGeneration: -1,
			expectedReservations: []*schedulingv1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "lrn-test-gen-0",
						Finalizers: []string{finalizerInternalGC},
						Labels: map[string]string{
							"fake-label":    "val",
							labelOwnedByLRN: "lrn-test",
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "scheduling.koordinator.sh/v1alpha1",
								Kind:               "LogicalResourceNode",
								Name:               "lrn-test",
								Controller:         utilpointer.Bool(true),
								BlockOwnerDeletion: utilpointer.Bool(true),
							},
						},
					},
					Spec: schedulingv1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									"node.koordinator.sh/gpu-model-series": "A800",
								},
								Containers: []corev1.Container{
									{
										Name:            "mock",
										Image:           "mock",
										ImagePullPolicy: corev1.PullIfNotPresent,
										Resources: corev1.ResourceRequirements{
											Requests: map[corev1.ResourceName]resource.Quantity{
												"cpu":            resource.MustParse("64"),
												"memory":         resource.MustParse("512Gi"),
												"nvidia.com/gpu": resource.MustParse("8"),
											},
											Limits: map[corev1.ResourceName]resource.Quantity{
												"cpu":            resource.MustParse("64"),
												"memory":         resource.MustParse("512Gi"),
												"nvidia.com/gpu": resource.MustParse("8"),
											},
										},
										TerminationMessagePolicy: corev1.TerminationMessageReadFile,
									},
								},
								RestartPolicy: corev1.RestartPolicyAlways,
								DNSPolicy:     corev1.DNSClusterFirst,
							},
						},
						Owners: []schedulingv1alpha1.ReservationOwner{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"tenant": "t12345"},
								},
							},
						},
						TTL:            &metav1.Duration{Duration: 0},
						AllocateOnce:   utilpointer.Bool(false),
						AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
					},
				},
			},
			expectedStatus: &schedulingv1alpha1.LogicalResourceNodeStatus{Phase: schedulingv1alpha1.LogicalResourceNodePending},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label": "val",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{
					Requirements: schedulingv1alpha1.LogicalResourceNodeRequirements{
						Resources: map[corev1.ResourceName]resource.Quantity{
							"cpu":            resource.MustParse("64"),
							"memory":         resource.MustParse("512Gi"),
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						NodeSelector: map[string]string{
							"node.koordinator.sh/gpu-model-series": "A800",
						},
					},
				},
			},
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test-gen-0",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":    "val",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"node.koordinator.sh/asw-id":            "XXX-P1-S1",
						"node.koordinator.sh/point-of-delivery": "XXX-P1",
						"node.koordinator.sh/gpu-model-series":  "A800",
						"node.koordinator.sh/gpu-model":         "A800-SXM4-80GB",
					},
					Name: "fake-node01",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			currentGeneration: 0,
			expectedReservations: []*schedulingv1alpha1.Reservation{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "lrn-test-gen-0",
						Finalizers: []string{finalizerInternalGC},
						Labels: map[string]string{
							"fake-label":    "val",
							labelOwnedByLRN: "lrn-test",
						},
						Annotations: map[string]string{
							apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
						},
						OwnerReferences: []metav1.OwnerReference{
							{
								APIVersion:         "scheduling.koordinator.sh/v1alpha1",
								Kind:               "LogicalResourceNode",
								Name:               "lrn-test",
								Controller:         utilpointer.Bool(true),
								BlockOwnerDeletion: utilpointer.Bool(true),
							},
						},
					},
					Spec: schedulingv1alpha1.ReservationSpec{
						Template: &corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								NodeSelector: map[string]string{
									"node.koordinator.sh/gpu-model-series": "A800",
								},
								Containers: []corev1.Container{
									{
										Name:            "mock",
										Image:           "mock",
										ImagePullPolicy: corev1.PullIfNotPresent,
										Resources: corev1.ResourceRequirements{
											Requests: map[corev1.ResourceName]resource.Quantity{
												"cpu":            resource.MustParse("64"),
												"memory":         resource.MustParse("512Gi"),
												"nvidia.com/gpu": resource.MustParse("8"),
											},
											Limits: map[corev1.ResourceName]resource.Quantity{
												"cpu":            resource.MustParse("64"),
												"memory":         resource.MustParse("512Gi"),
												"nvidia.com/gpu": resource.MustParse("8"),
											},
										},
										TerminationMessagePolicy: corev1.TerminationMessageReadFile,
									},
								},
								RestartPolicy: corev1.RestartPolicyAlways,
								DNSPolicy:     corev1.DNSClusterFirst,
							},
						},
						Owners: []schedulingv1alpha1.ReservationOwner{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"tenant": "t12345"},
								},
							},
						},
						TTL:            &metav1.Duration{Duration: 0},
						AllocateOnce:   utilpointer.Bool(false),
						AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
					},
					Status: schedulingv1alpha1.ReservationStatus{
						NodeName: "fake-node01",
						Phase:    schedulingv1alpha1.ReservationAvailable,
					},
				},
			},
			expectedStatus: &schedulingv1alpha1.LogicalResourceNodeStatus{
				Phase:    schedulingv1alpha1.LogicalResourceNodeAvailable,
				NodeName: "fake-node01",
				NodeStatus: &schedulingv1alpha1.LRNNodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
					PrintColumn: "Ready",
				},
			},
		},
	}

	for i, testCase := range cases {
		t.Run(fmt.Sprintf("Case #%d", i), func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(scheme)
			_ = schedulingv1alpha1.AddToScheme(scheme)
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testCase.lrn)
			if testCase.reservation != nil {
				clientBuilder.WithObjects(testCase.reservation)
			}
			if testCase.node != nil {
				clientBuilder.WithObjects(testCase.node)
			}
			reconciler := reservationReconciler{Client: clientBuilder.Build()}

			gotStatus, err := reconciler.reconcileWithReservation(context.TODO(), testCase.lrn, testCase.reservation, testCase.currentGeneration, nil)
			if err != nil {
				t.Fatalf("failed to reconcile with reservation: %v", err)
			}
			if !apiequality.Semantic.DeepEqual(gotStatus, testCase.expectedStatus) {
				t.Fatalf("expected status %s\n    got %s", util.DumpJSON(testCase.expectedStatus), util.DumpJSON(gotStatus))
			}

			gotReservationList := schedulingv1alpha1.ReservationList{}
			if err := reconciler.List(context.TODO(), &gotReservationList); err != nil {
				t.Fatalf("failed to list reservations: %v", err)
			}
			gotReservations := []*schedulingv1alpha1.Reservation{}
			for i := range gotReservationList.Items {
				obj := gotReservationList.Items[i].DeepCopy()
				obj.ResourceVersion = ""
				gotReservations = append(gotReservations, obj)
			}
			sort.SliceStable(gotReservations, func(i, j int) bool {
				return gotReservations[i].Name < gotReservations[j].Name
			})
			if !apiequality.Semantic.DeepEqual(gotReservations, testCase.expectedReservations) {
				t.Fatalf("expected reservations:\n%s\ngot:\n%s", util.DumpJSON(testCase.expectedReservations), util.DumpJSON(gotReservations))
			}
		})
	}
}

func TestGenerateLRNPatch(t *testing.T) {
	syncNodeLabelsFlag = func() *string {
		labels := "node.koordinator.sh/asw-id,node.koordinator.sh/point-of-delivery,node.koordinator.sh/gpu-model,node.koordinator.sh/gpu-model-series"
		return &labels
	}()

	cases := []struct {
		lrnMeta       *metav1.ObjectMeta
		generation    int64
		reservation   *schedulingv1alpha1.Reservation
		node          *corev1.Node
		expectedPatch *patchObject
	}{
		{
			lrnMeta: &metav1.ObjectMeta{
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			generation: 0,
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"node.koordinator.sh/asw-id":            "XXX-P1-S1",
						"node.koordinator.sh/point-of-delivery": "XXX-P1",
						"node.koordinator.sh/gpu-model-series":  "A800",
						"node.koordinator.sh/gpu-model":         "A800-SXM4-80GB",
					},
					Name: "fake-node01",
				},
			},
			expectedPatch: &patchObject{
				Metadata: patchMetaData{
					Labels: map[string]interface{}{
						labelReservationGeneration:                            "0",
						"node.koordinator.sh/asw-id":                          "XXX-P1-S1",
						"node.koordinator.sh/point-of-delivery":               "XXX-P1",
						"node.koordinator.sh/gpu-model-series":                "A800",
						"node.koordinator.sh/gpu-model":                       "A800-SXM4-80GB",
						schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "fake-node01",
					},
					Annotations: map[string]interface{}{
						schedulingv1alpha1.AnnotationLogicalResourceNodeDevices: `{"gpu":[{"minor":0},{"minor":1},{"minor":2},{"minor":3},{"minor":4},{"minor":5},{"minor":6},{"minor":7}]}`,
						annotationSyncNodeLabels:                                "node.koordinator.sh/asw-id,node.koordinator.sh/gpu-model,node.koordinator.sh/gpu-model-series,node.koordinator.sh/point-of-delivery",
					},
				},
			},
		},
		{
			lrnMeta: &metav1.ObjectMeta{
				Labels: map[string]string{
					labelReservationGeneration:                            "0",
					"node.koordinator.sh/asw-id":                          "XXX-P1-S1",
					"node.koordinator.sh/point-of-delivery":               "XXX-P1",
					"node.koordinator.sh/gpu-model-series":                "A800",
					"node.koordinator.sh/gpu-model":                       "A800-SXM4-80GB",
					"fake-previous-synced-label":                          "foo",
					schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "fake-node01",
				},
				Annotations: map[string]string{
					schedulingv1alpha1.AnnotationLogicalResourceNodeDevices: `{"gpu":[{"minor":0},{"minor":1},{"minor":2},{"minor":3},{"minor":4},{"minor":5},{"minor":6},{"minor":7}]}`,
					annotationSyncNodeLabels:                                "fake-previous-synced-label,node.koordinator.sh/asw-id,node.koordinator.sh/gpu-model,node.koordinator.sh/gpu-model-series,node.koordinator.sh/point-of-delivery",
				},
			},
			generation: 1,
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node02",
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"node.koordinator.sh/asw-id":            "XXX-P1-S1",
						"node.koordinator.sh/point-of-delivery": "XXX-P1",
						"node.koordinator.sh/gpu-model-series":  "H800",
						"node.koordinator.sh/gpu-model":         "H800",
					},
					Name: "fake-node02",
				},
			},
			expectedPatch: &patchObject{
				Metadata: patchMetaData{
					Labels: map[string]interface{}{
						labelReservationGeneration:                            "1",
						"node.koordinator.sh/asw-id":                          "XXX-P1-S1",
						"node.koordinator.sh/point-of-delivery":               "XXX-P1",
						"node.koordinator.sh/gpu-model-series":                "H800",
						"node.koordinator.sh/gpu-model":                       "H800",
						schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: "fake-node02",
						"fake-previous-synced-label":                          nil,
					},
					Annotations: map[string]interface{}{
						schedulingv1alpha1.AnnotationLogicalResourceNodeDevices: `{"gpu":[{"minor":0},{"minor":1},{"minor":2},{"minor":3},{"minor":4},{"minor":5},{"minor":6},{"minor":7}]}`,
						annotationSyncNodeLabels:                                "node.koordinator.sh/asw-id,node.koordinator.sh/gpu-model,node.koordinator.sh/gpu-model-series,node.koordinator.sh/point-of-delivery",
					},
				},
			},
		},
	}

	for i, testCase := range cases {
		t.Run(fmt.Sprintf("Case #%d", i), func(t *testing.T) {
			gotPatch := generateLRNPatch(testCase.lrnMeta, testCase.generation, testCase.reservation, testCase.node)
			if !reflect.DeepEqual(gotPatch, testCase.expectedPatch) {
				t.Fatalf("expected %v, got %v", util.DumpJSON(testCase.expectedPatch), util.DumpJSON(gotPatch))
			}
		})
	}
}

func TestUpdateReservation(t *testing.T) {
	cases := []struct {
		lrn                 *schedulingv1alpha1.LogicalResourceNode
		reservation         *schedulingv1alpha1.Reservation
		qosGroup            *terwayapis.ENIQosGroup
		expectedReservation *schedulingv1alpha1.Reservation
	}{
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label": "val",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{
					Requirements: schedulingv1alpha1.LogicalResourceNodeRequirements{
						Resources: map[corev1.ResourceName]resource.Quantity{
							"cpu":            resource.MustParse("64"),
							"memory":         resource.MustParse("512Gi"),
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						NodeSelector: map[string]string{
							"node.koordinator.sh/gpu-model-series": "A800",
						},
					},
				},
			},
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test-gen-0",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":    "val",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: schedulingv1alpha1.GroupVersion.String(),
					Kind:       "Reservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "lrn-test-gen-0",
					Finalizers:      []string{finalizerInternalGC},
					ResourceVersion: "999",
					Labels: map[string]string{
						"fake-label":    "val",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label": "val",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{
					Requirements: schedulingv1alpha1.LogicalResourceNodeRequirements{
						Resources: map[corev1.ResourceName]resource.Quantity{
							"cpu":            resource.MustParse("64"),
							"memory":         resource.MustParse("512Gi"),
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						NodeSelector: map[string]string{
							"node.koordinator.sh/gpu-model-series": "A800",
						},
					},
				},
			},
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test-gen-0",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":    "val",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Unschedulable: true,
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: schedulingv1alpha1.GroupVersion.String(),
					Kind:       "Reservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "lrn-test-gen-0",
					Finalizers:      []string{finalizerInternalGC},
					ResourceVersion: "999",
					Labels: map[string]string{
						"fake-label":    "val",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Unschedulable: true,
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label": "val",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{
					Requirements: schedulingv1alpha1.LogicalResourceNodeRequirements{
						Resources: map[corev1.ResourceName]resource.Quantity{
							"cpu":            resource.MustParse("64"),
							"memory":         resource.MustParse("512Gi"),
							"nvidia.com/gpu": resource.MustParse("8"),
						},
						NodeSelector: map[string]string{
							"node.koordinator.sh/gpu-model-series": "A800",
						},
					},
				},
			},
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test-gen-0",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":    "val",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Unschedulable: true,
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
			qosGroup: &terwayapis.ENIQosGroup{
				Status: terwayapis.ENIQosGroupStatus{
					AutoCreatedID: "qos-group-123",
					Phase:         terwayapis.QosGroupPhaseReady,
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: schedulingv1alpha1.GroupVersion.String(),
					Kind:       "Reservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "lrn-test-gen-0",
					Finalizers:      []string{finalizerInternalGC},
					ResourceVersion: "1000",
					Labels: map[string]string{
						"fake-label":                          "val",
						labelOwnedByLRN:                       "lrn-test",
						schedulingv1alpha1.LabelVPCQoSGroupID: "qos-group-123",
					},
					Annotations: map[string]string{
						apiext.AnnotationDeviceAllocated: `{"gpu":[{"minor":2,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":3,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":4,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":5,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":6,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":7,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":0,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}},{"minor":1,"resources":{"koordinator.sh/gpu-core":"100","koordinator.sh/gpu-memory":"85197848576","koordinator.sh/gpu-memory-ratio":"100"}}]}`,
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Unschedulable: false,
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeSelector: map[string]string{
								"node.koordinator.sh/gpu-model-series": "A800",
							},
							Containers: []corev1.Container{
								{
									Name:            "mock",
									Image:           "mock",
									ImagePullPolicy: corev1.PullIfNotPresent,
									Resources: corev1.ResourceRequirements{
										Requests: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
										Limits: map[corev1.ResourceName]resource.Quantity{
											"cpu":            resource.MustParse("64"),
											"memory":         resource.MustParse("512Gi"),
											"nvidia.com/gpu": resource.MustParse("8"),
										},
									},
									TerminationMessagePolicy: corev1.TerminationMessageReadFile,
								},
							},
							RestartPolicy: corev1.RestartPolicyAlways,
							DNSPolicy:     corev1.DNSClusterFirst,
						},
					},
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
					TTL:            &metav1.Duration{Duration: 0},
					AllocateOnce:   utilpointer.Bool(false),
					AllocatePolicy: schedulingv1alpha1.ReservationAllocatePolicyRestricted,
				},
				Status: schedulingv1alpha1.ReservationStatus{
					NodeName: "fake-node01",
					Phase:    schedulingv1alpha1.ReservationAvailable,
				},
			},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":   "val",
						"fake-label-1": "aaa",
						"fake-label-2": "aaa",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{},
			},
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test-gen-0",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":    "val",
						"fake-label-2":  "bbb",
						"fake-label-3":  "ccc",
						labelOwnedByLRN: "lrn-test",
					},
					Annotations: map[string]string{},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: schedulingv1alpha1.GroupVersion.String(),
					Kind:       "Reservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "lrn-test-gen-0",
					Finalizers:      []string{finalizerInternalGC},
					ResourceVersion: "1000",
					Labels: map[string]string{
						"fake-label":    "val",
						"fake-label-1":  "aaa",
						"fake-label-2":  "bbb",
						"fake-label-3":  "ccc",
						labelOwnedByLRN: "lrn-test",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{},
			},
		},
		{
			lrn: &schedulingv1alpha1.LogicalResourceNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":        "val",
						"fake-label-1":      "aaa",
						"fake-label-2":      "aaa",
						"fake-sync-label-1": "aaa",
						"fake-sync-label-2": "aaa",
					},
					Annotations: map[string]string{
						schedulingv1alpha1.AnnotationLogicalResourceNodePodLabelSelector: `{"tenant":"t12345"}`,
						schedulingv1alpha1.AnnotationForceSyncLabelRegex:                 `fake-sync-label-[0-9]`,
					},
				},
				Spec: schedulingv1alpha1.LogicalResourceNodeSpec{},
			},
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "lrn-test-gen-0",
					Finalizers: []string{finalizerInternalGC},
					Labels: map[string]string{
						"fake-label":        "val",
						"fake-label-2":      "bbb",
						"fake-label-3":      "ccc",
						"fake-sync-label-2": "bbb",
						"fake-sync-label-3": "ccc",
						labelOwnedByLRN:     "lrn-test",
					},
					Annotations: map[string]string{},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				TypeMeta: metav1.TypeMeta{
					APIVersion: schedulingv1alpha1.GroupVersion.String(),
					Kind:       "Reservation",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "lrn-test-gen-0",
					Finalizers:      []string{finalizerInternalGC},
					ResourceVersion: "1000",
					Labels: map[string]string{
						"fake-label":        "val",
						"fake-label-1":      "aaa",
						"fake-label-2":      "bbb",
						"fake-label-3":      "ccc",
						"fake-sync-label-1": "aaa",
						"fake-sync-label-2": "aaa",
						labelOwnedByLRN:     "lrn-test",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         "scheduling.koordinator.sh/v1alpha1",
							Kind:               "LogicalResourceNode",
							Name:               "lrn-test",
							Controller:         utilpointer.Bool(true),
							BlockOwnerDeletion: utilpointer.Bool(true),
						},
					},
				},
				Spec: schedulingv1alpha1.ReservationSpec{
					Owners: []schedulingv1alpha1.ReservationOwner{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"tenant": "t12345"},
							},
						},
					},
				},
				Status: schedulingv1alpha1.ReservationStatus{},
			},
		},
	}

	for i, testCase := range cases {
		t.Run(fmt.Sprintf("#%d", i), func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = clientgoscheme.AddToScheme(scheme)
			_ = schedulingv1alpha1.AddToScheme(scheme)
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testCase.lrn, testCase.reservation)
			reconciler := reservationReconciler{Client: clientBuilder.Build()}

			if err := reconciler.updateReservation(context.TODO(), testCase.lrn, testCase.reservation, testCase.qosGroup); err != nil {
				t.Fatal(err)
			}
			gotReservation := &schedulingv1alpha1.Reservation{}
			if err := reconciler.Get(context.TODO(), types.NamespacedName{Name: testCase.reservation.Name}, gotReservation); err != nil {
				t.Fatal(err)
			}
			if !apiequality.Semantic.DeepEqual(gotReservation, testCase.expectedReservation) {
				t.Fatalf("expected reservations:\n%s\ngot:\n%s", util.DumpJSON(testCase.expectedReservation), util.DumpJSON(gotReservation))
			}
		})
	}
}

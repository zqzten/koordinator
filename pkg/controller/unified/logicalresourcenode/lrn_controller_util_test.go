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
	"reflect"
	"testing"

	terwayapis "github.com/AliyunContainerService/terway-apis/network.alibabacloud.com/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilpointer "k8s.io/utils/pointer"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func TestGetObjectListNames(t *testing.T) {
	testCases := []struct {
		listObj       interface{}
		expectedNames []string
	}{
		{
			listObj: []*corev1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "n2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "n1"}},
			},
			expectedNames: []string{"n2", "n1"},
		},
		{
			listObj: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "p1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "p2"}},
			},
			expectedNames: []string{"p1", "p2"},
		},
	}

	for _, tc := range testCases {
		got := getObjectListNames(tc.listObj)
		if !reflect.DeepEqual(got, tc.expectedNames) {
			t.Fatalf("expected %v, got %v", tc.expectedNames, got)
		}
	}
}

func TestGenerateENIQoSGroup(t *testing.T) {
	reservation := &schedulingv1alpha1.Reservation{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				schedulingv1alpha1.AnnotationVPCQoSThreshold: `{"rx":"20Gi", "rxPps": "2Gi", "tx": "10Gi", "txPps": "1Gi"}`,
			},
			Labels: map[string]string{
				labelOwnedByLRN: "lrn-0",
			},
			Name: "lrn-0-gen-0",
			UID:  "uid-123",
		},
		Status: schedulingv1alpha1.ReservationStatus{
			NodeName: "fake-node-1",
		},
	}
	expectedQoSGroup := &terwayapis.ENIQosGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceSystem,
			Name:      "lrn-0-gen-0",
			Labels: map[string]string{
				labelOwnedByLRN: "lrn-0",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         schedulingv1alpha1.GroupVersion.String(),
				Kind:               "Reservation",
				Name:               "lrn-0-gen-0",
				UID:                "uid-123",
				Controller:         utilpointer.Bool(true),
				BlockOwnerDeletion: utilpointer.Bool(true),
			}},
		},
		Spec: terwayapis.ENIQosGroupSpec{
			NodeName: reservation.Status.NodeName,
			Bandwidth: terwayapis.ENIQosGroupBandwidth{
				Tx: resource.MustParse("10Gi"),
				Rx: resource.MustParse("20Gi"),
			},
			PPS: terwayapis.ENIQosGroupPPS{
				Tx: resource.MustParse("1Gi"),
				Rx: resource.MustParse("2Gi"),
			},
		},
	}

	gotGroup, err := generateENIQoSGroup(reservation)
	if err != nil {
		t.Fatal(err)
	}
	if !apiequality.Semantic.DeepEqual(gotGroup, expectedQoSGroup) {
		t.Fatalf("expected qos group:\n%s\ngot:\n%s", util.DumpJSON(expectedQoSGroup), util.DumpJSON(gotGroup))
	}
}

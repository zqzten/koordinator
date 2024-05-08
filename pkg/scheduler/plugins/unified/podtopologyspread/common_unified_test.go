/*
Copyright 2022 The Koordinator Authors.

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

package podtopologyspread

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	dummyworkloadapi "gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/api/acs/v1alpha1"
	dummyworkloadclientfake "gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/pkg/client/clientset/versioned/fake"
	dummyworkloadinformers "gitlab.alibaba-inc.com/serverlessinfra/dummy-workload/pkg/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/utils/pointer"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	koordfeatures "github.com/koordinator-sh/koordinator/pkg/features"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

func TestPlugin_buildDefaultConstraintsWithACSDefaultSpread(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, k8sfeature.DefaultMutableFeatureGate, koordfeatures.EnableACSDefaultSpread, true)()

	tests := []struct {
		name               string
		defaultConstraints []corev1.TopologySpreadConstraint
		action             corev1.UnsatisfiableConstraintAction
		ownerReference     *metav1.OwnerReference
		dummyWorkload      *dummyworkloadapi.DummyWorkload
		want               []topologySpreadConstraint
		wantErr            bool
	}{
		{
			name:               "no default constraints",
			defaultConstraints: nil,
			action:             corev1.ScheduleAnyway,
			ownerReference:     nil,
			want:               nil,
			wantErr:            false,
		},
		{
			name: "has default constraint, same action and has matched dummyWorkload",
			defaultConstraints: []corev1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       corev1.LabelTopologyZone,
					WhenUnsatisfiable: corev1.ScheduleAnyway,
				},
			},
			action:         corev1.ScheduleAnyway,
			ownerReference: &metav1.OwnerReference{Controller: pointer.Bool(true), Kind: "ReplicaSet", Name: "test-rs"},
			dummyWorkload: &dummyworkloadapi.DummyWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs-replicaset",
					Namespace: "default",
				},
				Status: dummyworkloadapi.DummyWorkloadStatus{
					LabelSelector: "app=test",
				},
			},
			want: []topologySpreadConstraint{
				{
					MaxSkew:     1,
					TopologyKey: corev1.LabelTopologyZone,
					Selector: func() labels.Selector {
						selector, err := labels.Parse("app=test")
						assert.NoError(t, err)
						return selector
					}(),
				},
			},
			wantErr: false,
		},
		{
			name: "has default constraint, different action and has matched dummyWorkload",
			defaultConstraints: []corev1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       corev1.LabelTopologyZone,
					WhenUnsatisfiable: corev1.ScheduleAnyway,
				},
			},
			action:         corev1.DoNotSchedule,
			ownerReference: &metav1.OwnerReference{Controller: pointer.Bool(true), Kind: "ReplicaSet", Name: "test-rs"},
			dummyWorkload: &dummyworkloadapi.DummyWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rs-replicaset",
					Namespace: "default",
				},
				Status: dummyworkloadapi.DummyWorkloadStatus{
					LabelSelector: "app=test",
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "has default constraint, same action and has no matched dummyWorkload",
			defaultConstraints: []corev1.TopologySpreadConstraint{
				{
					MaxSkew:           1,
					TopologyKey:       corev1.LabelTopologyZone,
					WhenUnsatisfiable: corev1.ScheduleAnyway,
				},
			},
			action:         corev1.DoNotSchedule,
			ownerReference: &metav1.OwnerReference{Controller: pointer.Bool(true), Kind: "ReplicaSet", Name: "test-rs"},
			dummyWorkload: &dummyworkloadapi.DummyWorkload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-test-rs-replicaset",
					Namespace: "default",
				},
				Status: dummyworkloadapi.DummyWorkloadStatus{
					LabelSelector: "app=test",
				},
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{}
			if tt.dummyWorkload != nil {
				objects = append(objects, tt.dummyWorkload)
			}
			client := dummyworkloadclientfake.NewSimpleClientset(objects...)
			informerFactory := dummyworkloadinformers.NewSharedInformerFactory(client, 0)
			dummyWorkloadLister := informerFactory.Acs().V1alpha1().DummyWorkloads().Lister()
			informerFactory.Start(nil)
			informerFactory.WaitForCacheSync(nil)
			pl := &PodTopologySpread{
				dummyWorkloadLister: dummyWorkloadLister,
				defaultConstraints:  tt.defaultConstraints,
			}
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Annotations: map[string]string{},
					Labels: map[string]string{
						"app": "test",
					},
				},
			}
			if tt.ownerReference != nil {
				data, err := json.Marshal([]*metav1.OwnerReference{tt.ownerReference})
				assert.NoError(t, err)
				pod.Annotations[extunified.AnnotationTenancyOwnerReferences] = string(data)
			}
			got, err := pl.buildDefaultConstraints(pod, tt.action)
			if (err != nil) != tt.wantErr {
				t.Log("unexpect error", err, tt.wantErr)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

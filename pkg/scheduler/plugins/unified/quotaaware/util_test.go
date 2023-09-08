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

package quotaaware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedfake "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned/fake"
	schedinformer "sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
)

func Test_newNodeAffinity(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		want    *nodeAffinity
		wantErr bool
	}{
		{
			name:    "missing any acs apis",
			pod:     &corev1.Pod{},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing acs quota id",
			pod:     st.MakePod().Label(LabelUserAccountId, "123456").Obj(),
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing acs pod type",
			pod:     st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Obj(),
			want:    nil,
			wantErr: true,
		},
		{
			name: "target pod",
			pod:  st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelPodType, "aaa").Obj(),
			want: &nodeAffinity{
				userID:         "123456",
				quotaID:        "666",
				podType:        "aaa",
				affinityZones:  sets.NewString(),
				affinityArches: sets.NewString("amd64"),
			},
			wantErr: false,
		},
		{
			name: "target pod with az selector",
			pod: st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelPodType, "aaa").
				NodeSelector(map[string]string{corev1.LabelTopologyZone: "az-1"}).Obj(),
			want: &nodeAffinity{
				userID:         "123456",
				quotaID:        "666",
				podType:        "aaa",
				affinityZones:  sets.NewString("az-1"),
				affinityArches: sets.NewString("amd64"),
			},
			wantErr: false,
		},
		{
			name: "target pod with az affinity",
			pod: st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelPodType, "aaa").
				NodeAffinityIn(corev1.LabelTopologyZone, []string{"az-1", "az-2"}).Obj(),
			want: &nodeAffinity{
				userID:         "123456",
				quotaID:        "666",
				podType:        "aaa",
				affinityZones:  sets.NewString("az-1", "az-2"),
				affinityArches: sets.NewString("amd64"),
			},
			wantErr: false,
		},
		{
			name: "target pod with arch affinity",
			pod: st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelPodType, "aaa").
				NodeAffinityIn(corev1.LabelArchStable, []string{"arm64"}).Obj(),
			want: &nodeAffinity{
				userID:         "123456",
				quotaID:        "666",
				podType:        "aaa",
				affinityZones:  sets.NewString(),
				affinityArches: sets.NewString("arm64"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newNodeAffinity(tt.pod)
			if tt.wantErr != (err != nil) {
				t.Errorf("newNodeAffinity() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.want, got)
		})
	}

}

func Test_nodAffinity_matchElasticQuotas(t *testing.T) {
	affinity := &nodeAffinity{
		userID:         "123",
		quotaID:        "666",
		podType:        "aaa",
		affinityZones:  sets.NewString("az-1"),
		affinityArches: sets.NewString("amd64"),
	}
	tests := []struct {
		name         string
		nodeAffinity *nodeAffinity
		quotas       []*schedv1alpha1.ElasticQuota
		want         []*schedv1alpha1.ElasticQuota
		wantErr      bool
	}{
		{
			name:         "no quotas",
			nodeAffinity: affinity,
			want:         nil,
			wantErr:      false,
		},
		{
			name:         "only has root quota",
			nodeAffinity: affinity,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "root-quota-a",
						Labels: map[string]string{
							apiext.LabelQuotaIsParent: "true",
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:         "only has zone root quota",
			nodeAffinity: affinity,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "root-quota-a",
						Labels: map[string]string{
							corev1.LabelTopologyZone:  "az-1",
							corev1.LabelArchStable:    "amd64",
							apiext.LabelQuotaIsParent: "true",
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:         "only has second-level quota",
			nodeAffinity: affinity,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "second-root-quota-a",
						Labels: map[string]string{
							corev1.LabelTopologyZone:  "az-1",
							corev1.LabelArchStable:    "amd64",
							apiext.LabelQuotaIsParent: "true",
							apiext.LabelQuotaParent:   "root-quota-a",
							LabelQuotaID:              "666",
							LabelUserAccountId:        "123",
							LabelPodType:              "aaa",
						},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:         "has leaf quotas",
			nodeAffinity: affinity,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "second-root-quota-a",
						Labels: map[string]string{
							corev1.LabelTopologyZone:  "az-1",
							corev1.LabelArchStable:    "amd64",
							apiext.LabelQuotaIsParent: "true",
							apiext.LabelQuotaParent:   "root-quota-a",
							LabelQuotaID:              "666",
							LabelUserAccountId:        "123",
							LabelPodType:              "aaa",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-a",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-1",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "second-root-quota-a",
							LabelQuotaID:             "666",
							LabelUserAccountId:       "123",
							LabelPodType:             "aaa",
						},
					},
				},
			},
			want: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "quota-a",
						Namespace: "default",
						Labels: map[string]string{
							corev1.LabelTopologyZone: "az-1",
							corev1.LabelArchStable:   "amd64",
							apiext.LabelQuotaParent:  "second-root-quota-a",
							LabelQuotaID:             "666",
							LabelUserAccountId:       "123",
							LabelPodType:             "aaa",
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := schedfake.NewSimpleClientset()
			for _, v := range tt.quotas {
				v.Namespace = "default"
				_, err := client.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), v, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			informerFactory := schedinformer.NewSharedInformerFactory(client, 0)
			elasticQuotaLister := informerFactory.Scheduling().V1alpha1().ElasticQuotas().Lister()
			informerFactory.Start(nil)
			informerFactory.WaitForCacheSync(nil)
			got, err := tt.nodeAffinity.matchElasticQuotas(elasticQuotaLister)
			if tt.wantErr != (err != nil) {
				t.Errorf("matchElasticQuotas() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_filterAvailableQuotas(t *testing.T) {
	defaultRequests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	tests := []struct {
		name     string
		requests corev1.ResourceList
		quotas   []*schedv1alpha1.ElasticQuota
		frozen   sets.String
		want1    []*QuotaObject
		want2    []*QuotaObject
	}{
		{
			name:     "no quotas",
			requests: defaultRequests,
			want1:    nil,
			want2:    nil,
		},
		{
			name:     "no satisfied quotas",
			requests: defaultRequests,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-a",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			want1: nil,
			want2: nil,
		},
		{
			name:     "has one satisfied quotas",
			requests: defaultRequests,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-a",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-b",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			want1: []*QuotaObject{
				{
					name: "quota-b",
					quotaObj: &schedv1alpha1.ElasticQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name: "quota-b",
						},
						Spec: schedv1alpha1.ElasticQuotaSpec{
							Max: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("4"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
					min: nil,
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					runtime: nil,

					pods: sets.NewString(),
					used: nil,
				},
			},
			want2: nil,
		},
		{
			name:     "has one satisfied and frozen quotas",
			requests: defaultRequests,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-a",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-b",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-c",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			frozen: sets.NewString("quota-c"),
			want1: []*QuotaObject{
				{
					name: "quota-b",
					quotaObj: &schedv1alpha1.ElasticQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name: "quota-b",
						},
						Spec: schedv1alpha1.ElasticQuotaSpec{
							Max: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("4"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
					min: nil,
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					runtime: nil,

					pods: sets.NewString(),
					used: nil,
				},
			},
			want2: []*QuotaObject{
				{
					name: "quota-c",
					quotaObj: &schedv1alpha1.ElasticQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name: "quota-c",
						},
						Spec: schedv1alpha1.ElasticQuotaSpec{
							Max: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("4"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
					min: nil,
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					runtime: nil,

					pods: sets.NewString(),
					used: nil,
				},
			},
		},
		{
			name:     "has one unsatisfied and frozen quotas",
			requests: defaultRequests,
			quotas: []*schedv1alpha1.ElasticQuota{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-a",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "quota-b",
					},
					Spec: schedv1alpha1.ElasticQuotaSpec{
						Max: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("4"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
			frozen: sets.NewString("quota-a"),
			want1: []*QuotaObject{
				{
					name: "quota-b",
					quotaObj: &schedv1alpha1.ElasticQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name: "quota-b",
						},
						Spec: schedv1alpha1.ElasticQuotaSpec{
							Max: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("4"),
								corev1.ResourceMemory: resource.MustParse("4Gi"),
							},
						},
					},
					min: nil,
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					runtime: nil,

					pods: sets.NewString(),
					used: nil,
				},
			},
			want2: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newQuotaCache()
			for _, v := range tt.quotas {
				cache.updateQuota(nil, v)
			}
			got1, got2 := filterAvailableQuotas(tt.requests, cache, tt.quotas, tt.frozen)
			assert.Equal(t, tt.want1, got1)
			assert.Equal(t, tt.want2, got2)
		})
	}
}

func Test_filterReplicasSufficientQuotas(t *testing.T) {
	defaultRequests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	tests := []struct {
		name                   string
		requests               corev1.ResourceList
		quotas                 []*QuotaObject
		preferredQuotaName     string
		byMin                  bool
		minReplicas            int
		ascending              bool
		wantSufficientQuotas   []*QuotaObject
		wantInsufficientQuotas []*QuotaObject
	}{
		{
			name:     "filter and sort by min",
			requests: defaultRequests,
			quotas: []*QuotaObject{
				{
					name: "quota-a",
					min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
				{
					name: "quota-b",
					min:  defaultRequests,
					max:  defaultRequests,
				},
			},
			byMin:       true,
			minReplicas: 100,
			ascending:   true,
			wantSufficientQuotas: []*QuotaObject{
				{
					name: "quota-b",
					min:  defaultRequests,
					max:  defaultRequests,
				},
				{
					name: "quota-a",
					min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
		},
		{
			name:     "filter min insufficient quotas",
			requests: defaultRequests,
			quotas: []*QuotaObject{
				{
					name: "quota-a",
					min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
				{
					name: "quota-b",
					min:  defaultRequests,
					max:  defaultRequests,
				},
			},
			byMin:       true,
			minReplicas: 100,
			ascending:   true,
			wantSufficientQuotas: []*QuotaObject{
				{
					name: "quota-b",
					min:  defaultRequests,
					max:  defaultRequests,
				},
			},
			wantInsufficientQuotas: []*QuotaObject{
				{
					name: "quota-a",
					min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
		},
		{
			name:     "filter min insufficient but max sufficient quotas",
			requests: defaultRequests,
			quotas: []*QuotaObject{
				{
					name: "quota-a",
					min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
				{
					name: "quota-b",
					min:  defaultRequests,
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			},
			byMin:       false,
			minReplicas: 100,
			ascending:   false,
			wantSufficientQuotas: []*QuotaObject{
				{
					name: "quota-b",
					min:  defaultRequests,
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("16"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				{
					name: "quota-a",
					min: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					max: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSufficientQuotas, gotInsufficientQuotas := filterReplicasSufficientQuotas(tt.requests, tt.quotas, tt.preferredQuotaName, tt.byMin, tt.minReplicas, tt.ascending)
			assert.Equalf(t, tt.wantSufficientQuotas, gotSufficientQuotas, "filterReplicasSufficientQuotas(%v, %v, %v, %v, %v, %v)", tt.requests, tt.quotas, tt.preferredQuotaName, tt.byMin, tt.minReplicas, tt.ascending)
			assert.Equalf(t, tt.wantInsufficientQuotas, gotInsufficientQuotas, "filterReplicasSufficientQuotas(%v, %v, %v, %v, %v, %v)", tt.requests, tt.quotas, tt.preferredQuotaName, tt.byMin, tt.minReplicas, tt.ascending)
		})
	}
}

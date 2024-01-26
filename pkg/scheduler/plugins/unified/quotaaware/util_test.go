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
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	st "k8s.io/kubernetes/pkg/scheduler/testing"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedfake "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned/fake"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"
	schedinformer "sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	elasticquotacore "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/elasticquota/core"
	nodeaffinityhelper "github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/nodeaffinity"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
)

func Test_newNodeAffinity(t *testing.T) {
	tests := []struct {
		name    string
		pod     *corev1.Pod
		want    *quotaAffinity
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
			pod:     st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelInstanceType, "666").Obj(),
			want:    nil,
			wantErr: true,
		},
		{
			name: "target pod",
			pod:  st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelInstanceType, "aaa").Obj(),
			want: &quotaAffinity{
				userID:         "123456",
				quotaID:        "666",
				instanceType:   "aaa",
				affinityZones:  sets.NewString(),
				affinityArches: sets.NewString("amd64"),
			},
			wantErr: false,
		},
		{
			name: "target pod with az selector",
			pod: st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelInstanceType, "aaa").
				NodeSelector(map[string]string{corev1.LabelTopologyZone: "az-1"}).Obj(),
			want: &quotaAffinity{
				userID:         "123456",
				quotaID:        "666",
				instanceType:   "aaa",
				affinityZones:  sets.NewString("az-1"),
				affinityArches: sets.NewString("amd64"),
			},
			wantErr: false,
		},
		{
			name: "target pod with az affinity",
			pod: st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelInstanceType, "aaa").
				NodeAffinityIn(corev1.LabelTopologyZone, []string{"az-1", "az-2"}).Obj(),
			want: &quotaAffinity{
				userID:         "123456",
				quotaID:        "666",
				instanceType:   "aaa",
				affinityZones:  sets.NewString("az-1", "az-2"),
				affinityArches: sets.NewString("amd64"),
			},
			wantErr: false,
		},
		{
			name: "target pod with arch affinity",
			pod: st.MakePod().Label(LabelUserAccountId, "123456").Label(LabelQuotaID, "666").Label(LabelInstanceType, "aaa").
				NodeAffinityIn(corev1.LabelArchStable, []string{"arm64"}).Obj(),
			want: &quotaAffinity{
				userID:         "123456",
				quotaID:        "666",
				instanceType:   "aaa",
				affinityZones:  sets.NewString(),
				affinityArches: sets.NewString("arm64"),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newQuotaAffinity(tt.pod)
			if tt.wantErr != (err != nil) {
				t.Errorf("newQuotaAffinity() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.want, got)
		})
	}

}

func Test_nodAffinity_matchElasticQuotas(t *testing.T) {
	affinity := &quotaAffinity{
		userID:         "123",
		quotaID:        "666",
		instanceType:   "aaa",
		affinityZones:  sets.NewString("az-1"),
		affinityArches: sets.NewString("amd64"),
	}
	tests := []struct {
		name         string
		nodeAffinity *quotaAffinity
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
							LabelInstanceType:         "aaa",
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
							LabelInstanceType:         "aaa",
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
							LabelInstanceType:        "aaa",
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
							LabelInstanceType:        "aaa",
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
			got, err := tt.nodeAffinity.matchElasticQuotas(elasticQuotaLister, true)
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
		name       string
		requests   corev1.ResourceList
		quotas     []*schedv1alpha1.ElasticQuota
		want       sets.String
		wantStatus *framework.Status
	}{
		{
			name:       "no quotas",
			requests:   defaultRequests,
			want:       sets.NewString(),
			wantStatus: framework.NewStatus(framework.UnschedulableAndUnresolvable, "No available quotas"),
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
			want:       sets.NewString(),
			wantStatus: framework.NewStatus(framework.Unschedulable, `Insufficient Quotas "quota-a", cpu capacity 1, allocated: 0; memory capacity 1Gi, allocated: 0`),
		},
		{
			name:     "has satisfied quotas",
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
			want: sets.NewString("quota-b", "quota-c"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil)
			for _, v := range tt.quotas {
				v.Namespace = "default"
				_, err := suit.schedclient.SchedulingV1alpha1().ElasticQuotas(v.Namespace).Create(context.TODO(), v, metav1.CreateOptions{})
				assert.NoError(t, err)
			}
			p, err := suit.proxyNew(nil, suit.Framework)
			assert.NoError(t, err)
			pl := p.(*Plugin)
			got, status := filterGuaranteeAvailableQuotas(&corev1.Pod{}, tt.requests, pl.Plugin, tt.quotas)
			assert.Equal(t, tt.wantStatus, status)
			t.Log(status.Message())
			gotQuotaNames := sets.NewString()
			for _, v := range got {
				gotQuotaNames.Insert(v.Name)
			}
			assert.Equal(t, tt.want, gotQuotaNames)
		})
	}
}

func Test_checkGuarantee_different_requests_and_used(t *testing.T) {
	defaultSystemQuotaGroupMax := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(math.MaxInt64/5, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(math.MaxInt64/5, resource.BinarySI),
	}
	qm := elasticquotacore.NewGroupQuotaManager("", defaultSystemQuotaGroupMax, defaultSystemQuotaGroupMax)
	err := qm.UpdateQuota(&schedv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: "quota-1",
			Labels: map[string]string{
				apiext.LabelQuotaIsParent: "true",
			},
		},
		Spec: schedv1alpha1.ElasticQuotaSpec{
			Max: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10"),
				corev1.ResourceMemory: resource.MustParse("10Gi"),
				"fakeResource":        resource.MustParse("10"),
			},
			Min: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10"),
				corev1.ResourceMemory: resource.MustParse("10Gi"),
				"fakeResource":        resource.MustParse("10"),
			},
		},
	}, false)
	assert.NoError(t, err)

	err = qm.UpdateQuota(&schedv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-quota",
			Labels: map[string]string{
				apiext.LabelQuotaIsParent: "false",
				apiext.LabelQuotaParent:   "quota-1",
			},
		},
		Spec: schedv1alpha1.ElasticQuotaSpec{
			Max: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10"),
				corev1.ResourceMemory: resource.MustParse("10Gi"),
				"fakeResource":        resource.MustParse("10"),
			},
		},
	}, false)
	assert.NoError(t, err)

	quotaInfo := qm.GetQuotaInfoByName("test-quota")
	quotaInfo.CalculateInfo = elasticquotacore.QuotaCalculateInfo{
		Max: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("10"),
			corev1.ResourceMemory: resource.MustParse("10Gi"),
			"fakeResource":        resource.MustParse("10"),
		},
		Used: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			"fakeResource":        resource.MustParse("12"),
		},
		Allocated: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			"fakeResource":        resource.MustParse("12"),
		},
		Guaranteed: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
			"fakeResource":        resource.MustParse("12"),
		},
	}

	qm.SetTotalResourceForTree(corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("100"),
		corev1.ResourceMemory: resource.MustParse("100Gi"),
		"fakeResource":        resource.MustParse("100"),
	})

	requests := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("1"),
		corev1.ResourceMemory: resource.MustParse("1Gi"),
	}
	passed, _, _, status := checkGuarantee(qm, quotaInfo, requests, quotav1.ResourceNames(requests))
	assert.True(t, passed)
	assert.True(t, status.IsSuccess())
}

func Test_addTemporaryNodeAffinity(t *testing.T) {
	cycleState := framework.NewCycleState()
	addTemporaryNodeAffinity(cycleState, []*schedv1alpha1.ElasticQuota{
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-1",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					corev1.LabelTopologyZone: "az-2",
				},
			},
		},
	})
	affinity := nodeaffinityhelper.GetTemporaryNodeAffinity(cycleState)
	assert.True(t, affinity.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-1",
			},
		},
	}))
	assert.True(t, affinity.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-2",
			},
		},
	}))
	assert.False(t, affinity.Match(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-3",
			},
		},
	}))
}

func Test_generateAffinityConflictMessage(t *testing.T) {
	quota := &schedv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "quota-a",
			Namespace: "default",
			Labels: map[string]string{
				corev1.LabelTopologyZone: "az-1",
				corev1.LabelArchStable:   "amd64",
				apiext.LabelQuotaParent:  "",
				LabelQuotaID:             "666",
				LabelUserAccountId:       "123",
				LabelInstanceType:        "aaa",
			},
		},
		Spec: schedv1alpha1.ElasticQuotaSpec{
			Max: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("40"),
				corev1.ResourceMemory: resource.MustParse("80Gi"),
			},
			Min: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}

	schedclient := schedfake.NewSimpleClientset()
	_, err := schedclient.SchedulingV1alpha1().ElasticQuotas("default").Create(context.TODO(), quota, metav1.CreateOptions{})
	assert.NoError(t, err)

	scheSharedInformerFactory := externalversions.NewSharedInformerFactory(schedclient, 0)
	transformer.SetupElasticQuotaTransformers(scheSharedInformerFactory)
	elasticQuotaInformer := scheSharedInformerFactory.Scheduling().V1alpha1().ElasticQuotas()
	lister := elasticQuotaInformer.Lister()
	scheSharedInformerFactory.Start(nil)
	scheSharedInformerFactory.WaitForCacheSync(nil)

	tests := []struct {
		name         string
		affinityZone string
		userID       string
		want         string
	}{
		{
			name:         "conflict affinity",
			affinityZone: "az-b",
			want:         "Available ElasticQuotas can't be used because of NodeAffinity issues. Check the Pod's NodeAffinity or the cluster's vSwitch settings.",
		},
		{
			name: "no zones",
			want: "",
		},
		{
			name:         "missing account id but affinity available zone",
			affinityZone: "az-1",
			userID:       "non-exist-user-id",
			want:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affinity := &quotaAffinity{
				userID:         "123",
				quotaID:        "666",
				instanceType:   "aaa",
				affinityZones:  sets.NewString(),
				affinityArches: sets.NewString("amd64"),
			}
			if tt.affinityZone != "" {
				affinity.affinityZones.Insert(tt.affinityZone)
			}
			if tt.userID != "" {
				affinity.userID = tt.userID
			}
			message := generateAffinityConflictMessage(affinity, lister)
			assert.Equal(t, tt.want, message)
		})
	}

}

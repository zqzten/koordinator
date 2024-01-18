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

package elasticquota

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
)

func (r *resourceWrapper) BatchCPU(val int64) *resourceWrapper {
	r.ResourceList[extension.BatchCPU] = *resource.NewQuantity(val, resource.DecimalSI)
	return r
}

func (r *resourceWrapper) BatchMemory(val int64) *resourceWrapper {
	r.ResourceList[extension.BatchMemory] = *resource.NewQuantity(val, resource.DecimalSI)
	return r
}

func (p *podWrapper) NodeName(nodeName string) *podWrapper {
	p.Pod.Spec.NodeName = nodeName
	return p
}

func (e *eqWrapper) Label(key, val string) *eqWrapper {
	if e.ElasticQuota.Labels == nil {
		e.ElasticQuota.Labels = make(map[string]string)
	}
	e.ElasticQuota.Labels[key] = val
	return e
}

func TestController_RunBatch(t *testing.T) {
	ctx := context.TODO()
	elasticQuota := MakeEQ("t1-ns1-x", "t1-ns1-x").Min(MakeResourceList().CPU(3).Mem(5).Obj()).
		Max(MakeResourceList().CPU(5).Mem(15).Obj()).Label(transformer.LabelInstanceType, transformer.BestEffortInstanceType).Obj()
	pods := []*v1.Pod{
		MakePod("t1-ns1-x", "pod1").Phase(v1.PodRunning).NodeName("node1").Container(
			MakeResourceList().BatchCPU(1000).BatchMemory(2).Obj()).UID("pod1").Label(extension.LabelQuotaName, "t1-ns1-x").Obj(),
		MakePod("t1-ns1-x", "pod2").Phase(v1.PodPending).Container(
			MakeResourceList().BatchCPU(1000).BatchMemory(2).Obj()).UID("pod2").Label(extension.LabelQuotaName, "t1-ns1-x").Obj(),
	}

	suit := newPluginTestSuitWithPod(t, nil, nil)
	suit.client.SchedulingV1alpha1().ElasticQuotas(elasticQuota.Namespace).Create(ctx, elasticQuota, metav1.CreateOptions{})

	for _, p := range pods {
		suit.Handle.ClientSet().CoreV1().Pods(p.Namespace).Create(ctx, p, metav1.CreateOptions{})
	}
	plugin, err := suit.proxyNew(suit.elasticQuotaArgs, suit.Handle)
	assert.Nil(t, err)
	p := plugin.(*Plugin)
	ctrl := NewElasticQuotaController(p)
	ctrl.syncElasticQuotaStatusWorker()

	get, _ := suit.client.SchedulingV1alpha1().ElasticQuotas(elasticQuota.Namespace).Get(ctx, elasticQuota.Name, metav1.GetOptions{})
	assert.NotNil(t, get)
	request, innerErr := extension.GetRequest(get)
	assert.NoError(t, innerErr)

	wantUsed := MakeResourceList().CPU(1).Mem(2).Obj()
	wantRequests := MakeResourceList().CPU(2).Mem(4).Obj()
	if !quotav1.Equals(request, wantRequests) || !quotav1.Equals(get.Status.Used, wantUsed) {
		err = fmt.Errorf("want used: %v, request: %v, got used: %v, request: %v,quotaName: %v",
			wantUsed, wantRequests, get.Status.Used, request, get.Name)
	} else {
		t.Logf("got: %v", util.DumpJSON(get))
		err = nil
	}
	if err != nil {
		t.Errorf("Elastic Quota Test Failed, err: %v", err)
	}
}

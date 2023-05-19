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

package resourceflavor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	cosvsche1beta1 "gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	cosv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/flavor"
)

func TestControllerBasic(t *testing.T) {
	scheme := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(scheme)
	assert.NoError(t, err)
	err = v1beta1.AddToScheme(scheme)
	assert.NoError(t, err)
	err = schedulingv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)
	scheme.AddKnownTypes(cosv1beta1.GroupVersion, &cosv1beta1.Device{})
	metav1.AddToGroupVersion(scheme, cosv1beta1.GroupVersion)
	scheme.AddKnownTypes(cosv1beta1.GroupVersion, &cosv1beta1.Device{})
	metav1.AddToGroupVersion(scheme, cosv1beta1.GroupVersion)

	_ = cosvsche1beta1.AddToScheme(scheme)
	scheme.AddKnownTypes(cosvsche1beta1.SchemeGroupVersion, &cosvsche1beta1.ElasticQuotaTree{}, &cosvsche1beta1.ElasticQuotaTreeList{})
	metav1.AddToGroupVersion(scheme, cosvsche1beta1.SchemeGroupVersion)

	assert.NoError(t, err)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &Reconciler{
		Client: client,
	}

	nodeCache := cache.NewNodeCache()
	quotaNodeBinderCache := cache.NewResourceFlavorCache()
	r.nodeCache = nodeCache
	r.resourceFlavorCache = quotaNodeBinderCache
	r.resourceFlavor = flavor.NewResourceFlavor(r.Client, nodeCache, quotaNodeBinderCache)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "N0",
			Labels: map[string]string{
				"AA": "BB",
			},
		},
	}
	r.Create(context.Background(), node)
	r.loadNodes()
	if len(nodeCache.GetAllNodeCr()) != 1 {
		t.Errorf("error:%v", len(nodeCache.GetAllNodeCr()))
	}

	quotaNodeBinder := &schedulingv1alpha1.ResourceFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: "B1",
		},
	}
	r.Create(context.Background(), quotaNodeBinder)
	r.loadResourceFlavor()
	if len(quotaNodeBinderCache.GetAllResourceFlavor()) != 1 {
		t.Errorf("error:%v", len(quotaNodeBinderCache.GetAllResourceFlavor()))
	}
}

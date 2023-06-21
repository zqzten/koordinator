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

package options

import (
	kruisev1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	kruisev1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	cosvsche1beta1 "gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	autoscaling "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	cosv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	univ1bata1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"

	_ "github.com/koordinator-sh/koordinator/apis/extension/ack"
	_ "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/unified"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor"
	"github.com/koordinator-sh/koordinator/pkg/controller/unified/resourcesummary"
	_ "github.com/koordinator-sh/koordinator/pkg/webhook/elasticquota/unified"
	_ "github.com/koordinator-sh/koordinator/pkg/webhook/pod/validating/unified"
)

func init() {
	_ = univ1bata1.AddToScheme(clientgoscheme.Scheme)
	clientgoscheme.Scheme.AddKnownTypes(cosv1beta1.GroupVersion, &cosv1beta1.Device{}, &cosv1beta1.DeviceList{})
	_ = univ1bata1.AddToScheme(Scheme)
	Scheme.AddKnownTypes(cosv1beta1.GroupVersion, &cosv1beta1.Device{}, &cosv1beta1.DeviceList{})

	_ = cosvsche1beta1.AddToScheme(clientgoscheme.Scheme)
	clientgoscheme.Scheme.AddKnownTypes(cosvsche1beta1.SchemeGroupVersion, &cosvsche1beta1.ElasticQuotaTree{}, &cosvsche1beta1.ElasticQuotaTreeList{})
	_ = cosvsche1beta1.AddToScheme(Scheme)
	Scheme.AddKnownTypes(cosvsche1beta1.SchemeGroupVersion, &cosvsche1beta1.ElasticQuotaTree{}, &cosvsche1beta1.ElasticQuotaTreeList{})

	_ = unified.AddToScheme(clientgoscheme.Scheme)
	_ = unified.AddToScheme(Scheme)

	controllerAddFuncs["resourcesummary"] = resourcesummary.Add
	controllerAddFuncs[resourceflavor.ControllerName] = resourceflavor.Add

	_ = clientgoscheme.AddToScheme(Scheme)
	_ = autoscaling.AddToScheme(Scheme)
	_ = kruisev1alpha1.AddToScheme(Scheme)
	_ = kruisev1beta1.AddToScheme(Scheme)
	controllerAddFuncs[RecommenderControllerName] = AddRecommender
}

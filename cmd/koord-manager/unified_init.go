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

package main

import (
	univ1bata1 "gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	_ "github.com/koordinator-sh/koordinator/apis/extension/ack"
	_ "github.com/koordinator-sh/koordinator/apis/extension/unified"
	_ "github.com/koordinator-sh/koordinator/pkg/webhook/elasticquota/unified"
	_ "github.com/koordinator-sh/koordinator/pkg/webhook/pod/validating/unified"

	"github.com/koordinator-sh/koordinator/pkg/controller/unified/resourcesummary"
)

func init() {
	_ = univ1bata1.AddToScheme(clientgoscheme.Scheme)
	_ = univ1bata1.AddToScheme(scheme)

	controllerAddFuncs["ResourceSummary"] = resourcesummary.Add
}

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

package recommendationfinder

import (
	recommendationv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
)

func (r *ControllerFinder) GetRecommendationsForRef(apiVersion, kind, name, ns string) ([]*recommendationv1alpha1.Recommendation, error) {
	obj, err := r.GetScaleAndSelectorForRef(apiVersion, kind, ns, name, "")
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	return GetRecommendationsByOwnerRef(r.recommendationInformer.Informer().GetIndexer(), ns, obj.Name, obj.Kind, obj.APIVersion)
}

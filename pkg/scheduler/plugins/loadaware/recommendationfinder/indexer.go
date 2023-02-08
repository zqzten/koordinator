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
	"errors"
	"fmt"

	recommendationv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	RecommendationOwnerRefIndex = "recommendation.ownerRef"
)

var RecommendationIndexers = map[string]cache.IndexFunc{
	RecommendationOwnerRefIndex: func(obj interface{}) ([]string, error) {
		recommendation, ok := obj.(*recommendationv1alpha1.Recommendation)
		if !ok {
			return nil, errors.New("not a Recommendation")
		}
		if recommendation.Spec.WorkloadRef != nil {
			workloadRef := recommendation.Spec.WorkloadRef
			return []string{fmt.Sprintf("%s/%s/%s/%s", recommendation.Namespace, workloadRef.Name, workloadRef.Kind, workloadRef.APIVersion)}, nil
		}
		return []string{}, nil
	},
}

func GetRecommendationsByOwnerRef(indexer cache.Indexer, namespace, name, kind, apiVersion string) ([]*recommendationv1alpha1.Recommendation, error) {
	items, err := indexer.Index(RecommendationOwnerRefIndex, &recommendationv1alpha1.Recommendation{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: recommendationv1alpha1.RecommendationSpec{
			WorkloadRef: &recommendationv1alpha1.CrossVersionObjectReference{
				Name:       name,
				Kind:       kind,
				APIVersion: apiVersion,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []*recommendationv1alpha1.Recommendation{}, nil
	}
	recommendations := make([]*recommendationv1alpha1.Recommendation, 0, len(items))
	for _, v := range items {
		recommendations = append(recommendations, v.(*recommendationv1alpha1.Recommendation))
	}
	return recommendations, nil
}

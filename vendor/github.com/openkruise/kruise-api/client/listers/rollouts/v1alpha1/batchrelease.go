/*
Copyright 2022 The Kruise Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/openkruise/kruise-api/rollouts/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// BatchReleaseLister helps list BatchReleases.
// All objects returned here must be treated as read-only.
type BatchReleaseLister interface {
	// List lists all BatchReleases in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.BatchRelease, err error)
	// BatchReleases returns an object that can list and get BatchReleases.
	BatchReleases(namespace string) BatchReleaseNamespaceLister
	BatchReleaseListerExpansion
}

// batchReleaseLister implements the BatchReleaseLister interface.
type batchReleaseLister struct {
	indexer cache.Indexer
}

// NewBatchReleaseLister returns a new BatchReleaseLister.
func NewBatchReleaseLister(indexer cache.Indexer) BatchReleaseLister {
	return &batchReleaseLister{indexer: indexer}
}

// List lists all BatchReleases in the indexer.
func (s *batchReleaseLister) List(selector labels.Selector) (ret []*v1alpha1.BatchRelease, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.BatchRelease))
	})
	return ret, err
}

// BatchReleases returns an object that can list and get BatchReleases.
func (s *batchReleaseLister) BatchReleases(namespace string) BatchReleaseNamespaceLister {
	return batchReleaseNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// BatchReleaseNamespaceLister helps list and get BatchReleases.
// All objects returned here must be treated as read-only.
type BatchReleaseNamespaceLister interface {
	// List lists all BatchReleases in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.BatchRelease, err error)
	// Get retrieves the BatchRelease from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.BatchRelease, error)
	BatchReleaseNamespaceListerExpansion
}

// batchReleaseNamespaceLister implements the BatchReleaseNamespaceLister
// interface.
type batchReleaseNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all BatchReleases in the indexer for a given namespace.
func (s batchReleaseNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.BatchRelease, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.BatchRelease))
	})
	return ret, err
}

// Get retrieves the BatchRelease from the indexer for a given namespace and name.
func (s batchReleaseNamespaceLister) Get(name string) (*v1alpha1.BatchRelease, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("batchrelease"), name)
	}
	return obj.(*v1alpha1.BatchRelease), nil
}

/*
Copyright 2021 Alibaba Cloud.

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
// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
	versioned "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	internalinterfaces "gitlab.alibaba-inc.com/cos/unified-resource-api/client/informers/externalversions/internalinterfaces"
	v1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/client/listers/resources/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// CgroupsInformer provides access to a shared informer and lister for
// Cgroupses.
type CgroupsInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.CgroupsLister
}

type cgroupsInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewCgroupsInformer constructs a new informer for Cgroups type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewCgroupsInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredCgroupsInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredCgroupsInformer constructs a new informer for Cgroups type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredCgroupsInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ResourcesV1alpha1().Cgroupses(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ResourcesV1alpha1().Cgroupses(namespace).Watch(context.TODO(), options)
			},
		},
		&resourcesv1alpha1.Cgroups{},
		resyncPeriod,
		indexers,
	)
}

func (f *cgroupsInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredCgroupsInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *cgroupsInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&resourcesv1alpha1.Cgroups{}, f.defaultInformer)
}

func (f *cgroupsInformer) Lister() v1alpha1.CgroupsLister {
	return v1alpha1.NewCgroupsLister(f.Informer().GetIndexer())
}

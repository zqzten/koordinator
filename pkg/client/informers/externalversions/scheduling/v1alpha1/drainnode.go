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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	"context"
	time "time"

	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	versioned "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned"
	internalinterfaces "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// DrainNodeInformer provides access to a shared informer and lister for
// DrainNodes.
type DrainNodeInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.DrainNodeLister
}

type drainNodeInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewDrainNodeInformer constructs a new informer for DrainNode type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewDrainNodeInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredDrainNodeInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredDrainNodeInformer constructs a new informer for DrainNode type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredDrainNodeInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SchedulingV1alpha1().DrainNodes().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SchedulingV1alpha1().DrainNodes().Watch(context.TODO(), options)
			},
		},
		&schedulingv1alpha1.DrainNode{},
		resyncPeriod,
		indexers,
	)
}

func (f *drainNodeInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredDrainNodeInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *drainNodeInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&schedulingv1alpha1.DrainNode{}, f.defaultInformer)
}

func (f *drainNodeInformer) Lister() v1alpha1.DrainNodeLister {
	return v1alpha1.NewDrainNodeLister(f.Informer().GetIndexer())
}

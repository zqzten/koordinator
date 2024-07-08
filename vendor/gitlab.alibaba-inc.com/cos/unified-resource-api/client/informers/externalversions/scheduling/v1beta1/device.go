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

package v1beta1

import (
	"context"
	time "time"

	schedulingv1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/scheduling/v1beta1"
	versioned "gitlab.alibaba-inc.com/cos/unified-resource-api/client/clientset/versioned"
	internalinterfaces "gitlab.alibaba-inc.com/cos/unified-resource-api/client/informers/externalversions/internalinterfaces"
	v1beta1 "gitlab.alibaba-inc.com/cos/unified-resource-api/client/listers/scheduling/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// DeviceInformer provides access to a shared informer and lister for
// Devices.
type DeviceInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1beta1.DeviceLister
}

type deviceInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewDeviceInformer constructs a new informer for Device type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewDeviceInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredDeviceInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredDeviceInformer constructs a new informer for Device type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredDeviceInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SchedulingV1beta1().Devices().List(context.TODO(), options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.SchedulingV1beta1().Devices().Watch(context.TODO(), options)
			},
		},
		&schedulingv1beta1.Device{},
		resyncPeriod,
		indexers,
	)
}

func (f *deviceInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredDeviceInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *deviceInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&schedulingv1beta1.Device{}, f.defaultInformer)
}

func (f *deviceInformer) Lister() v1beta1.DeviceLister {
	return v1beta1.NewDeviceLister(f.Informer().GetIndexer())
}

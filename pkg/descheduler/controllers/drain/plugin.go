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

package drain

import (
	"context"

	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/cache"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/group"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/node"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/reservation"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/options"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/framework"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

const (
	Name = "Drain"
)

type Plugin struct {
	cache cache.CacheEventHandler
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.Infof("new drain plugin")
	p := &Plugin{
		cache: cache.GetCacheEventHandler(),
	}

	manager := options.Manager
	nodeInformer, err := manager.GetCache().GetInformer(context.Background(), &v1.Node{})
	if err != nil {
		return nil, err
	}

	nodeInformer.AddEventHandler(clientcache.ResourceEventHandlerFuncs{
		AddFunc:    p.cache.OnNodeAdd,
		UpdateFunc: p.cache.OnNodeUpdate,
		DeleteFunc: p.cache.OnNodeDelete,
	})

	podInformer, err := manager.GetCache().GetInformer(context.Background(), &v1.Pod{})
	if err != nil {
		return nil, err
	}
	podInformer.AddEventHandler(clientcache.ResourceEventHandlerFuncs{
		AddFunc:    p.cache.OnPodAdd,
		UpdateFunc: p.cache.OnPodUpdate,
		DeleteFunc: p.cache.OnPodDelete,
	})

	reservationInterpreter := reservation.NewInterpreter(manager.GetClient(), manager.GetAPIReader())
	reservationInformer, err := manager.GetCache().GetInformer(context.Background(), reservationInterpreter.GetReservationType())
	if err != nil {
		return nil, err
	}
	reservationInformer.AddEventHandler(clientcache.ResourceEventHandlerFuncs{
		AddFunc:    p.cache.OnReservationAdd,
		UpdateFunc: p.cache.OnReservationUpdate,
		DeleteFunc: p.cache.OnReservationDelete,
	})

	klog.Infof("new drain controllers!")
	if _, err := group.NewDrainNodeGroupController(args, handle); err != nil {
		return nil, err
	}
	if _, err := node.NewDrainNodeController(args, handle); err != nil {
		return nil, err
	}

	klog.Infof("new drain plugin success!")
	return p, nil
}

func (p *Plugin) Name() string {
	return Name
}

func (p *Plugin) Deschedule(ctx context.Context, nodes []*v1.Node) *framework.Status {
	return nil
}

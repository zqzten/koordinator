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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/cache"
	"github.com/koordinator-sh/koordinator/pkg/controller/resourceflavor/flavor"
	"github.com/koordinator-sh/koordinator/pkg/features"
	utilfeature "github.com/koordinator-sh/koordinator/pkg/util/feature"
)

const (
	ControllerName string = "ResourceFlavor"
)

// Reconciler reconciles resourceFlavor and node object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme

	//resourceFlavor execute the concrete bind\unbind logic between quota and nodes;
	//resourceFlavor also holds nodeCache && resourceFlavorCache
	resourceFlavor *flavor.ResourceFlavor
	//store all nodes
	nodeCache *cache.NodeCache
	//store all resourceFlavor
	resourceFlavorCache *cache.ResourceFlavorCache
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=resourceflavor,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=resourceflavor/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=scheduling.alibabacloud.com,resources=resourceflavor/finalizers,verbs=update

func Add(mgr ctrl.Manager) error {
	if !utilfeature.DefaultFeatureGate.Enabled(features.EnableResourceFlavor) {
		return nil
	}
	reconciler := &Reconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		nodeCache:           cache.NewNodeCache(),
		resourceFlavorCache: cache.NewResourceFlavorCache(),
	}
	reconciler.resourceFlavor = flavor.NewResourceFlavor(
		reconciler.Client, reconciler.nodeCache, reconciler.resourceFlavorCache)

	reconciler.Start()

	return reconciler.SetupWithManager(mgr)
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return nil
}

// Start execute a background loop to add\remove nodes for each resourceFlavorCache.
// The reason we don't use Reconcile to trigger ResourceFlavor() is that, there are so many cases can be the "trigger".
// example1: node create\update\delete event, because you don't know which resourceFlavor can pick the node,
// so you have to for all resourceFlavors to have a try.
// example2: if a node update\delete or resourceFlavor update lead to the resourceFlavor abandon a node, other
// resourceFlavors may can pick the node, so you still have to for all resourceFlavors to have a try.
// from the above we can see that even a single event may lead to for all resourceFlavors, so we simply start a
// back-round loop to check all nodes and resourceFlavors relationships.
func (r *Reconciler) Start() {
	loopRunTime := time.Second * 5

	go wait.Until(func() {
		loadNodesSuccess := r.loadNodes()
		loadResourceFlavorSuccess := r.loadResourceFlavor()
		if loadNodesSuccess && loadResourceFlavorSuccess {
			r.resourceFlavor.ResourceFlavor()
		}
	}, loopRunTime, wait.NeverStop)

	klog.Infof("ResourceFlavor start, loopSecond:%v", loopRunTime)
}

// loadResourceFlavor loads all resourceFlavor and store them into resourceFlavorCache.
func (r *Reconciler) loadResourceFlavor() bool {
	resourceFlavorList := &v1alpha1.ResourceFlavorList{}
	if err := r.Client.List(context.Background(), resourceFlavorList, &client.ListOptions{}); err != nil {
		klog.Errorf("load all resourceFlavor failed:%v", err.Error())
		return false
	}

	r.resourceFlavorCache.UpdateResourceFlavors(resourceFlavorList.Items)
	return true
}

// loadNodes loads all node and store them into nodeCache.
func (r *Reconciler) loadNodes() bool {
	nodeList := &corev1.NodeList{}
	err := r.Client.List(context.Background(), nodeList, &client.ListOptions{})
	if err != nil {
		klog.Errorf("load all node failed:%v", err.Error())
		return false
	}

	r.nodeCache.UpdateNodes(nodeList.Items)
	return true
}

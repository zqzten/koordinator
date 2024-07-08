//go:build !github
// +build !github

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

package extensions

import (
	"context"
	"os"
	"time"

	kruisev1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	kruisev1beta1 "github.com/openkruise/kruise/apis/apps/v1beta1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	cgroups "github.com/koordinator-sh/koordinator/pkg/slo-controller/ackcgroups"
	sloconfig "github.com/koordinator-sh/koordinator/pkg/slo-controller/config"

	recommender "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers"
	autoscaling "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

var (
	setupLog   = ctrl.Log.WithName("setup-ack-extensions")
	restConfig *rest.Config
)

// for third-party extensions
func PrepareExtensions(cfg *rest.Config, mgr ctrl.Manager) {
	restConfig = cfg
	mgr.AddMetricsExtraHandler("/default-metrics", promhttp.Handler())
	prepareRecommender(mgr)
	prepareCgroupsController(mgr)
}

func StartExtensions(ctx context.Context, mgr ctrl.Manager) {
	startRecommenderController(mgr, ctx.Done())
	startCgroupsController(mgr, ctx.Done())
}

func prepareRecommender(mgr ctrl.Manager) {
	_ = autoscaling.AddToScheme(mgr.GetScheme())
	_ = kruisev1alpha1.AddToScheme(mgr.GetScheme())
	_ = kruisev1beta1.AddToScheme(mgr.GetScheme())
}

func prepareCgroupsController(mgr ctrl.Manager) {
	_ = resourcesv1alpha1.AddToScheme(mgr.GetScheme())
}

type recommenderRunner struct {
	// TODO: share informers with the controllerManager
	informerFactory informers.SharedInformerFactory
	recommender     recommender.ControllerManager
}

func (r *recommenderRunner) Start(ctx context.Context) error {
	klog.Infof("start informer factory for recommender")
	go r.informerFactory.Start(ctx.Done())
	waitInformerSynceds := []cache.InformerSynced{r.informerFactory.Core().V1().ConfigMaps().Informer().HasSynced,
		r.informerFactory.Core().V1().Nodes().Informer().HasSynced}
	// wait for node cache and configmap cache
	if !cache.WaitForCacheSync(wait.NeverStop, waitInformerSynceds...) {
		klog.Error("timed out waiting for resource cache to sync")
		os.Exit(1)
	}

	setupLog.Info("staring recommender")
	go r.recommender.Start(ctx.Done())

	return nil
}

func startRecommenderController(mgr ctrl.Manager, stopCh <-chan struct{}) {
	// TODO using same client with koord-manager for recommender
	failureRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(time.Duration(5)*time.Millisecond,
		time.Duration(1000)*time.Second)
	clientSet := clientset.NewForConfigOrDie(restConfig)
	informerFactory := informers.NewSharedInformerFactory(clientSet, time.Hour*24)

	recommenderOpt := recommender.Option{
		Scheme:                             mgr.GetScheme(),
		ConfigMapNamespace:                 sloconfig.ConfigNameSpace,
		ConfigMapName:                      sloconfig.SLOCtrlConfigMap,
		RecommendationProfileQPS:           int(restConfig.QPS),
		RecommendationProfileBurst:         restConfig.Burst,
		RecommendationProfileMaxConcurrent: 1,
		Manager:                            mgr,
		FailureRateLimiter:                 failureRateLimiter,
		InformerFactory:                    informerFactory,
	}
	rc := recommender.NewControllerManager(restConfig, recommenderOpt)

	runner := &recommenderRunner{
		informerFactory: informerFactory,
		recommender:     rc,
	}
	if err := mgr.Add(runner); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Recommender")
		os.Exit(1)
	}
	// controllerManager runs the recommended controllers if it is leader elected or the leader election is disabled
}

func startCgroupsController(mgr ctrl.Manager, stopCh <-chan struct{}) {
	cgroupsRecycler := cgroups.NewCgroupsInfoRecycler(mgr.GetClient())
	if err := (&cgroups.CgroupsReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Cgroups"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, controller.Options{}, cgroupsRecycler); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cgroups")
		os.Exit(1)
	}

	if err := (&cgroups.PodCgroupsReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, controller.Options{}, cgroupsRecycler); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cgroups.Pod")
		os.Exit(1)
	}

	cgroupsRecycler.Start(stopCh)
}

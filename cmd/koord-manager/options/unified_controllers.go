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

package options

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	recommender "gitlab.alibaba-inc.com/cos/recommender/pkg/controllers"

	deschedulerinformers "github.com/koordinator-sh/koordinator/pkg/descheduler/informers"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

const (
	RecommenderControllerName = "recommender"
)

var _ manager.Runnable = (*RecommenderRunner)(nil)

type RecommenderRunner struct {
	informerFactory informers.SharedInformerFactory
	recommender     recommender.ControllerManager
}

func newRecommenderRunner(mgr ctrl.Manager) *RecommenderRunner {
	restConfig := mgr.GetConfig()

	failureRateLimiter := workqueue.NewItemExponentialFailureRateLimiter(time.Duration(5)*time.Millisecond,
		time.Duration(1000)*time.Second)

	// share the common informers with the controller manager to reduce overhead
	// TODO: adopt NewSharedInformerFactory in a public pkg
	informerFactory := deschedulerinformers.NewSharedInformerFactory(mgr, time.Hour*24)

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

	return &RecommenderRunner{
		informerFactory: informerFactory,
		recommender:     rc,
	}
}

func (r *RecommenderRunner) Start(ctx context.Context) error {
	klog.Info("staring informers for the recommender")
	go r.informerFactory.Start(ctx.Done())
	waitInformerSynceds := []cache.InformerSynced{r.informerFactory.Core().V1().ConfigMaps().Informer().HasSynced,
		r.informerFactory.Core().V1().Nodes().Informer().HasSynced}
	// wait for node cache and configmap cache
	if !cache.WaitForCacheSync(wait.NeverStop, waitInformerSynceds...) {
		klog.Fatal("timed out waiting for resource cache to sync")
	}

	klog.Info("staring recommender")
	go r.recommender.Start(ctx.Done())

	return nil
}

func AddRecommender(mgr ctrl.Manager) error {
	recommenderRunner := newRecommenderRunner(mgr)
	return mgr.Add(recommenderRunner)
}

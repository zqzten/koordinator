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

package asiquotaadaptor

import (
	"context"
	"runtime"

	asiquotav1 "gitlab.alibaba-inc.com/unischeduler/api/apis/quotas/v1"
	uniexternalversions "gitlab.alibaba-inc.com/unischeduler/api/client/informers/externalversions"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	schedulerv1alpha1 "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	schedclientset "github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/generated/clientset/versioned"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"
)

type QuotaSyncController struct {
	schedClient        schedclientset.Interface
	queue              workqueue.RateLimitingInterface
	elasticQuotaLister v1alpha1.ElasticQuotaLister
	asiInformerFactory uniexternalversions.SharedInformerFactory
}

func NewQuotaSyncController(schedClient schedclientset.Interface, elasticQuotaLister v1alpha1.ElasticQuotaLister, asiInformerFactory uniexternalversions.SharedInformerFactory) *QuotaSyncController {
	rateLimiter := workqueue.DefaultControllerRateLimiter()
	return &QuotaSyncController{
		schedClient:        schedClient,
		queue:              workqueue.NewNamedRateLimitingQueue(rateLimiter, "asiQuotaSyncController"),
		elasticQuotaLister: elasticQuotaLister,
		asiInformerFactory: asiInformerFactory,
	}
}

func (c *QuotaSyncController) Name() string {
	return "asiQuotaSyncController"
}

func (c *QuotaSyncController) Start() {
	if !enableSyncASIQuota {
		return
	}
	c.setupEventHandler()
	for i := 0; i < runtime.NumCPU(); i++ {
		go c.syncWorker()
	}
}

func (c *QuotaSyncController) setupEventHandler() {
	c.asiInformerFactory.Quotas().V1().Quotas().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			asiQuota := toASIQuota(obj)
			if asiQuota == nil {
				return
			}
			c.queue.Add(asiQuota.Name)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldASIQuota := toASIQuota(oldObj)
			if oldASIQuota == nil {
				return
			}
			asiQuota := toASIQuota(newObj)
			if asiQuota == nil {
				return
			}
			c.queue.Add(asiQuota.Name)
		},
		DeleteFunc: func(obj interface{}) {
			asiQuota := toASIQuota(obj)
			if asiQuota == nil {
				return
			}
			c.queue.Add(asiQuota.Name)
		},
	})
}

func (c *QuotaSyncController) syncWorker() {
	for c.processNextWorkItem() {

	}
}

func (c *QuotaSyncController) processNextWorkItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	err := c.reconcile(item.(string))
	c.handleError(item, err)
	return true
}

func (c *QuotaSyncController) handleError(item interface{}, err error) {
	if err == nil {
		c.queue.Forget(item)
		return
	}
	klog.ErrorS(err, "Failed to reconcile ASIQuota", "asiQuota", item)
	c.queue.AddRateLimited(item)
}

func (c *QuotaSyncController) reconcile(asiQuotaName string) error {
	asiQuotaLister := c.asiInformerFactory.Quotas().V1().Quotas().Lister()
	asiQuota, err := asiQuotaLister.Get(asiQuotaName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return c.deleteElasticQuota(asiQuotaName)
		}
		return err
	}

	quota, err := c.elasticQuotaLister.ElasticQuotas(asiQuotaNamespace).Get(asiQuotaName)
	if apierrors.IsNotFound(err) {
		err := c.createElasticQuota(asiQuota)
		if err != nil {
			klog.ErrorS(err, "Failed to create ElasticQuota", "quota", asiQuotaName)
		} else {
			klog.V(4).InfoS("Successfully create ElasticQuota", "quota", asiQuotaName)
		}
		return err
	}
	if err != nil {
		klog.ErrorS(err, "Unexpected error, failed to Get ElasticQuota", "quota", asiQuotaName)
		return err
	}

	klog.V(4).InfoS("Update ElasticQuota spec", "quota", asiQuotaName)
	err = c.updateElasticQuota(asiQuota, quota)
	if err != nil {
		klog.ErrorS(err, "Failed to update ElasticQuota", "quota", asiQuotaName)
	} else {
		klog.InfoS("Successfully update ElasticQuota", "quota", asiQuotaName)
	}
	return err
}

func (c *QuotaSyncController) deleteElasticQuota(asiQuotaName string) error {
	err := c.schedClient.SchedulingV1alpha1().ElasticQuotas(asiQuotaNamespace).Delete(context.TODO(), asiQuotaName, metav1.DeleteOptions{})
	if err == nil {
		klog.V(4).InfoS("Successfully delete ElasticQuota since the ASI Quota has been deleted", "quota", klog.KRef(asiQuotaNamespace, asiQuotaName))
	} else {
		klog.V(4).ErrorS(err, "Failed to delete ElasticQuota", "quota", klog.KRef(asiQuotaNamespace, asiQuotaName))
	}
	return err
}

func (c *QuotaSyncController) createElasticQuota(asiQuota *asiquotav1.Quota) error {
	return createElasticQuota(c.schedClient, asiQuota)
}

func (c *QuotaSyncController) updateElasticQuota(asiQuota *asiquotav1.Quota, elasticQuota *schedulerv1alpha1.ElasticQuota) error {
	return updateElasticQuota(elasticQuota, c.schedClient, asiQuota)
}

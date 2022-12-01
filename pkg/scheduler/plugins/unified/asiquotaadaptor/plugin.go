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
	"flag"

	uniclientset "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned"
	uniexternalversions "gitlab.alibaba-inc.com/unischeduler/api/client/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	Name = "ASIQuotaAdaptor"
)

var asiQuotaNamespace = "asi-quota"
var enableCompatibleWithAsiQuota = false

func init() {
	flag.BoolVar(&enableCompatibleWithAsiQuota, "enable-compatible-with-asi-quota", false, "Enable "+
		"Enable compatible with ASIQuota")
	flag.StringVar(&asiQuotaNamespace, "asi-quota-namespace", "asi-quota",
		"The namespace of the elasticQuota created by ASIQuota")
}

var _ framework.PreFilterPlugin = &Plugin{}

type Plugin struct {
	quotaClient             versioned.Interface
	quotaLister             v1alpha1.ElasticQuotaLister
	asiQuotaInformerFactory uniexternalversions.SharedInformerFactory
}

func (p *Plugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod) *framework.Status {
	return framework.NewStatus(framework.Success, "")
}
func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	client, ok := handle.(uniclientset.Interface)
	if !ok {
		kubeConfig := handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		client = uniclientset.NewForConfigOrDie(kubeConfig)
	}
	asiQuotaInformerFactory := uniexternalversions.NewSharedInformerFactory(client, 0)

	quotaClient, ok := handle.(versioned.Interface)
	if !ok {
		kubeConfig := *handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		quotaClient = versioned.NewForConfigOrDie(&kubeConfig)
	}
	quotaFactory := externalversions.NewSharedInformerFactory(quotaClient, 0)
	quotaLister := quotaFactory.Scheduling().V1alpha1().ElasticQuotas().Lister()

	adaptor := &Plugin{
		quotaClient:             quotaClient,
		quotaLister:             quotaLister,
		asiQuotaInformerFactory: asiQuotaInformerFactory,
	}

	asiQuotaInformerFactory.Quotas().V1().Quotas().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    adaptor.OnQuotaAdd,
		UpdateFunc: adaptor.OnQuotaUpdate,
		DeleteFunc: adaptor.OnQuotaDelete,
	})

	if ns, err := handle.ClientSet().CoreV1().Namespaces().Get(context.TODO(), asiQuotaNamespace, metav1.GetOptions{}); err == nil {
		klog.Infof("get ASIQuota namespace success, namespace:%v", ns.Name)
		return adaptor, nil
	}
	err := util.RetryOnConflictOrTooManyRequests(
		func() error {
			_, err := handle.ClientSet().CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: asiQuotaNamespace,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				klog.V(4).ErrorS(err, "create ASIQuota namespace fail, namespace:%v", asiQuotaNamespace)
				return err
			}
			klog.Infof("create ASIQuota namespace success, namespace:%v", asiQuotaNamespace)
			return nil
		})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, err
		}
		klog.Errorf("create ASIQuota namespace fail, namespace:%v, err:%v", asiQuotaNamespace, err.Error())
	}
	ctx := context.TODO()
	quotaFactory.Start(ctx.Done())
	quotaFactory.WaitForCacheSync(ctx.Done())

	return adaptor, nil
}

func (p *Plugin) NewControllers() ([]frameworkext.Controller, error) {
	return []frameworkext.Controller{p}, nil
}

func (p *Plugin) Name() string {
	return Name
}

func (p *Plugin) Start() {
	if enableCompatibleWithAsiQuota {
		ctx := context.TODO()
		p.asiQuotaInformerFactory.Start(ctx.Done())
		p.asiQuotaInformerFactory.WaitForCacheSync(ctx.Done())
		klog.Infof("start ASIQuotaAdaptor")
	}
}

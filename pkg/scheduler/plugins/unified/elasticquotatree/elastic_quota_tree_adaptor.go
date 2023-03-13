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

package elasticquotatree

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/spf13/pflag"
	"gitlab.alibaba-inc.com/cos/scheduling-api/pkg/apis/scheduling/v1beta1"
	cosclientset "gitlab.alibaba-inc.com/cos/scheduling-api/pkg/client/clientset/versioned"
	cosexternalversions "gitlab.alibaba-inc.com/cos/scheduling-api/pkg/client/informers/externalversions"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	apiv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"
	"sigs.k8s.io/scheduler-plugins/pkg/generated/listers/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	Name = "ElasticACKQuotaTreeAdaptor"
)

var enableCompatibleWithACKElasticQuotaTree = false
var ackElasticQuotaTreeNamespace = "ack-elastic-quota-tree"

func init() {
	pflag.BoolVar(&enableCompatibleWithACKElasticQuotaTree, "compatible-with-ack-elastic-quota-tree", false,
		"Enable compatible with ACKElasticQuotaTree")
	pflag.StringVar(&ackElasticQuotaTreeNamespace, "ack-elastic-quota-tree-namespace", "ack-elastic-quota-tree",
		"The namespace of the elasticQuota created by ackElasticQuotaTree")
}

var _ framework.PreFilterPlugin = &Plugin{}

type Plugin struct {
	client                   versioned.Interface
	quotaLister              v1alpha1.ElasticQuotaLister
	quotaTreeInformerFactory cosexternalversions.SharedInformerFactory
}

func (p *Plugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) *framework.Status {
	return framework.NewStatus(framework.Success, "")
}

func (p *Plugin) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func New(args runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	treeClient, ok := handle.(cosclientset.Interface)
	if !ok {
		kubeConfig := *handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		treeClient = cosclientset.NewForConfigOrDie(&kubeConfig)
	}

	treeFactory := cosexternalversions.NewSharedInformerFactory(treeClient, 0)
	treeInformer := treeFactory.Scheduling().V1beta1().ElasticQuotaTrees()

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
		client:                   quotaClient,
		quotaLister:              quotaLister,
		quotaTreeInformerFactory: treeFactory,
	}

	treeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    adaptor.OnQuotaTreeAdd,
		UpdateFunc: adaptor.OnQuotaTreeUpdate,
		DeleteFunc: adaptor.OnQuotaTreeDelete,
	})

	if ns, err := handle.ClientSet().CoreV1().Namespaces().Get(context.TODO(), ackElasticQuotaTreeNamespace, metav1.GetOptions{}); err == nil {
		klog.Infof("get elasticQuotaTree namespace success, namespace:%v", ns.Name)
		return adaptor, nil
	}

	err := util.RetryOnConflictOrTooManyRequests(
		func() error {
			_, err := handle.ClientSet().CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: ackElasticQuotaTreeNamespace,
				},
			}, metav1.CreateOptions{})
			if err != nil {
				klog.V(4).ErrorS(err, "create elasticQuotaTree namespace fail, namespace:%v",
					ackElasticQuotaTreeNamespace)
				return err
			}
			klog.Infof("create elasticQuotaTree namespace success, namespace:%v", ackElasticQuotaTreeNamespace)
			return nil
		})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, err
		}
		klog.Errorf("create elasticQuotaTree namespace fail, namespace:%v, err:%v", ackElasticQuotaTreeNamespace, err.Error())
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
	if enableCompatibleWithACKElasticQuotaTree {
		ctx := context.TODO()
		p.quotaTreeInformerFactory.Start(ctx.Done())
		p.quotaTreeInformerFactory.WaitForCacheSync(ctx.Done())
		klog.Infof("start ACKQuotaTreeAdaptor")
	}
}

func (p *Plugin) OnQuotaTreeAdd(obj interface{}) {
	quotaTree := toElasticQuotaTree(obj)
	if quotaTree == nil {
		return
	}
	p.createOrUpdateQuotaByTraversalTree(extension.RootQuotaName, quotaTree.Spec.Root)
	p.deleteElasticQuota(quotaTree.Spec.Root)
	klog.Infof("QuotaTree %v", quotaTree)
}

func (p *Plugin) OnQuotaTreeUpdate(oldObj, newObj interface{}) {
	p.OnQuotaTreeAdd(newObj)
}

func (p *Plugin) OnQuotaTreeDelete(obj interface{}) {
	quotaTree := toElasticQuotaTree(obj)
	if quotaTree == nil {
		return
	}
	p.deleteElasticQuota(v1beta1.ElasticQuotaSpec{})
}

func (p *Plugin) createOrUpdateQuotaByTraversalTree(parent string, quotaSpec v1beta1.ElasticQuotaSpec) {
	quotaName := quotaSpec.Name
	if quotaSpec.Name != extension.RootQuotaName || len(quotaSpec.Namespaces) > 0 {
		quotaName = generateElasticQuotaName(quotaSpec.Namespaces)
		eq, err := p.client.SchedulingV1alpha1().ElasticQuotas(ackElasticQuotaTreeNamespace).Get(context.TODO(), quotaName,
			metav1.GetOptions{ResourceVersion: "0"})
		if err != nil {
			p.createElasticQuota(parent, quotaSpec)
		} else {
			p.updateElasticQuota(parent, eq, quotaSpec)
		}
	}
	for _, child := range quotaSpec.Children {
		p.createOrUpdateQuotaByTraversalTree(quotaName, child)
	}
}

func (p *Plugin) createElasticQuota(parent string, quotaSpec v1beta1.ElasticQuotaSpec) {
	quotaName := generateElasticQuotaName(quotaSpec.Namespaces)
	elasticQuota := &apiv1alpha1.ElasticQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:        quotaName,
			Namespace:   ackElasticQuotaTreeNamespace,
			Annotations: make(map[string]string),
			Labels:      make(map[string]string),
		},
		Spec: apiv1alpha1.ElasticQuotaSpec{
			Max: quotaSpec.Max,
			Min: quotaSpec.Min,
		},
	}
	parseElasticQuotaSpec(elasticQuota, quotaSpec, parent)
	err := util.RetryOnConflictOrTooManyRequests(
		func() error {
			eq, err := p.client.SchedulingV1alpha1().ElasticQuotas(ackElasticQuotaTreeNamespace).
				Create(context.TODO(), elasticQuota, metav1.CreateOptions{})
			if err != nil {
				klog.V(4).ErrorS(err, "create elastic quota fail with elasticQuotaTree, namespace:%v, name:%v",
					ackElasticQuotaTreeNamespace, elasticQuota.Name)
				return err
			}
			klog.Infof("create elastic quota success with elasticQuotaTree, namespace:%v, name:%v, quota:%v",
				ackElasticQuotaTreeNamespace, elasticQuota.Name, eq)
			return nil
		})
	if err != nil {
		klog.Errorf("create elastic quota fail with elasticQuotaTree, namespace:%v, name:%v, err:%v",
			ackElasticQuotaTreeNamespace, elasticQuota.Name, err.Error())
	}
}

func (p *Plugin) updateElasticQuota(parent string, oldQuota *apiv1alpha1.ElasticQuota, spec v1beta1.ElasticQuotaSpec) {
	newQuota := oldQuota.DeepCopy()
	newQuota.Spec.Max = spec.Max.DeepCopy()
	newQuota.Spec.Min = spec.Min.DeepCopy()
	if newQuota.Labels == nil {
		newQuota.Labels = make(map[string]string)
	}
	if newQuota.Annotations == nil {
		newQuota.Annotations = make(map[string]string)
	}
	parseElasticQuotaSpec(newQuota, spec, parent)

	oldData, _ := json.Marshal(oldQuota)
	newData, _ := json.Marshal(newQuota)
	patchBytes, _ := jsonpatch.CreateMergePatch(oldData, newData)

	err := util.RetryOnConflictOrTooManyRequests(func() error {
		eq, err := p.client.SchedulingV1alpha1().ElasticQuotas(ackElasticQuotaTreeNamespace).
			Patch(context.TODO(), newQuota.Name, apimachinerytypes.MergePatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			klog.V(4).ErrorS(err, "update elastic quota fail with elasticQuotaTree, namespace:%v, name:%v",
				ackElasticQuotaTreeNamespace, newQuota.Name)
			return err
		}
		klog.Infof("update elastic quota success with elasticQuotaTree, namespace:%v, name:%v, quota:%v",
			ackElasticQuotaTreeNamespace, newQuota.Name, eq)
		return nil
	})
	if err != nil {
		klog.Errorf("update elastic quota fail with elasticQuotaTree, namespace:%v, name:%v, err:%v",
			ackElasticQuotaTreeNamespace, newQuota.Name, err)
	}
}

func (p *Plugin) deleteElasticQuota(spec v1beta1.ElasticQuotaSpec) {
	toDeleteElasticQuotas := p.getToDeleteElasticQuotas(spec)
	for _, eq := range toDeleteElasticQuotas {
		err := util.RetryOnConflictOrTooManyRequests(
			func() error {
				err := p.client.SchedulingV1alpha1().ElasticQuotas(ackElasticQuotaTreeNamespace).
					Delete(context.TODO(), eq.Name, metav1.DeleteOptions{})
				if err != nil {
					klog.V(4).ErrorS(err, "delete elastic quota fail, namespace:%v, name:%v",
						ackElasticQuotaTreeNamespace, eq.Name)
					return err
				}
				return nil
			})
		if err != nil {
			klog.Errorf("delete elastic quota fail, namespace:%v, name:%v,err:%v", ackElasticQuotaTreeNamespace,
				eq.Name, err)
		}
	}
}

// getToDeleteElasticQuotas 先删除子组，再删除父组
// TODO 暂时只支持两层结构
func (p *Plugin) getToDeleteElasticQuotas(spec v1beta1.ElasticQuotaSpec) []*apiv1alpha1.ElasticQuota {
	toDeleteQuota := make([]*apiv1alpha1.ElasticQuota, 0)
	eqs, err := p.quotaLister.List(labels.Everything())
	if err != nil {
		return toDeleteQuota
	}
	totalQuota := make(map[string]struct{})
	getQuotaTreeTotalQuota(spec, totalQuota, "")
	for _, eq := range eqs {
		if _, exist := totalQuota[eq.Name]; !exist {
			if eq.Name != extension.SystemQuotaName && eq.Name != extension.DefaultQuotaName {
				toDeleteQuota = append(toDeleteQuota, eq)
			}
		}
	}
	sort.Slice(toDeleteQuota, func(i, j int) bool {
		return !extension.IsParentQuota(toDeleteQuota[i])
	})
	return toDeleteQuota
}

func getQuotaTreeTotalQuota(spec v1beta1.ElasticQuotaSpec, totalQuota map[string]struct{}, parent string) {
	quotaName := spec.Name
	if parent != "" {
		quotaName = generateElasticQuotaName(spec.Namespaces)
		totalQuota[quotaName] = struct{}{}
	}

	for _, child := range spec.Children {
		getQuotaTreeTotalQuota(child, totalQuota, quotaName)
	}
}

func toElasticQuotaTree(obj interface{}) *v1beta1.ElasticQuotaTree {
	if obj == nil {
		return nil
	}

	var unstructedObj *unstructured.Unstructured
	switch t := obj.(type) {
	case *v1beta1.ElasticQuotaTree:
		return obj.(*v1beta1.ElasticQuotaTree)
	case *unstructured.Unstructured:
		unstructedObj = obj.(*unstructured.Unstructured)
	case cache.DeletedFinalStateUnknown:
		var ok bool
		unstructedObj, ok = t.Obj.(*unstructured.Unstructured)
		if !ok {
			klog.Errorf("Fail to convert quota object %T to *unstructured.Unstructured", obj)
			return nil
		}
	default:
		klog.Errorf("Unable to handle quota object in %T", obj)
		return nil
	}

	quota := &v1beta1.ElasticQuotaTree{}
	err := scheme.Scheme.Convert(unstructedObj, quota, nil)
	if err != nil {
		klog.Errorf("Fail to convert unstructed object %v to Quota: %v", obj, err)
		return nil
	}
	return quota
}

func generateElasticQuotaName(namespaces []string) string {
	allNamespace := strings.Join(namespaces, ",")
	allNamespace = strings.ToLower(allNamespace)
	return allNamespace
}

func parseElasticQuotaSpec(quota *apiv1alpha1.ElasticQuota, spec v1beta1.ElasticQuotaSpec, parent string) {
	quota.Labels[extension.LabelQuotaIsParent] = "false"
	if len(spec.Children) > 0 {
		quota.Labels[extension.LabelQuotaIsParent] = "true"
	}
	quota.Labels[extension.LabelQuotaParent] = parent
}

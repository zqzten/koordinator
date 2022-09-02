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

package podconstraint

import (
	"context"

	"gitlab.alibaba-inc.com/unischeduler/api/apis/scheduling/v1beta1"
	unifiedclientset "gitlab.alibaba-inc.com/unischeduler/api/client/clientset/versioned"
	unifiedinformer "gitlab.alibaba-inc.com/unischeduler/api/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type podConstraintEventHandler struct {
	podConstraintCache *PodConstraintCache
}

func registerPodConstraintEventHandler(handle framework.Handle, podConstraintCache *PodConstraintCache) error {
	unifiedClient, ok := handle.(unifiedclientset.Interface)
	if !ok {
		kubeConfig := handle.KubeConfig()
		kubeConfig.ContentType = runtime.ContentTypeJSON
		kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
		var err error
		unifiedClient, err = unifiedclientset.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}
	}
	podConstraintInformerFactory := unifiedinformer.NewSharedInformerFactoryWithOptions(unifiedClient, 0)
	eventHandler := &podConstraintEventHandler{
		podConstraintCache: podConstraintCache,
	}
	podConstraintInformerFactory.Scheduling().V1beta1().PodConstraints().Informer().AddEventHandler(eventHandler)
	podConstraintInformerFactory.Start(context.TODO().Done())
	podConstraintInformerFactory.WaitForCacheSync(context.TODO().Done())
	return nil
}

func (p podConstraintEventHandler) OnAdd(obj interface{}) {
	constraint := toPodConstraint(obj)
	if constraint == nil {
		return
	}
	klog.V(5).Infof("PodConstraint:%v/%v on create", constraint.Namespace, constraint.Name)
	p.podConstraintCache.SetPodConstraint(constraint)
}

func (p podConstraintEventHandler) OnUpdate(oldObj, newObj interface{}) {
	constraint := toPodConstraint(newObj)
	if constraint == nil {
		return
	}
	klog.V(5).Infof("PodConstraint:%v/%v on update", constraint.Namespace, constraint.Name)
	p.podConstraintCache.SetPodConstraint(constraint)

}

func (p podConstraintEventHandler) OnDelete(obj interface{}) {
	constraint := toPodConstraint(obj)
	if constraint == nil {
		return
	}
	klog.V(5).Infof("PodConstraint:%v/%v on delete", constraint.Namespace, constraint.Name)
	p.podConstraintCache.DelPodConstraint(constraint)
}

func toPodConstraint(obj interface{}) *v1beta1.PodConstraint {
	if obj == nil {
		return nil
	}
	var podConstraint *v1beta1.PodConstraint
	switch t := obj.(type) {
	case *v1beta1.PodConstraint:
		podConstraint = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		podConstraint, ok = t.Obj.(*v1beta1.PodConstraint)
		if !ok {
			klog.Errorf("unable to convert object %T to *v1beta1.PodConstraint", obj)
			return nil
		}
	default:
		klog.Errorf("unable to handle object in %T", obj)
		return nil
	}
	return podConstraint
}

/*
Copyright 2019 The Kubernetes Authors.

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

package target

import (
	"context"
	"fmt"
	"time"

	kruisev1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	kruise_informer "github.com/openkruise/kruise-api/client/informers/externalversions"
	recv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/autoscaling/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	cacheddiscovery "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	kube_client "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

const (
	discoveryResetPeriod = 5 * time.Minute
)

// RecommendationTargetSelectorFetcher gets a labelSelector used to gather Pods controlled by the given VPA.
type RecommendationTargetSelectorFetcher interface {
	// Fetch returns a labelSelector used to gather Pods controlled by the given VPA.
	// If error is nil, the returned labelSelector is not nil.
	Fetch(recommendation *recv1alpha1.Recommendation) (labels.Selector, error)
}

type wellKnownController string

const (
	// 原生 workload
	daemonSet             wellKnownController = "DaemonSet"
	deployment            wellKnownController = "Deployment"
	replicaSet            wellKnownController = "ReplicaSet"
	statefulSet           wellKnownController = "StatefulSet"
	replicationController wellKnownController = "ReplicationController"
	job                   wellKnownController = "Job"
	cronJob               wellKnownController = "CronJob"
	// kruise workload
	cloneSet          wellKnownController = "CloneSet"
	statefulSetKruise wellKnownController = "StatefulSets"
	daemonSetKruise   wellKnownController = "Daemonset"
	advancedCronJob   wellKnownController = "AdvancedCronJob"
)

// NewRecommendationTargetSelectorFetcher returns new instance of VpaTargetSelectorFetcher
func NewRecommendationTargetSelectorFetcher(config *rest.Config, kubeClient kube_client.Interface, factory informers.SharedInformerFactory,
	kruiseFactory kruise_informer.SharedInformerFactory, supportKruise bool) RecommendationTargetSelectorFetcher {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		klog.Fatalf("Could not create discoveryClient: %v", err)
	}
	resolver := scale.NewDiscoveryScaleKindResolver(discoveryClient)
	restClient := kubeClient.CoreV1().RESTClient()
	cachedDiscoveryClient := cacheddiscovery.NewMemCacheClient(discoveryClient)
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedDiscoveryClient)
	go wait.Until(func() {
		mapper.Reset()
	}, discoveryResetPeriod, make(chan struct{}))

	informersMap := map[wellKnownController]cache.SharedIndexInformer{
		daemonSet:             factory.Apps().V1().DaemonSets().Informer(),
		deployment:            factory.Apps().V1().Deployments().Informer(),
		replicaSet:            factory.Apps().V1().ReplicaSets().Informer(),
		statefulSet:           factory.Apps().V1().StatefulSets().Informer(),
		replicationController: factory.Core().V1().ReplicationControllers().Informer(),
		job:                   factory.Batch().V1().Jobs().Informer(),
		cronJob:               factory.Batch().V1beta1().CronJobs().Informer(),
	}

	if supportKruise {
		informersMap[cloneSet] = kruiseFactory.Apps().V1alpha1().CloneSets().Informer()
		informersMap[statefulSetKruise] = kruiseFactory.Apps().V1alpha1().StatefulSets().Informer()
		informersMap[daemonSetKruise] = kruiseFactory.Apps().V1alpha1().DaemonSets().Informer()
		informersMap[advancedCronJob] = kruiseFactory.Apps().V1alpha1().AdvancedCronJobs().Informer()
	}

	for kind, informer := range informersMap {
		stopCh := make(chan struct{})
		go informer.Run(stopCh)
		synced := cache.WaitForCacheSync(stopCh, informer.HasSynced)
		if !synced {
			klog.Fatalf("Could not sync cache for %s: %v", kind, err)
		} else {
			klog.Infof("Initial sync of %s completed", kind)
		}
	}

	scaleNamespacer := scale.New(restClient, mapper, dynamic.LegacyAPIPathResolverFunc, resolver)
	return &recommendationTargetSelectorFetcher{
		scaleNamespacer: scaleNamespacer,
		mapper:          mapper,
		informersMap:    informersMap,
	}
}

// recommendationTargetSelectorFetcher implements RecommendationTargetSelectorFetcher interface
// by querying API server for the controller pointed by Recommendation's targetRef
type recommendationTargetSelectorFetcher struct {
	scaleNamespacer scale.ScalesGetter
	mapper          apimeta.RESTMapper
	informersMap    map[wellKnownController]cache.SharedIndexInformer
}

func (f *recommendationTargetSelectorFetcher) Fetch(rec *recv1alpha1.Recommendation) (labels.Selector, error) {
	if rec.Spec.WorkloadRef == nil {
		return nil, fmt.Errorf("workloadRef not defined. If this is a v1beta1 object switch to v1beta2")
	}
	kind := wellKnownController(rec.Spec.WorkloadRef.Kind)
	informer, exists := f.informersMap[kind]
	if exists {
		return getLabelSelector(informer, rec.Spec.WorkloadRef.Kind, rec.Namespace, rec.Spec.WorkloadRef.Name)
	}

	// not on a list of known controllers, use scale sub-resource
	// TODO: cache response
	groupVersion, err := schema.ParseGroupVersion(rec.Spec.WorkloadRef.APIVersion)
	if err != nil {
		return nil, err
	}
	groupKind := schema.GroupKind{
		Group: groupVersion.Group,
		Kind:  rec.Spec.WorkloadRef.Kind,
	}

	selector, err := f.getLabelSelectorFromResource(groupKind, rec.Namespace, rec.Spec.WorkloadRef.Name)
	if err != nil {
		return nil, fmt.Errorf("unhandled targetRef %s / %s / %s, last error %v",
			rec.Spec.WorkloadRef.APIVersion, rec.Spec.WorkloadRef.Kind, rec.Spec.WorkloadRef.Name, err)
	}
	return selector, nil
}

func getLabelSelector(informer cache.SharedIndexInformer, kind, namespace, name string) (labels.Selector, error) {
	obj, exists, err := informer.GetStore().GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%s %s/%s does not exist", kind, namespace, name)
	}
	switch obj := obj.(type) {
	case *appsv1.DaemonSet:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *appsv1.Deployment:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *appsv1.StatefulSet:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *appsv1.ReplicaSet:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *batchv1.Job:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *batchv1beta1.CronJob:
		return metav1.LabelSelectorAsSelector(metav1.SetAsLabelSelector(obj.Spec.JobTemplate.Spec.Template.Labels))
	case *corev1.ReplicationController:
		return metav1.LabelSelectorAsSelector(metav1.SetAsLabelSelector(obj.Spec.Selector))
	case *kruisev1alpha1.CloneSet:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *kruisev1alpha1.StatefulSet:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *kruisev1alpha1.DaemonSet:
		return metav1.LabelSelectorAsSelector(obj.Spec.Selector)
	case *kruisev1alpha1.AdvancedCronJob:
		return metav1.LabelSelectorAsSelector(obj.Spec.Template.JobTemplate.Spec.Selector)
	}
	return nil, fmt.Errorf("don't know how to read label seletor")
}

func (f *recommendationTargetSelectorFetcher) getLabelSelectorFromResource(
	groupKind schema.GroupKind, namespace, name string,
) (labels.Selector, error) {
	mappings, err := f.mapper.RESTMappings(groupKind)
	if err != nil {
		return nil, err
	}

	var lastError error
	for _, mapping := range mappings {
		groupResource := mapping.Resource.GroupResource()
		scale, err := f.scaleNamespacer.Scales(namespace).Get(context.TODO(), groupResource, name, metav1.GetOptions{})
		if err == nil {
			if scale.Status.Selector == "" {
				return nil, fmt.Errorf("resource %s/%s has an empty selector for scale sub-resource", namespace, name)
			}
			selector, err := labels.Parse(scale.Status.Selector)
			if err != nil {
				return nil, err
			}
			return selector, nil
		}
		lastError = err
	}

	// nothing found, apparently the resource does support scale (or we lack RBAC)
	return nil, lastError
}

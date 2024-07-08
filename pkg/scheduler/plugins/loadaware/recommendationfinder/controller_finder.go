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

package recommendationfinder

import (
	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"

	autoscalinginformers "gitlab.alibaba-inc.com/cos/unified-resource-api/client/informers/externalversions/autoscaling/v1alpha1"
)

// ScaleAndSelector is used to return (controller, scale, selector) fields from the
// controller finder functions.
type ScaleAndSelector struct {
	ControllerReference
	// controller.spec.Replicas
	Scale int32
	// controller.spec.Selector
	Selector *metav1.LabelSelector
	// metadata
	Metadata metav1.ObjectMeta
}

type ControllerReference struct {
	// API version of the referent.
	APIVersion string `json:"apiVersion" protobuf:"bytes,5,opt,name=apiVersion"`
	// Kind of the referent.
	Kind string `json:"kind" protobuf:"bytes,1,opt,name=kind"`
	// Name of the referent.
	Name string `json:"name" protobuf:"bytes,3,opt,name=name"`
	// UID of the referent.
	UID types.UID `json:"uid" protobuf:"bytes,4,opt,name=uid,casttype=k8s.io/apimachinery/pkg/types.UID"`
}

// PodControllerFinder is a function type that maps a pod to a list of
// controllers and their scale.
type PodControllerFinder func(ref ControllerReference, namespace string) (*ScaleAndSelector, error)

type ControllerFinder struct {
	recommendationInformer        autoscalinginformers.RecommendationInformer
	statefulSetInformer           appslisters.StatefulSetLister
	deploymentInformer            appslisters.DeploymentLister
	replicaSetInformer            appslisters.ReplicaSetLister
	replicationControllerInformer corev1listers.ReplicationControllerLister
}

func NewControllerFinder(
	recommendationInformer autoscalinginformers.RecommendationInformer,
	statefulSetInformer appslisters.StatefulSetLister,
	deploymentInformer appslisters.DeploymentLister,
	replicaSetInformer appslisters.ReplicaSetLister,
	replicationControllerInformer corev1listers.ReplicationControllerLister,
) *ControllerFinder {
	return &ControllerFinder{
		recommendationInformer:        recommendationInformer,
		statefulSetInformer:           statefulSetInformer,
		deploymentInformer:            deploymentInformer,
		replicationControllerInformer: replicationControllerInformer,
		replicaSetInformer:            replicaSetInformer,
	}
}

func (r *ControllerFinder) GetScaleAndSelectorForRef(apiVersion, kind, ns, name string, uid types.UID) (*ScaleAndSelector, error) {
	targetRef := ControllerReference{
		APIVersion: apiVersion,
		Kind:       kind,
		Name:       name,
		UID:        uid,
	}

	for _, finder := range r.Finders() {
		scale, err := finder(targetRef, ns)
		if scale != nil || err != nil {
			return scale, err
		}
	}
	return nil, nil
}

func (r *ControllerFinder) Finders() []PodControllerFinder {
	return []PodControllerFinder{
		r.getPodReplicationController,
		r.getPodDeployment,
		r.getPodReplicaSet,
		r.getPodStatefulSet,
	}
}

var (
	ControllerKindRS  = apps.SchemeGroupVersion.WithKind("ReplicaSet")
	ControllerKindSS  = apps.SchemeGroupVersion.WithKind("StatefulSet")
	ControllerKindRC  = corev1.SchemeGroupVersion.WithKind("ReplicationController")
	ControllerKindDep = apps.SchemeGroupVersion.WithKind("Deployment")
)

// getPodReplicaSet finds a replicaset which has no matching deployments.
func (r *ControllerFinder) getPodReplicaSet(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref, ControllerKindRS.Kind, []string{ControllerKindRS.Group})
	if !ok {
		return nil, nil
	}
	replicaSet, err := r.getReplicaSet(ref, namespace)
	if err != nil {
		return nil, err
	}
	if replicaSet == nil {
		return nil, nil
	}
	controllerRef := metav1.GetControllerOf(replicaSet)
	if controllerRef != nil && controllerRef.Kind == ControllerKindDep.Kind {
		refSs := ControllerReference{
			APIVersion: controllerRef.APIVersion,
			Kind:       controllerRef.Kind,
			Name:       controllerRef.Name,
			UID:        controllerRef.UID,
		}
		return r.getPodDeployment(refSs, namespace)
	}

	apiVersion := replicaSet.APIVersion
	if apiVersion == "" {
		apiVersion = appsv1.SchemeGroupVersion.String()
	}
	kind := replicaSet.Kind
	if kind == "" {
		kind = "ReplicaSet"
	}

	return &ScaleAndSelector{
		Scale:    *(replicaSet.Spec.Replicas),
		Selector: replicaSet.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       replicaSet.Name,
			UID:        replicaSet.UID,
		},
		Metadata: replicaSet.ObjectMeta,
	}, nil
}

// getPodReplicaSet finds a replicaset which has no matching deployments.
func (r *ControllerFinder) getReplicaSet(ref ControllerReference, namespace string) (*apps.ReplicaSet, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref, ControllerKindRS.Kind, []string{ControllerKindRS.Group})
	if !ok {
		return nil, nil
	}
	replicaSet, err := r.replicaSetInformer.ReplicaSets(namespace).Get(ref.Name)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && replicaSet.UID != ref.UID {
		return nil, nil
	}
	return replicaSet, nil
}

// getPodStatefulSet returns the statefulset referenced by the provided controllerRef.
func (r *ControllerFinder) getPodStatefulSet(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref, ControllerKindSS.Kind, []string{ControllerKindSS.Group})
	if !ok {
		return nil, nil
	}
	statefulSet, err := r.statefulSetInformer.StatefulSets(namespace).Get(ref.Name)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && statefulSet.UID != ref.UID {
		return nil, nil
	}

	apiVersion := statefulSet.APIVersion
	if apiVersion == "" {
		apiVersion = appsv1.SchemeGroupVersion.String()
	}
	kind := statefulSet.Kind
	if kind == "" {
		kind = "StatefulSet"
	}

	return &ScaleAndSelector{
		Scale:    *(statefulSet.Spec.Replicas),
		Selector: statefulSet.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       statefulSet.Name,
			UID:        statefulSet.UID,
		},
		Metadata: statefulSet.ObjectMeta,
	}, nil
}

// getPodDeployments finds deployments for any replicasets which are being managed by deployments.
func (r *ControllerFinder) getPodDeployment(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref, ControllerKindDep.Kind, []string{ControllerKindDep.Group})
	if !ok {
		return nil, nil
	}
	deployment, err := r.deploymentInformer.Deployments(namespace).Get(ref.Name)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && deployment.UID != ref.UID {
		return nil, nil
	}
	apiVersion := deployment.APIVersion
	if apiVersion == "" {
		apiVersion = appsv1.SchemeGroupVersion.String()
	}
	kind := deployment.Kind
	if kind == "" {
		kind = "Deployment"
	}

	return &ScaleAndSelector{
		Scale:    *(deployment.Spec.Replicas),
		Selector: deployment.Spec.Selector,
		ControllerReference: ControllerReference{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       deployment.Name,
			UID:        deployment.UID,
		},
		Metadata: deployment.ObjectMeta,
	}, nil
}

func (r *ControllerFinder) getPodReplicationController(ref ControllerReference, namespace string) (*ScaleAndSelector, error) {
	// This error is irreversible, so there is no need to return error
	ok, _ := verifyGroupKind(ref, ControllerKindRC.Kind, []string{ControllerKindRC.Group})
	if !ok {
		return nil, nil
	}
	rc, err := r.replicationControllerInformer.ReplicationControllers(namespace).Get(ref.Name)
	if err != nil {
		// when error is NotFound, it is ok here.
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	if ref.UID != "" && rc.UID != ref.UID {
		return nil, nil
	}
	apiVersion := rc.APIVersion
	if apiVersion == "" {
		apiVersion = corev1.SchemeGroupVersion.String()
	}
	kind := rc.Kind
	if kind == "" {
		kind = "ReplicationController"
	}

	return &ScaleAndSelector{
		Scale:    *(rc.Spec.Replicas),
		Selector: &metav1.LabelSelector{MatchLabels: rc.Spec.Selector},
		ControllerReference: ControllerReference{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       rc.Name,
			UID:        rc.UID,
		},
		Metadata: rc.ObjectMeta,
	}, nil
}

func verifyGroupKind(ref ControllerReference, expectedKind string, expectedGroups []string) (bool, error) {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil {
		return false, err
	}

	if ref.Kind != expectedKind {
		return false, nil
	}

	for _, group := range expectedGroups {
		if group == gv.Group {
			return true, nil
		}
	}

	return false, nil
}

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

package reservation

import (
	"context"
	"fmt"

	"k8s.io/utils/pointer"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/drain/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Interpreter = &interpreterImpl{}

type interpreterImpl struct {
	apiReader client.Reader
	client.Client
}

func newInterpreter(c client.Client, apiReader client.Reader) Interpreter {
	return &interpreterImpl{
		apiReader: apiReader,
		Client:    c,
	}
}

func (p *interpreterImpl) GetReservationType() client.Object {
	return &sev1alpha1.Reservation{}
}

func (p *interpreterImpl) ToReservation(obj interface{}) Object {
	r, ok := obj.(*sev1alpha1.Reservation)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			klog.V(3).Infof("Couldn't get object from tombstone %#v", obj)
			return nil
		}
		r, ok = tombstone.Obj.(*sev1alpha1.Reservation)
		if !ok {
			klog.V(3).Infof("Tombstone contained object that is not a Reservation: %#v", obj)
			return nil
		}
	}
	return &Reservation{Reservation: r}
}

func (p *interpreterImpl) GetReservation(ctx context.Context, drainNode string, podUID types.UID) (Object, error) {
	r := &sev1alpha1.Reservation{}
	name := fmt.Sprintf("%s-%.8v", drainNode, podUID)
	if err := p.Client.Get(ctx, types.NamespacedName{Name: name}, r); err != nil && errors.IsNotFound(err) {
		if err := p.apiReader.Get(ctx, types.NamespacedName{Name: name}, r); err != nil && errors.IsNotFound(err) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return &Reservation{Reservation: r}, nil
}

func (p *interpreterImpl) DeleteReservations(ctx context.Context, drainNode string) error {
	if len(drainNode) == 0 {
		return nil
	}
	klog.Infof("delete reservations of drainNode %v", drainNode)
	selector, _ := labels.Parse(fmt.Sprintf("%v=%v", utils.DrainNodeKey, drainNode))
	list := &sev1alpha1.ReservationList{}
	if err := p.Client.List(ctx, list, &client.ListOptions{LabelSelector: selector}); err != nil {
		return err
	}
	for i := range list.Items {
		r := &list.Items[i]
		klog.V(3).Infof("delete reservations %v", r.Name)
		if err := p.Client.Delete(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (p *interpreterImpl) DeleteReservation(ctx context.Context, reservationName string) error {
	if len(reservationName) == 0 {
		return nil
	}
	r := &sev1alpha1.Reservation{}
	if err := p.Client.Get(ctx, types.NamespacedName{Name: reservationName}, r); err != nil {
		return err
	}
	klog.Infof("delete reservations %v", reservationName)
	if err := p.Client.Delete(ctx, r); err != nil {
		return err
	}
	return nil
}

func (p *interpreterImpl) ListReservation(ctx context.Context, drainNode string) ([]Object, error) {
	if len(drainNode) == 0 {
		return []Object{}, nil
	}
	selector, _ := labels.Parse(fmt.Sprintf("%v=%v", utils.DrainNodeKey, drainNode))
	list := &sev1alpha1.ReservationList{}
	if err := p.Client.List(ctx, list, &client.ListOptions{LabelSelector: selector}); err != nil {
		return []Object{}, err
	}
	ret := make([]Object, len(list.Items))
	for i := range list.Items {
		ret[i] = &Reservation{Reservation: &list.Items[i]}
	}
	return ret, nil
}

func mergeAffinity(affinity *corev1.Affinity) *corev1.Affinity {
	require := corev1.NodeSelectorRequirement{
		Key:      utils.PlanningKey,
		Operator: corev1.NodeSelectorOpDoesNotExist,
	}

	preferr := corev1.PreferredSchedulingTerm{
		Weight: 100,
		Preference: corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{{
				Key:      utils.GroupKey,
				Operator: corev1.NodeSelectorOpDoesNotExist,
			}},
		},
	}

	affinity = getAffinity(affinity)

	nodeSelectorTerms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	if len(nodeSelectorTerms) == 0 {
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms =
			append(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, corev1.NodeSelectorTerm{
				MatchExpressions: []corev1.NodeSelectorRequirement{require},
			})
	} else {
		for i := range nodeSelectorTerms {
			t := &nodeSelectorTerms[i]
			t.MatchExpressions = append(t.MatchExpressions, require)
		}
	}

	affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution =
		append(affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution, preferr)

	return affinity
}

func getAffinity(affinity *corev1.Affinity) *corev1.Affinity {
	if affinity == nil {
		affinity = &corev1.Affinity{}
	}
	nodeAffinity := affinity.NodeAffinity
	if nodeAffinity == nil {
		affinity.NodeAffinity = &corev1.NodeAffinity{}
		nodeAffinity = affinity.NodeAffinity
	}
	required := nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	if required == nil {
		nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
		required = nodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	}
	if required.NodeSelectorTerms == nil {
		required.NodeSelectorTerms = []corev1.NodeSelectorTerm{}
	}

	return affinity
}

func (p *interpreterImpl) CreateReservation(ctx context.Context, dn *sev1alpha1.DrainNode, pod *corev1.Pod) (Object, error) {
	if dn == nil || pod == nil {
		return nil, fmt.Errorf("parameter is nil")
	}
	podOwnerReference := metav1.GetControllerOf(pod)
	if podOwnerReference == nil {
		return nil, fmt.Errorf("pod OwnerReference is nil")
	}
	newPod := pod.DeepCopy()
	newPod.Spec.Affinity = mergeAffinity(newPod.Spec.Affinity)
	newPod.Spec.NodeName = ""
	if newPod.Labels == nil {
		newPod.Labels = map[string]string{}
	}
	newPod.Labels[utils.PodNameKey] = pod.Name
	template := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       newPod.Namespace,
			Labels:          newPod.Labels,
			Annotations:     newPod.Annotations,
			OwnerReferences: newPod.OwnerReferences,
		},
		Spec: newPod.Spec,
	}
	name := fmt.Sprintf("%s-%.8v", dn.Name, pod.UID)
	reservation := &sev1alpha1.Reservation{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				utils.DrainNodeKey:    dn.Name,
				utils.PodNamespaceKey: pod.Namespace,
				utils.PodNameKey:      pod.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(dn, utils.DrainNodeKind)},
		},
		Spec: sev1alpha1.ReservationSpec{
			Template: template,
			Owners: []sev1alpha1.ReservationOwner{
				{Controller: &sev1alpha1.ReservationControllerReference{
					OwnerReference: *podOwnerReference,
					Namespace:      pod.Namespace,
				}},
			},
			AllocateOnce: pointer.Bool(true),
		},
	}

	err := p.Client.Create(ctx, reservation)
	if errors.IsAlreadyExists(err) {
		err = p.Client.Get(ctx, types.NamespacedName{Name: reservation.Name}, reservation)
	}
	if err != nil {
		return nil, err
	}
	return &Reservation{Reservation: reservation}, nil
}

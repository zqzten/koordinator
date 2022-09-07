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

package cgroups

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

// PodCgroupsReconciler watches Pod object event and update corresponding NodeSLO
type PodCgroupsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

func (r *PodCgroupsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = log.FromContext(ctx, "cgroups-pod", req.NamespacedName)

	klog.V(5).Infof("starting process pod %v", req.NamespacedName.String())

	pod := &corev1.Pod{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, pod)

	if err != nil {
		if errors.IsNotFound(err) {
			// pod is delete, handle in gc queue
			klog.V(5).Infof("pod %v not found, maybe already deleted", req.NamespacedName.String())
			return reconcile.Result{}, nil
		} else {
			klog.Errorf("pod cgroup reconciler failed to find pod %v, error %v", req.NamespacedName.String(), err)
			return reconcile.Result{Requeue: true}, err
		}
	}

	// pod is create or update, update corresponding NodeSLO
	return r.handleCreateOrUpdatePod(pod)
}

func (r *PodCgroupsReconciler) handleCreateOrUpdatePod(pod *corev1.Pod) (ctrl.Result, error) {
	if pod == nil || pod.Spec.NodeName == "" {
		return reconcile.Result{}, nil
	}

	// get Cgroups by pod
	cgroups, containerInfo, err := r.findCgroupsByPod(pod)
	if err != nil {
		klog.Warningf("find cgroup by pod failed during handle %v/%v", pod.Namespace, pod.Name)
		return reconcile.Result{Requeue: true}, err
	}
	if cgroups == nil {
		// cgroups not exist, ignore pod
		klog.V(5).Infof("no cgroup found for pod %v/%v", pod.Namespace, pod.Name)
		return reconcile.Result{}, nil
	}

	// get nodeSLO by node name of pod
	nodeSLO := &slov1alpha1.NodeSLO{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, nodeSLO)
	if err != nil {
		klog.Warningf("failed to get node slo %v for pod %v/%v during handle cgroup for pod",
			nodeSLO.Name, pod.Namespace, pod.Name)
		return reconcile.Result{Requeue: true}, err
	}

	podCgroup := generatePodCgroup(pod, containerInfo)

	// insert pod cgroup into node slo
	newNodeSLOSpec := nodeSLO.Spec.DeepCopy()
	oldNodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLO.Spec)
	if err != nil {
		klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLO.Name, err)
		return reconcile.Result{}, nil
	}
	newNodeCgroups := insertPodCgroupToNode(cgroups.Namespace, cgroups.Name, podCgroup, oldNodeCgroups)
	slov1alpha1.SetNodeCgroups(newNodeSLOSpec, newNodeCgroups)

	// update node slo cgroup spec
	nodeSLO.Spec = *newNodeSLOSpec
	err = r.Client.Update(context.TODO(), nodeSLO)

	if err != nil {
		detailStr, _ := json.Marshal(nodeSLO.Spec.Extensions)
		klog.Warningf("update node slo %v failed during handle pod create or update, detail %v, error %v",
			nodeSLO.Name, string(detailStr), err)
		return reconcile.Result{Requeue: true}, err
	}

	klog.Infof("update node slo %v succeed for pod %v/%v", nodeSLO.Name, pod.Namespace, pod.Name)
	return reconcile.Result{}, nil
}

func (r *PodCgroupsReconciler) findCgroupsByPod(pod *corev1.Pod) (*resourcesv1alpha1.Cgroups,
	[]resourcesv1alpha1.ContainerInfoSpec, error) {
	cgroupsList := &resourcesv1alpha1.CgroupsList{}
	err := r.Client.List(context.TODO(), cgroupsList)
	if err != nil {
		return nil, nil, err
	}
	for i := range cgroupsList.Items {
		if cgroupsList.Items[i].Spec.PodInfo.Name != "" &&
			matchCgroupsPodInfoSpec(pod, &cgroupsList.Items[i].Spec.PodInfo) {
			return &cgroupsList.Items[i], cgroupsList.Items[i].Spec.PodInfo.Containers, nil
		}
		if cgroupsList.Items[i].Spec.DeploymentInfo.Name != "" {
			deployemnt, err := getDeploymentByPod(r.Client, pod)
			if err != nil {
				return nil, nil, err
			}
			if matchCgroupsDeploymentInfoSpec(deployemnt, &cgroupsList.Items[i].Spec.DeploymentInfo) {
				return &cgroupsList.Items[i], cgroupsList.Items[i].Spec.DeploymentInfo.Containers, nil
			}
		}
		if cgroupsList.Items[i].Spec.JobInfo.Name != "" &&
			matchCgroupsJobInfoSpec(pod, &cgroupsList.Items[i].Spec.JobInfo) {
			return &cgroupsList.Items[i], cgroupsList.Items[i].Spec.JobInfo.Containers, nil
		}
	}
	return nil, nil, nil
}

func (r *PodCgroupsReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options,
	recycler *CgroupsInfoRecycler) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&corev1.Pod{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &podEventHandler{recycler: recycler}).
		Complete(r)
}

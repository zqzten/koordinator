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
	"fmt"

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

// CgroupsReconciler watches Cgroups object event and update corresponding NodeSLO
type CgroupsReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=resources.alibabacloud.com,resources=cgroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=resources.alibabacloud.com,resources=cgroups/status,verbs=get;update;patch

func (r *CgroupsReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	_ = log.FromContext(ctx, "cgroups", req.NamespacedName)

	klog.V(5).Infof("starting process cgroups %v", req.NamespacedName.String())

	cgroups := &resourcesv1alpha1.Cgroups{}
	err := r.Client.Get(context.TODO(), req.NamespacedName, cgroups)

	if err != nil {
		if errors.IsNotFound(err) {
			// cgroups is deleted, handle in gc queue
			klog.V(5).Infof("cgroups %v not found, maybe already deleted", req.NamespacedName.String())
			return r.handleDelete(&req.NamespacedName)
		} else {
			klog.Errorf("cgroup reconciler failed to find cgroups %v, error %v", req.NamespacedName.String(), err)
			return reconcile.Result{Requeue: true}, err
		}
	}

	// cgroups id create or update, update corresponding NodeSLO
	return r.handleCreatOrUpdate(cgroups)
}

func (r *CgroupsReconciler) handleCreatOrUpdate(cgroups *resourcesv1alpha1.Cgroups) (ctrl.Result, error) {
	if cgroups == nil {
		return reconcile.Result{}, nil
	}
	// handle PodInfo from Cgroup crd
	if cgroups.Spec.PodInfo.Name != "" {
		podResult, err := r.handleCreateOrUpdateOfPodInfo(cgroups)
		if err != nil {
			return podResult, err
		}
	}

	// handle DeploymentInfo from Cgroup crd
	if cgroups.Spec.DeploymentInfo.Name != "" {
		deploymentResult, err := r.handleCreateOrUpdateOfDeploymentInfo(cgroups)
		if err != nil {
			return deploymentResult, err
		}
	}

	// handle JobInfo from Cgroup crd
	if cgroups.Spec.JobInfo.Name != "" {
		jobResult, err := r.handleCreateOrUpdateOfJobInfo(cgroups)
		if err != nil {
			return jobResult, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *CgroupsReconciler) handleCreateOrUpdateOfPodInfo(cgroups *resourcesv1alpha1.Cgroups) (ctrl.Result, error) {
	// get pod by namespaced-name in Cgroup.PodInfo
	pod, err := r.getTargetPodByPodInfo(&cgroups.Spec.PodInfo)
	if err != nil {
		// some error happened during get pod, retry
		klog.Warningf("failed to get pod %v/%v during handle cgroup %v/%v, error %v",
			cgroups.Spec.PodInfo.Namespace, cgroups.Spec.PodInfo.Name, cgroups.Namespace, cgroups.Name, err)
		return reconcile.Result{Requeue: true}, err
	}
	if err := r.handleOnePod(cgroups.Namespace, cgroups.Name, cgroups.Spec.PodInfo.Containers, pod); err != nil {
		klog.Warningf("handle cgroups %v/%v pod info %v/%v failed, error %v", cgroups.Namespace, cgroups.Name,
			cgroups.Spec.PodInfo.Namespace, cgroups.Spec.PodInfo.Name, err)
		return reconcile.Result{Requeue: true}, err
	}
	return reconcile.Result{}, nil
}

func (r *CgroupsReconciler) handleCreateOrUpdateOfDeploymentInfo(cgroups *resourcesv1alpha1.Cgroups) (ctrl.Result, error) {
	pods, err := r.getTargetPodsByDeploymentInfo(&cgroups.Spec.DeploymentInfo)
	if err != nil {
		// some error happened during get pod by deployment, retry
		klog.Warningf("failed to get pod by deployment %v/%v during handle cgroup %v/%v, error %v",
			cgroups.Spec.DeploymentInfo.Namespace, cgroups.Spec.DeploymentInfo.Name,
			cgroups.Namespace, cgroups.Name, err)
		return reconcile.Result{Requeue: true}, err
	}
	result := reconcile.Result{}
	var errResult error
	for _, pod := range pods {
		if err := r.handleOnePod(
			cgroups.Namespace, cgroups.Name, cgroups.Spec.DeploymentInfo.Containers, pod); err != nil {
			klog.Warningf("handle cgroups %v/%v deployment info failed, pod %v/%v, error %v",
				cgroups.Namespace, cgroups.Name, pod.Namespace, pod.Name, err)
			result.Requeue = true
			errResult = err
		}
	}
	return result, errResult
}

func (r *CgroupsReconciler) handleCreateOrUpdateOfJobInfo(cgroups *resourcesv1alpha1.Cgroups) (ctrl.Result, error) {
	pods, err := r.getTargetPodsByJobInfo(&cgroups.Spec.JobInfo)
	if err != nil {
		// some error happened during get pod by deployment, retry
		klog.Warningf("failed to get pod by job %v/%v during handle cgroup %v/%v, error %v",
			cgroups.Spec.JobInfo.Namespace, cgroups.Spec.JobInfo.Name, cgroups.Namespace, cgroups.Name, err)
		return reconcile.Result{Requeue: true}, err
	}
	result := reconcile.Result{}
	var errResult error
	for _, pod := range pods {
		if err := r.handleOnePod(cgroups.Namespace, cgroups.Name, cgroups.Spec.JobInfo.Containers, pod); err != nil {
			klog.Warningf("handle cgroups %v/%v job info failed, pod %v/%v, error %v",
				cgroups.Namespace, cgroups.Name, pod.Namespace, pod.Name, err)
			result.Requeue = true
			errResult = err
		}
	}
	return result, errResult
}

func (r *CgroupsReconciler) handleOnePod(cgroupsNamespace, cgroupsName string,
	containerSpec []resourcesv1alpha1.ContainerInfoSpec, pod *corev1.Pod) error {
	if pod == nil || pod.Spec.NodeName == "" {
		// pod has not been created or scheduled, will be handled later in pod reconciler
		return nil
	}

	// get nodeSLO by node name of pod
	nodeSLO := &slov1alpha1.NodeSLO{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{Name: pod.Spec.NodeName}, nodeSLO); err != nil {
		return fmt.Errorf("failed to get node slo %v, error %v", nodeSLO.Name, err)
	}

	// insert pod cgroup into node slo
	updated := false
	newNodeSLOSpec := nodeSLO.Spec.DeepCopy()
	oldNodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLO.Spec)
	if err != nil {
		klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLO.Name, err)
		return nil
	}
	newNodeCgroups, updated := insertOrReplaceContainerSpecToNode(cgroupsNamespace, cgroupsName, pod, containerSpec,
		oldNodeCgroups)
	slov1alpha1.SetNodeCgroups(newNodeSLOSpec, newNodeCgroups)

	if !updated {
		return nil
	}
	// update node slo cgroup spec
	nodeSLO.Spec = *newNodeSLOSpec
	if err := r.Client.Update(context.TODO(), nodeSLO); err != nil {
		detailStr, _ := json.Marshal(nodeSLO.Spec.Extensions)
		return fmt.Errorf("update node slo %v failed, detail %v, error %v", nodeSLO.Name, string(detailStr), err)
	}
	return nil
}

func (r *CgroupsReconciler) handleDelete(cgroups *types.NamespacedName) (ctrl.Result, error) {
	if cgroups == nil {
		return reconcile.Result{}, nil
	}
	nodeSLOList := &slov1alpha1.NodeSLOList{}
	err := r.Client.List(context.TODO(), nodeSLOList)
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	// prepare new NodeSLO for update
	// FIXME for Cgroups.PodInfoSpec, update correspoinding NodeSLO by pod.NodeName,
	// for Cgroups.DeployemntInfoSpec and Cgroups.JobInfoSpec, udpate in gc async
	newNodeSLOs := make([]*slov1alpha1.NodeSLO, 0)
	for i := range nodeSLOList.Items {
		oldNodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLOList.Items[i].Spec)
		if err != nil {
			klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLOList.Items[i].Name, err)
			continue
		}
		newNodeCgroups, exist := removeOneEntireCgroupsIfExist(cgroups.Namespace, cgroups.Name, oldNodeCgroups)
		if exist {
			newNodeSLO := nodeSLOList.Items[i].DeepCopy()
			slov1alpha1.SetNodeCgroups(&newNodeSLO.Spec, newNodeCgroups)
			newNodeSLOs = append(newNodeSLOs, newNodeSLO)
		}
	}

	// update NodeSLO
	for _, nodeSLO := range newNodeSLOs {
		err = r.Client.Update(context.TODO(), nodeSLO)
		if err != nil {
			detailStr, _ := json.Marshal(nodeSLO.Spec.Extensions)
			klog.Warningf("update node slo %v failed during handle cgroup crd delete, detail %v, error %v",
				nodeSLO.Name, string(detailStr), err)
			return reconcile.Result{Requeue: true}, err
		}
		klog.Infof("update node slo %v succeed during handle cgroup crd delete for cgroup %v/%v",
			nodeSLO.Name, cgroups.Namespace, cgroups.Name)
	}
	return reconcile.Result{}, nil
}

func (r *CgroupsReconciler) getTargetPodByPodInfo(podInfo *resourcesv1alpha1.PodInfoSpec) (*corev1.Pod, error) {
	if podInfo == nil {
		return nil, nil
	}
	pod := &corev1.Pod{}
	podNamespacedName := types.NamespacedName{
		Namespace: podInfo.Namespace,
		Name:      podInfo.Name,
	}
	err := r.Client.Get(context.TODO(), podNamespacedName, pod)
	if err == nil {
		return pod, nil
	} else if errors.IsNotFound(err) {
		// pod has not created
		return nil, nil
	} else {
		// some error happened
		klog.Warningf("cgroup reconciler get pod failed %v, error %v", podNamespacedName, err)
		return pod, err
	}
}

func (r *CgroupsReconciler) getTargetPodsByDeploymentInfo(
	deploymentInfo *resourcesv1alpha1.DeploymentInfoSpec) ([]*corev1.Pod, error) {
	if deploymentInfo == nil {
		return []*corev1.Pod{}, nil
	}
	return getPodsByDeployment(r.Client, deploymentInfo.Namespace, deploymentInfo.Name)
}

func (r *CgroupsReconciler) getTargetPodsByJobInfo(jobInfo *resourcesv1alpha1.JobInfoSpec) ([]*corev1.Pod, error) {
	if jobInfo == nil {
		return []*corev1.Pod{}, nil
	}
	return getPodsByJobInfo(r.Client, jobInfo.Namespace, jobInfo.Name)
}

func (r *CgroupsReconciler) SetupWithManager(mgr ctrl.Manager, options controller.Options,
	recycler *CgroupsInfoRecycler) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&resourcesv1alpha1.Cgroups{}).
		Watches(&source.Kind{Type: &resourcesv1alpha1.Cgroups{}}, &cgroupsEventHandler{recycler: recycler}).
		Complete(r)
}

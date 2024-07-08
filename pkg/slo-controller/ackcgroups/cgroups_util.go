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
	"fmt"
	"reflect"

	appv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

const (
	PodLabelKeyJobName = "job-name"

	ownerKindDeployment = "Deployment"
	ownerKindReplicaSet = "ReplicaSet"
)

func insertOrReplaceContainerSpecToNode(cgroupsNamespace, cgroupName string, pod *corev1.Pod,
	containerSpec []resourcesv1alpha1.ContainerInfoSpec,
	oldNodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo) ([]*resourcesv1alpha1.NodeCgroupsInfo, bool) {
	updated := false
	// get targetNodeCgroup from oldNodeCgroups by cgroup crd name, create new into oldNodeCgroups if not exist
	// remove duplicate podCgroup from node if already exist
	newNodeCgroups := make([]*resourcesv1alpha1.NodeCgroupsInfo, 0, len(oldNodeCgroups))
	var targetNodeCgroup *resourcesv1alpha1.NodeCgroupsInfo
	for _, oldNodeCgroup := range oldNodeCgroups {
		if oldNodeCgroup.CgroupsNamespace == cgroupsNamespace && oldNodeCgroup.CgroupsName == cgroupName {
			targetNodeCgroup = oldNodeCgroup
			newNodeCgroups = append(newNodeCgroups, oldNodeCgroup)
			continue
		} else {
			newPodCgroup := make([]*resourcesv1alpha1.PodCgroupsInfo, 0, len(oldNodeCgroup.PodCgroups))
			for _, oldPodCgroup := range oldNodeCgroup.PodCgroups {
				if oldPodCgroup.PodNamespace == pod.Namespace && oldPodCgroup.PodName == pod.Name {
					// pod exist in another cgroups, remove old
					updated = true
					continue
				}
				newPodCgroup = append(newPodCgroup, oldPodCgroup)
			}
			if len(newPodCgroup) > 0 {
				newNodeCgroup := &resourcesv1alpha1.NodeCgroupsInfo{
					CgroupsNamespace: oldNodeCgroup.CgroupsNamespace,
					CgroupsName:      oldNodeCgroup.CgroupsName,
					PodCgroups:       newPodCgroup,
				}
				newNodeCgroups = append(newNodeCgroups, newNodeCgroup)
			}
		}
	}

	if targetNodeCgroup == nil {
		targetNodeCgroup = &resourcesv1alpha1.NodeCgroupsInfo{
			CgroupsNamespace: cgroupsNamespace,
			CgroupsName:      cgroupName,
			PodCgroups:       make([]*resourcesv1alpha1.PodCgroupsInfo, 0),
		}
		newNodeCgroups = append(newNodeCgroups, targetNodeCgroup)
		updated = true
	}

	// get targetPodCgroup from targetNodeCgroup by pod namespace/name, create new into targetNodeCgroup if not exist
	var targetPodCgroup *resourcesv1alpha1.PodCgroupsInfo
	for _, podCgroup := range targetNodeCgroup.PodCgroups {
		if podCgroup.PodNamespace == pod.Namespace && podCgroup.PodName == pod.Name {
			targetPodCgroup = podCgroup
			break
		}
	}
	if targetPodCgroup == nil {
		targetPodCgroup = &resourcesv1alpha1.PodCgroupsInfo{
			PodNamespace: pod.Namespace,
			PodName:      pod.Name,
		}
		targetNodeCgroup.PodCgroups = append(targetNodeCgroup.PodCgroups, targetPodCgroup)
		updated = true
	}
	newContainerCgroups := generateNewContainerCgroups(containerSpec)
	if !reflect.DeepEqual(targetPodCgroup.Containers, newContainerCgroups) {
		targetPodCgroup.Containers = newContainerCgroups
		updated = true
	}
	return newNodeCgroups, updated
}

func insertPodCgroupToNode(cgroupsNamespace, cgroupName string, newPodCgroup *resourcesv1alpha1.PodCgroupsInfo,
	oldNodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo) []*resourcesv1alpha1.NodeCgroupsInfo {
	newNodeCgroups := oldNodeCgroups
	if newNodeCgroups == nil {
		newNodeCgroups = make([]*resourcesv1alpha1.NodeCgroupsInfo, 0, 1)
	}
	// get cgroup on node, create new if not exist
	var targetCgroup *resourcesv1alpha1.NodeCgroupsInfo
	for _, oldCgroups := range newNodeCgroups {
		if oldCgroups.CgroupsNamespace == cgroupsNamespace && oldCgroups.CgroupsName == cgroupName {
			targetCgroup = oldCgroups
			break
		}
	}
	if targetCgroup == nil {
		targetCgroup = &resourcesv1alpha1.NodeCgroupsInfo{
			CgroupsNamespace: cgroupsNamespace,
			CgroupsName:      cgroupName,
			PodCgroups:       make([]*resourcesv1alpha1.PodCgroupsInfo, 0, 1),
		}
		newNodeCgroups = append(newNodeCgroups, targetCgroup)
	}

	// update container if pod cgroup already exists
	for _, oldPodCgroups := range targetCgroup.PodCgroups {
		if oldPodCgroups.PodNamespace == newPodCgroup.PodNamespace && oldPodCgroups.PodName == newPodCgroup.PodName {
			// new pod cgroup already exist, only needs to update containers
			oldPodCgroups.Containers = newPodCgroup.Containers
			return newNodeCgroups
		}
	}
	// append new pod cgroup
	targetCgroup.PodCgroups = append(targetCgroup.PodCgroups, newPodCgroup)
	return newNodeCgroups
}

func removeOneEntireCgroupsIfExist(cgroupsNamespace, cgroupsName string,
	oldNodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo) ([]*resourcesv1alpha1.NodeCgroupsInfo, bool) {
	if len(oldNodeCgroups) == 0 {
		return oldNodeCgroups, false
	}

	newNodeCgroups := make([]*resourcesv1alpha1.NodeCgroupsInfo, 0, len(oldNodeCgroups))
	exist := false
	for _, nodeCgroups := range oldNodeCgroups {
		if nodeCgroups.CgroupsNamespace == cgroupsNamespace && nodeCgroups.CgroupsName == cgroupsName {
			exist = true
		} else {
			newNodeCgroups = append(newNodeCgroups, nodeCgroups)
		}
	}
	return newNodeCgroups, exist
}

func removeEntireCgroupsIfExist(cgroupsNamespacedName map[string]struct{},
	oldNodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo) ([]*resourcesv1alpha1.NodeCgroupsInfo, bool) {
	if len(cgroupsNamespacedName) == 0 || len(oldNodeCgroups) == 0 {
		return oldNodeCgroups, false
	}

	newNodeCgroups := make([]*resourcesv1alpha1.NodeCgroupsInfo, 0, len(oldNodeCgroups))
	exist := false
	for _, nodeCgroups := range oldNodeCgroups {
		oldNodeCgroupName := types.NamespacedName{
			Namespace: nodeCgroups.CgroupsNamespace,
			Name:      nodeCgroups.CgroupsName,
		}
		if _, cgroupsExist := cgroupsNamespacedName[oldNodeCgroupName.String()]; cgroupsExist {
			exist = true
		} else {
			newNodeCgroups = append(newNodeCgroups, nodeCgroups)
		}
	}
	return newNodeCgroups, exist
}

func removeOnePodCgroupFromNodeIfExist(podNamespace, podName string,
	oldNodeCgroups []*resourcesv1alpha1.NodeCgroupsInfo) ([]*resourcesv1alpha1.NodeCgroupsInfo, bool) {
	newNodeCgroups := make([]*resourcesv1alpha1.NodeCgroupsInfo, 0, len(oldNodeCgroups))
	exist := false
	for _, nodeCgroups := range oldNodeCgroups {
		newPodCgroup := make([]*resourcesv1alpha1.PodCgroupsInfo, 0, len(nodeCgroups.PodCgroups))
		for _, podCgroup := range nodeCgroups.PodCgroups {
			if podCgroup.PodNamespace == podNamespace && podCgroup.PodName == podName {
				exist = true
			} else {
				newPodCgroup = append(newPodCgroup, podCgroup)
			}
		}

		newNodeCgroupsItem := nodeCgroups
		if exist {
			newNodeCgroupsItem.PodCgroups = newPodCgroup
		}
		if len(newNodeCgroupsItem.PodCgroups) > 0 {
			newNodeCgroups = append(newNodeCgroups, newNodeCgroupsItem)
		}
	}
	return newNodeCgroups, exist
}

func removePodCgroupsFromNodeIfExist(podsNamespacedName map[string]struct{},
	oldNodeCgroupsList []*resourcesv1alpha1.NodeCgroupsInfo) ([]*resourcesv1alpha1.NodeCgroupsInfo, bool) {
	if len(oldNodeCgroupsList) == 0 || len(podsNamespacedName) == 0 {
		return oldNodeCgroupsList, false
	}

	newNodeCgroups := make([]*resourcesv1alpha1.NodeCgroupsInfo, 0, len(oldNodeCgroupsList))
	exist := false
	for _, oldNodeCgroups := range oldNodeCgroupsList {
		newPodCgroup := make([]*resourcesv1alpha1.PodCgroupsInfo, 0, len(oldNodeCgroups.PodCgroups))
		for _, podCgroup := range oldNodeCgroups.PodCgroups {
			curPod := types.NamespacedName{
				Namespace: podCgroup.PodNamespace,
				Name:      podCgroup.PodName,
			}
			curPodKey := curPod.String()
			_, podExist := podsNamespacedName[curPodKey]
			if podExist {
				exist = true
			} else {
				newPodCgroup = append(newPodCgroup, podCgroup)
			}
		}

		newNodeCgroupsItem := oldNodeCgroups
		if exist {
			newNodeCgroupsItem.PodCgroups = newPodCgroup
		}
		if len(newNodeCgroupsItem.PodCgroups) > 0 {
			newNodeCgroups = append(newNodeCgroups, newNodeCgroupsItem)
		}
	}
	return newNodeCgroups, exist
}

func generateNewContainerCgroups(
	inputContainers []resourcesv1alpha1.ContainerInfoSpec) []*resourcesv1alpha1.ContainerInfoSpec {
	output := make([]*resourcesv1alpha1.ContainerInfoSpec, 0, len(inputContainers))
	for i := range inputContainers {
		output = append(output, &inputContainers[i])
	}
	return output
}

func generatePodCgroup(pod *corev1.Pod,
	containerInfo []resourcesv1alpha1.ContainerInfoSpec) *resourcesv1alpha1.PodCgroupsInfo {
	if pod == nil || len(containerInfo) == 0 {
		return nil
	}
	return &resourcesv1alpha1.PodCgroupsInfo{
		PodNamespace: pod.Namespace,
		PodName:      pod.Name,
		Containers:   generateNewContainerCgroups(containerInfo),
	}
}

func isCgroupsSpecEmpty(cgroups *resourcesv1alpha1.Cgroups) bool {
	return cgroups.Spec.PodInfo.Namespace == "" && cgroups.Spec.PodInfo.Name == "" &&
		cgroups.Spec.DeploymentInfo.Namespace == "" && cgroups.Spec.DeploymentInfo.Name == "" &&
		cgroups.Spec.JobInfo.Namespace == "" && cgroups.Spec.JobInfo.Name == ""
}

func cgroupsSub(l, r *resourcesv1alpha1.Cgroups) *resourcesv1alpha1.Cgroups {
	// return different set of l - r by namespace and name of PodInfo/DeploymentInfo/JobInfo
	result := &resourcesv1alpha1.Cgroups{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: l.Namespace,
			Name:      l.Name,
		},
	}
	if l.Spec.PodInfo.Namespace != r.Spec.PodInfo.Namespace || l.Spec.PodInfo.Name != r.Spec.PodInfo.Name {
		result.Spec.PodInfo.Namespace = l.Spec.PodInfo.Namespace
		result.Spec.PodInfo.Name = l.Spec.PodInfo.Name
	}
	if l.Spec.DeploymentInfo.Namespace != r.Spec.DeploymentInfo.Namespace ||
		l.Spec.DeploymentInfo.Name != r.Spec.DeploymentInfo.Name {
		result.Spec.DeploymentInfo.Namespace = l.Spec.DeploymentInfo.Namespace
		result.Spec.DeploymentInfo.Name = l.Spec.DeploymentInfo.Name
	}
	if l.Spec.JobInfo.Namespace != r.Spec.JobInfo.Namespace || l.Spec.JobInfo.Name != r.Spec.JobInfo.Name {
		result.Spec.JobInfo.Namespace = l.Spec.JobInfo.Namespace
		result.Spec.JobInfo.Name = l.Spec.JobInfo.Name
	}
	return result
}

func containsCgroups(nodeCgroupsList []*resourcesv1alpha1.NodeCgroupsInfo, cgroups *resourcesv1alpha1.Cgroups) bool {
	if cgroups == nil || len(nodeCgroupsList) == 0 {
		return false
	}
	for _, nodeCgroups := range nodeCgroupsList {
		if nodeCgroups.CgroupsNamespace == cgroups.Namespace && nodeCgroups.CgroupsName == cgroups.Name {
			return true
		}
	}
	return false
}

func matchCgroupsPodInfoSpec(pod *corev1.Pod, podInfo *resourcesv1alpha1.PodInfoSpec) bool {
	if pod == nil || podInfo == nil {
		return false
	}
	return podInfo.Namespace == pod.Namespace && podInfo.Name == pod.Name
}

func matchCgroupsDeploymentInfoSpec(deployment *appv1.Deployment,
	deploymentInfo *resourcesv1alpha1.DeploymentInfoSpec) bool {
	if deployment == nil || deploymentInfo == nil {
		return false
	}
	return deploymentInfo.Namespace == deployment.Namespace && deploymentInfo.Name == deployment.Name
}

func matchCgroupsJobInfoSpec(pod *corev1.Pod, jobInfo *resourcesv1alpha1.JobInfoSpec) bool {
	if pod == nil || pod.Labels == nil || jobInfo == nil {
		return false
	}
	var podJobName string
	var exist bool
	if podJobName, exist = pod.Labels[PodLabelKeyJobName]; !exist {
		return false
	}
	return jobInfo.Namespace == pod.Namespace && jobInfo.Name == podJobName
}

func ownerRefMatchUID(owners []metav1.OwnerReference, targetUID types.UID) bool {
	for _, owner := range owners {
		if owner.UID == targetUID {
			return true
		}
	}
	return false
}

func getPodsByDeployment(c client.Client, namespace, name string) ([]*corev1.Pod, error) {
	targetPods := make([]*corev1.Pod, 0)

	// get deployment
	depNamespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	deployment := &appv1.Deployment{}
	if err := c.Get(context.TODO(), depNamespacedName, deployment); errors.IsNotFound(err) {
		// deployment has not created
		return targetPods, nil
	} else if err != nil {
		return targetPods, fmt.Errorf("get deployment failed %v, error %v", depNamespacedName, err)
	} // else err == nil

	// prepare selector
	depLabelselector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return targetPods, fmt.Errorf("prepare label selector for deployment %v failed, error %v",
			depNamespacedName, err)
	}
	listOpt := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: depLabelselector,
	}

	// get replicaSet of deployment
	replicaSetList := &appv1.ReplicaSetList{}
	if err := c.List(context.TODO(), replicaSetList, listOpt); err != nil {
		return targetPods, fmt.Errorf("get replicaSet for deployment %v failed, error %v", depNamespacedName, err)
	}
	// filter replicaSet by deployment uid
	var targetReplicaset *appv1.ReplicaSet
	for i := range replicaSetList.Items {
		replicaSet := &replicaSetList.Items[i]
		if ownerRefMatchUID(replicaSet.OwnerReferences, deployment.UID) {
			targetReplicaset = replicaSet
		}
	}
	if targetReplicaset == nil {
		return targetPods, fmt.Errorf("replicaSet not exist in deployment %v, no need to do cgroup reconcile",
			depNamespacedName)
	}

	// get pod of deployment
	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList, listOpt); err != nil {
		return targetPods, fmt.Errorf("get pod for deployment %v failed, error %v", depNamespacedName, err)
	}
	// filter pods by replicaSet uid
	for i := range podList.Items {
		pod := &podList.Items[i]
		if ownerRefMatchUID(pod.OwnerReferences, targetReplicaset.UID) {
			targetPods = append(targetPods, pod)
		}
	}
	return targetPods, nil
}

func getPodsByJobInfo(c client.Client, namespace, name string) ([]*corev1.Pod, error) {
	targetPods := make([]*corev1.Pod, 0)

	// get job
	jobNamespacedName := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	job := &batchv1.Job{}

	if err := c.Get(context.TODO(), jobNamespacedName, job); errors.IsNotFound(err) {
		// job has not created
		return targetPods, nil
	} else if err != nil {
		return targetPods, fmt.Errorf("get job failed %v, error %v", jobNamespacedName, err)
	} // else err == nil

	// prepare selector
	listOpt := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{PodLabelKeyJobName: job.Name}),
	}

	// get pod of job
	podList := &corev1.PodList{}
	if err := c.List(context.TODO(), podList, listOpt); err != nil {
		return targetPods, fmt.Errorf("get pod of job %v failed, error %v", jobNamespacedName, err)
	}
	for i := range podList.Items {
		targetPods = append(targetPods, &podList.Items[i])
	}
	return targetPods, nil
}

func getDeploymentByPod(c client.Client, pod *corev1.Pod) (*appv1.Deployment, error) {
	if pod == nil || len(pod.OwnerReferences) == 0 {
		return nil, nil
	}

	var replicaSetName string
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == ownerKindReplicaSet {
			replicaSetName = owner.Name
		}
	}
	replicaSet := &appv1.ReplicaSet{}
	replicaKey := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      replicaSetName,
	}
	if err := c.Get(context.TODO(), replicaKey, replicaSet); errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get replica %v failed by pod %v, error %v", replicaKey, pod.Name, err)
	} // else err == nil

	if replicaSet == nil || len(replicaSet.OwnerReferences) == 0 {
		return nil, nil
	}

	var deploymentName string
	for _, owner := range replicaSet.OwnerReferences {
		if owner.Kind == ownerKindDeployment {
			deploymentName = owner.Name
		}
	}
	deployment := &appv1.Deployment{}
	deploymentKey := types.NamespacedName{
		Namespace: pod.Namespace,
		Name:      deploymentName,
	}
	if err := c.Get(context.TODO(), deploymentKey, deployment); errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get deployment %v failed by replica %v of pod %v, error %v", deploymentKey,
			replicaSetName, pod.Name, err)
	} // else err == nil

	return deployment, nil
}

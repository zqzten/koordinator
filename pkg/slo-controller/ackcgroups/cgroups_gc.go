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
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

const (
	recycleDanglingsIntervalSeconds = 600
)

type podRecycleCandidate struct {
	namespace string
	name      string
	nodeName  string
}

// cgroupsInfoRecycler delete cgroups info in NodeSLO when Cgroups or Pod delete
type CgroupsInfoRecycler struct {
	client.Client

	podRWLock   sync.RWMutex
	podList     []*podRecycleCandidate
	podReceived chan struct{}

	cgroupsRWLock  sync.RWMutex
	cgroupsList    []*resourcesv1alpha1.Cgroups
	cgroupReceived chan struct{}
}

func NewCgroupsInfoRecycler(c client.Client) *CgroupsInfoRecycler {
	return &CgroupsInfoRecycler{
		Client:         c,
		podList:        make([]*podRecycleCandidate, 0),
		podReceived:    make(chan struct{}, 1),
		cgroupsList:    make([]*resourcesv1alpha1.Cgroups, 0),
		cgroupReceived: make(chan struct{}, 1),
	}
}

func (r *CgroupsInfoRecycler) Start(stopCh <-chan struct{}) {
	go r.recycleLoop(stopCh)
}

func (r *CgroupsInfoRecycler) recycleLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(recycleDanglingsIntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.podReceived:
			r.recyclePods()
		case <-r.cgroupReceived:
			r.recycleCgroups()
		case <-ticker.C:
			r.recycleDanglings()
		case <-stopCh:
			klog.Infof("cgroup info recycler is exit")
			return
		}
	}
}

func (r *CgroupsInfoRecycler) AddPod(podNamespace, podName, nodeName string) {
	r.podRWLock.Lock()
	defer r.podRWLock.Unlock()
	r.podList = append(r.podList, &podRecycleCandidate{
		namespace: podNamespace,
		name:      podName,
		nodeName:  nodeName,
	})
	r.sendPodSignalUnblocked()

}

func (r *CgroupsInfoRecycler) AddCgroups(cgroups *resourcesv1alpha1.Cgroups) {
	r.cgroupsRWLock.Lock()
	defer r.cgroupsRWLock.Unlock()
	r.cgroupsList = append(r.cgroupsList, cgroups)
	r.sendCgroupSignalUnblocked()
}

func (r *CgroupsInfoRecycler) recycleCgroups() {
	// cgroups crd is updated, recycle old-new part
	candidates := r.popAllCgroups()
	failedCgroups := make([]*resourcesv1alpha1.Cgroups, 0, len(candidates))
	for _, cgroups := range candidates {
		err := r.recycleOneCgroups(cgroups)
		if err != nil {
			failedCgroups = append(failedCgroups, cgroups)
		}
	}
	if len(failedCgroups) > 0 {
		r.batchPushCgroups(failedCgroups)
		klog.Warningf("%v cgroups recycle failed in last round, retry next time", len(failedCgroups))
	}
}

func (r *CgroupsInfoRecycler) recyclePods() {
	candidates := r.popAllPods()
	failedPods := make([]*podRecycleCandidate, 0, len(candidates))
	for _, podCandidate := range candidates {
		err := r.recycleOnePod(podCandidate)
		if err != nil {
			failedPods = append(failedPods, podCandidate)
		}
	}
	if len(failedPods) > 0 {
		r.batchPushPods(failedPods)
		klog.Warningf("%v pods recycle failed in last round, retry next time", len(failedPods))
	}
}

func (r *CgroupsInfoRecycler) recycleDanglings() {
	// get all NodeSLO
	nodeSLOList := &slov1alpha1.NodeSLOList{}
	err := r.Client.List(context.TODO(), nodeSLOList)
	if err != nil {
		klog.Warningf("list node slo failed during recycle danglings")
	}

	// get all Cgroups
	cgroupsList := &resourcesv1alpha1.CgroupsList{}
	err = r.Client.List(context.TODO(), cgroupsList)
	if err != nil {
		klog.Warningf("list cgroups failed during recycle danglings")
	}
	cgroupsMap := map[string]struct{}{}
	for i := range cgroupsList.Items {
		cgroupsName := types.NamespacedName{
			Namespace: cgroupsList.Items[i].Namespace,
			Name:      cgroupsList.Items[i].Name,
		}
		cgroupsMap[cgroupsName.String()] = struct{}{}
	}

	for i := range nodeSLOList.Items {
		oldNodeCgroupsList, err := slov1alpha1.GetNodeCgroups(&nodeSLOList.Items[i].Spec)
		if err != nil {
			klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLOList.Items[i].Name, err)
			continue
		}
		needRecycleEntireCgroups := make(map[string]struct{}, 0)
		needRecyclePodCgroups := make(map[string]struct{}, 0)
		for _, oldNodeCgroups := range oldNodeCgroupsList {
			oldCgroupsName := &types.NamespacedName{
				Namespace: oldNodeCgroups.CgroupsNamespace,
				Name:      oldNodeCgroups.CgroupsName,
			}
			if _, cgroupsExist := cgroupsMap[oldCgroupsName.String()]; !cgroupsExist {
				// cgroup not exist
				needRecycleEntireCgroups[oldCgroupsName.String()] = struct{}{}
				continue
			}

			for _, oldPodCgroups := range oldNodeCgroups.PodCgroups {
				// pod not exist
				oldPodName := types.NamespacedName{
					Namespace: oldPodCgroups.PodNamespace,
					Name:      oldPodCgroups.PodName,
				}
				pod := &corev1.Pod{}
				err = r.Client.Get(context.TODO(), oldPodName, pod)
				if err != nil && errors.IsNotFound(err) {
					needRecyclePodCgroups[oldPodName.String()] = struct{}{}
					break
				} else if err != nil {
					klog.Warningf("get pod %v failed during recycle danglings", oldPodName.String())
				}
			}
		}

		if len(needRecycleEntireCgroups) == 0 && len(needRecyclePodCgroups) == 0 {
			klog.V(5).Infof("no need to recycle node slo %v", nodeSLOList.Items[i].Name)
			continue
		}
		newNodeSLO := nodeSLOList.Items[i].DeepCopy()
		if len(needRecycleEntireCgroups) > 0 {
			currentCgroups, err := slov1alpha1.GetNodeCgroups(&newNodeSLO.Spec)
			if err != nil {
				klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLOList.Items[i].Name, err)
				continue
			}
			removedNodeCgroups, _ := removeEntireCgroupsIfExist(needRecycleEntireCgroups, currentCgroups)
			slov1alpha1.SetNodeCgroups(&newNodeSLO.Spec, removedNodeCgroups)
		}
		if len(needRecyclePodCgroups) > 0 {
			currentCgroups, err := slov1alpha1.GetNodeCgroups(&newNodeSLO.Spec)
			if err != nil {
				klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLOList.Items[i].Name, err)
				continue
			}
			removedNodeCgroups, _ := removePodCgroupsFromNodeIfExist(needRecyclePodCgroups, currentCgroups)
			slov1alpha1.SetNodeCgroups(&newNodeSLO.Spec, removedNodeCgroups)
		}
		err = r.Client.Update(context.TODO(), newNodeSLO)
		if err != nil {
			detailStr, _ := json.Marshal(newNodeSLO.Spec.Extensions)
			klog.Warningf("update node slo %v failed during recycle danglings, detail %v, error %v",
				newNodeSLO.Name, string(detailStr), err)
		} else {
			klog.Infof("update node slo %v succeed during recycle danglings", newNodeSLO.Name)
		}
	} // end for NodeSLOList.Items
}

func (r *CgroupsInfoRecycler) sendPodSignalUnblocked() {
	select {
	case r.podReceived <- struct{}{}:
	default:
	}
}

func (r *CgroupsInfoRecycler) sendCgroupSignalUnblocked() {
	select {
	case r.cgroupReceived <- struct{}{}:
	default:
	}
}

func (r *CgroupsInfoRecycler) batchPushPods(newPods []*podRecycleCandidate) {
	r.podRWLock.Lock()
	defer r.podRWLock.Unlock()
	r.podList = append(r.podList, newPods...)
	r.sendPodSignalUnblocked()
}

func (r *CgroupsInfoRecycler) batchPushCgroups(newCgroups []*resourcesv1alpha1.Cgroups) {
	r.cgroupsRWLock.Lock()
	defer r.cgroupsRWLock.Unlock()
	r.cgroupsList = append(r.cgroupsList, newCgroups...)
	r.sendCgroupSignalUnblocked()
}

func (r *CgroupsInfoRecycler) popAllPods() []*podRecycleCandidate {
	r.podRWLock.Lock()
	defer r.podRWLock.Unlock()
	output := r.podList
	r.podList = make([]*podRecycleCandidate, 0)
	return output
}

func (r *CgroupsInfoRecycler) popAllCgroups() []*resourcesv1alpha1.Cgroups {
	r.cgroupsRWLock.Lock()
	defer r.cgroupsRWLock.Unlock()
	output := r.cgroupsList
	r.cgroupsList = make([]*resourcesv1alpha1.Cgroups, 0)
	return output
}

func (r *CgroupsInfoRecycler) recycleOnePod(pod *podRecycleCandidate) error {
	if pod == nil {
		return nil
	}
	nodeSLO := &slov1alpha1.NodeSLO{}
	err := r.Client.Get(context.TODO(), types.NamespacedName{Name: pod.nodeName}, nodeSLO)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.V(5).Infof("no need to recycle cgroups since node slo %v not exist", pod.nodeName)
			return nil
		} else {
			klog.Warningf("get node slo %v failed during recycle pod, error %v", pod.nodeName, err)
			return err
		}
	}

	currentNodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLO.Spec)
	if err != nil {
		klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLO.Name, err)
		return nil
	}
	newNodeCgroups, exist := removeOnePodCgroupFromNodeIfExist(pod.namespace, pod.name, currentNodeCgroups)
	if !exist {
		klog.V(5).Infof("no need to recycle cgroups since pod %v/%v not exist in node slo %v",
			pod.namespace, pod.name, pod.nodeName)
		return nil
	}
	newNodeSLO := nodeSLO.DeepCopy()
	slov1alpha1.SetNodeCgroups(&newNodeSLO.Spec, newNodeCgroups)
	err = r.Client.Update(context.TODO(), newNodeSLO)

	if err != nil {
		detailStr, _ := json.Marshal(newNodeSLO.Spec.Extensions)
		klog.Warningf("update node slo %v failed during recycle pod, detail %v, error %v",
			newNodeSLO.Name, string(detailStr), err)
		return err
	}
	klog.Infof("update node slo %v succeed during recycle pod %v/%v", nodeSLO.Name, pod.namespace, pod.name)
	return nil
}

func (r *CgroupsInfoRecycler) recycleOneCgroups(cgroups *resourcesv1alpha1.Cgroups) error {
	if cgroups == nil {
		return nil
	}
	// get related NodeSLO by cgroups
	nodeSLOList := &slov1alpha1.NodeSLOList{}
	err := r.Client.List(context.TODO(), nodeSLOList)
	if err != nil {
		klog.Warningf("list node slo failed during recycle cgroup %v/%v", cgroups.Namespace, cgroups.Name)
		return err
	}
	relatedNodeSLOs := make([]*slov1alpha1.NodeSLO, 0)
	for i := range nodeSLOList.Items {
		nodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLOList.Items[i].Spec)
		if err != nil {
			klog.Warningf("get node %v cgroups from nodeslo failed, error %v", nodeSLOList.Items[i].Name, err)
			continue
		}
		if containsCgroups(nodeCgroups, cgroups) {
			relatedNodeSLOs = append(relatedNodeSLOs, &nodeSLOList.Items[i])
		}
	}
	if len(relatedNodeSLOs) == 0 {
		klog.V(5).Infof("cgroups %v has no related NodeSLO", len(relatedNodeSLOs))
		return nil
	}
	// get related pod names by cgroups
	relatedPodNamespacedNames := map[string]struct{}{}
	// recycle PodInfo from Cgroup crd
	if cgroups.Spec.PodInfo.Name != "" {
		podKey := &types.NamespacedName{
			Namespace: cgroups.Spec.PodInfo.Namespace,
			Name:      cgroups.Spec.PodInfo.Name,
		}
		relatedPodNamespacedNames[podKey.String()] = struct{}{}
	}
	if cgroups.Spec.DeploymentInfo.Name != "" {
		pods, err := getPodsByDeployment(r.Client, cgroups.Spec.DeploymentInfo.Namespace,
			cgroups.Spec.DeploymentInfo.Name)
		if err != nil {
			klog.Warningf("get related pods by deployment failed during recycle cgroup %v/%v",
				cgroups.Namespace, cgroups.Name)
			return err
		}
		for _, pod := range pods {
			podKey := &types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			}
			relatedPodNamespacedNames[podKey.String()] = struct{}{}
		}
	}
	if cgroups.Spec.JobInfo.Name != "" {
		pods, err := getPodsByJobInfo(r.Client, cgroups.Spec.JobInfo.Namespace, cgroups.Spec.JobInfo.Name)
		if err != nil {
			klog.Warningf("get related pods by job failed during recycle cgroup %v/%v",
				cgroups.Namespace, cgroups.Name)
			return err
		}
		for _, pod := range pods {
			podKey := &types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			}
			relatedPodNamespacedNames[podKey.String()] = struct{}{}
		}
	}
	newNodeSLOs := make([]*slov1alpha1.NodeSLO, 0)
	for _, oldNodeSLO := range relatedNodeSLOs {
		oldNodeCgroups, err := slov1alpha1.GetNodeCgroups(&oldNodeSLO.Spec)
		if err != nil {
			klog.Warningf("get node %v cgroups from nodeslo failed, error %v", oldNodeSLO.Name, err)
			continue
		}
		newNodeCgroups, exist := removePodCgroupsFromNodeIfExist(relatedPodNamespacedNames, oldNodeCgroups)
		if exist {
			newNodeSLO := oldNodeSLO.DeepCopy()
			slov1alpha1.SetNodeCgroups(&newNodeSLO.Spec, newNodeCgroups)
			newNodeSLOs = append(newNodeSLOs, newNodeSLO)
		}
	}
	// update NodeSLO
	klog.V(5).Infof("ready to recycle %d nodeSLO", len(newNodeSLOs))
	for _, nodeSLO := range newNodeSLOs {
		err = r.Client.Update(context.TODO(), nodeSLO)
		if err != nil {
			detailStr, _ := json.Marshal(nodeSLO.Spec.Extensions)
			klog.Warningf("update node slo %v failed during handle cgroup crd delete, detail %v, error %v",
				nodeSLO.Name, string(detailStr), err)
			return err
		}
		klog.Infof("update node slo %v succeed during handle cgroup crd delete for cgroup %v/%v",
			nodeSLO.Name, cgroups.Namespace, cgroups.Name)
	}
	return nil
}

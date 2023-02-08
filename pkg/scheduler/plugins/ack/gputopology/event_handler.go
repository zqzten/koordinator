package gputopology

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/frameworkcache"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/internal/gputopology"
)

func addPod(resourceName v1.ResourceName, pod *v1.Pod, cache *GPUTopologyCache) {
	podFullName := fmt.Sprintf("%v/%v", pod.Namespace, pod.Name)
	// if pod is not requested target resources,skip it
	if !IsMyPod(pod) {
		return
	}
	// if pod is completed,skip it
	if IsCompletedPod(pod) {
		return
	}
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return
	}
	if cache.GetNodeTopologyCache(nodeName).PodIsCached(pod.UID) {
		return
	}
	allocation, err := GetPodTopologyAllocation(pod.UID, pod.Annotations)
	if err != nil {
		klog.Errorf("failed to get topology allocation from pod(%v) annotation: %v", podFullName, err)
		return
	}
	if allocation == nil {
		return
	}
	allocation.NodeName = nodeName
	allocation.SetPhase(gputopology.SchedulePhaseFinishBind)
	cache.AddPod(allocation)
	klog.V(5).Infof("Node:%v,NodeGPUTopologyCache: %v", nodeName, cache.GetNodeTopologyCache(nodeName).String(false))
	klog.V(5).Infof("succeed to add pod %v/%v to gputopology cache", pod.Namespace, pod.Name)
}

func updatePod(resourceName v1.ResourceName, oldPod *v1.Pod, newPod *v1.Pod, cache *GPUTopologyCache) {
	// if pod is not requested target resources,skip it
	if !IsMyPod(newPod) {
		return
	}
	if IsCompletedPod(newPod) {
		removePod(resourceName, newPod, cache)
		return
	}
	if !reflect.DeepEqual(oldPod.Annotations, newPod.Annotations) {
		addPod(resourceName, newPod, cache)
	}
}

func removePod(resourceName v1.ResourceName, pod *v1.Pod, cache *GPUTopologyCache) {
	// if pod is not requested target resources,skip it
	if !IsMyPod(pod) {
		return
	}
	if cache.RemovePod(pod.UID) {
		klog.V(5).Infof("succeed to remove pod %v/%v from gputopology cache", pod.Namespace, pod.Name)
	}
}

func addConfigMap(configmap *v1.ConfigMap, frameworkCache *frameworkcache.FrameworkCache) {
	if val, ok := configmap.Labels[GPUTopologyEnableLabelKey]; !ok || val != "topology" {
		return
	}
	var bandwidthMatrix [][]float32
	var gpus map[int]string
	nodeName := configmap.Data["nodeName"]
	bandwidth := configmap.Data["bandwidth"]
	devices := configmap.Data["devices"]

	if nodeName == "" {
		klog.Errorf("not found nodeName in Configmap %v/%v Data", configmap.Namespace, configmap.Name)
		return
	}

	if bandwidth == "" {
		klog.Errorf("not found bandwidth in Configmap %v/%v Data", configmap.Namespace, configmap.Name)
		return
	}

	if devices == "" {
		klog.Errorf("not found devices in Configmap %v/%v Data", configmap.Namespace, configmap.Name)
		return
	}

	err := json.Unmarshal([]byte(bandwidth), &bandwidthMatrix)
	if err != nil {
		klog.Errorf("failed to unmarshal bandwidth from configmap %v/%v: %v", configmap.Namespace, configmap.Name, err)
		return
	}

	err = json.Unmarshal([]byte(devices), &gpus)
	if err != nil {
		klog.Errorf("failed to unmarshal devices from configmap %v/%v: %v", configmap.Namespace, configmap.Name, err)
		return
	}
	allGPUs := []string{}
	unhealthyGPUs := []string{}
	for id, health := range gpus {
		allGPUs = append(allGPUs, fmt.Sprintf("%v", id))
		if health != "Healthy" {
			klog.Infof("node %v gpu device %v is Unhealthy", nodeName, id)
			unhealthyGPUs = append(unhealthyGPUs, fmt.Sprintf("%v", id))
		}
	}
	gpuTopologyCache := frameworkCache.GetExtendNodeInfo(nodeName).GetGPUTopologyCache()
	if err := gpuTopologyCache.SetBandwidthMatrix(bandwidthMatrix); err != nil {
		klog.Errorf("failed to set node %v gpu topology bandwidth matrix: %v", nodeName, err)
		return
	}
	gpuTopologyCache.SetGPUs(allGPUs, unhealthyGPUs)
	klog.V(5).Infof("node: %v, GPUTopologyCache: %v", nodeName, gpuTopologyCache.String(true))
	klog.V(5).Infof("succeed to add node %v to gputopology cache", nodeName)
}

func updateConfigmap(old, cur *v1.ConfigMap, frameworkCache *frameworkcache.FrameworkCache) {
	// if configmap is not the gpu topology configmap,skip it
	if val, ok := cur.Labels[GPUTopologyEnableLabelKey]; !ok || val != "topology" {
		return
	}
	if cur.DeletionTimestamp != nil {
		removeConfigmap(cur, frameworkCache)
		return
	}
	if reflect.DeepEqual(old.Data, cur.Data) {
		return
	}
	addConfigMap(cur, frameworkCache)
}

func removeConfigmap(configmap *v1.ConfigMap, frameworkCache *frameworkcache.FrameworkCache) {
	if val, ok := configmap.Labels[GPUTopologyEnableLabelKey]; !ok || val != "topology" {
		return
	}
	nodeName := configmap.Data["nodeName"]
	if nodeName == "" {
		return
	}
	frameworkCache.GetExtendNodeInfo(nodeName).GetGPUTopologyCache().Reset()
}

func updateNode(old, cur *v1.Node, client clientset.Interface) {
	resourceName := v1.ResourceName(GPUTopologyResourceName)
	oldCapacity := GetResourceCapacity(resourceName, old)
	curCapacity := GetResourceCapacity(resourceName, cur)
	if oldCapacity != 0 && curCapacity == 0 {
		namespace := "kube-system"
		configmapName := fmt.Sprintf("%v-gputopo-report", cur.Name)
		err := client.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), configmapName, metav1.DeleteOptions{})
		if !k8serrors.IsNotFound(err) {
			klog.Errorf("failed to delete configmap %v/%v: %v", namespace, configmapName, err)
		}
		return
	}
}

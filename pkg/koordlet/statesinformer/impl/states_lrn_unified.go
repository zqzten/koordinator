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

package impl

import (
	"context"
	"encoding/json"
	"flag"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordclientset "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned"
	listerschedulingv1alpha1 "github.com/koordinator-sh/koordinator/pkg/client/listers/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metrics"
)

func init() {
	flag.DurationVar(&reportLRNInterval, "report-lrn-interval", reportLRNInterval, "The duration of reporting metrics of the LogicalResourceNodes. Default is 20s. Zero means the reporting is disabled. Non-zero values should contain a corresponding time unit (e.g. 1s, 2m, 3h).")
}

var reportLRNInterval = 20 * time.Second

const (
	lrnInformerName PluginName = "lrnInformer"
)

var _ informerPlugin = (*lrnInformer)(nil)

type lrnInformer struct {
	nodeName     string
	nodeInformer *nodeInformer
	podsInformer *podsInformer
	lrnInformer  cache.SharedIndexInformer
	lrnLister    listerschedulingv1alpha1.LogicalResourceNodeLister
}

func newLRNInformer() *lrnInformer {
	return &lrnInformer{}
}

func (l *lrnInformer) Setup(ctx *PluginOption, state *PluginState) {
	l.nodeName = ctx.NodeName
	nodeInformerIf := state.informerPlugins[nodeInformerName]
	ni, ok := nodeInformerIf.(*nodeInformer)
	if !ok {
		klog.Fatalf("node informer format error, got %T", nodeInformerIf)
	}
	l.nodeInformer = ni

	podsInformerIf := state.informerPlugins[podsInformerName]
	pi, ok := podsInformerIf.(*podsInformer)
	if !ok {
		klog.Fatalf("pods informer format error, got %T", podsInformerIf)
	}
	l.podsInformer = pi

	l.lrnInformer = newLogicalResourceNodeInformer(ctx.KoordClient, l.nodeName)
	l.lrnLister = listerschedulingv1alpha1.NewLogicalResourceNodeLister(l.lrnInformer.GetIndexer())
}

func (l *lrnInformer) Start(stopCh <-chan struct{}) {
	klog.V(2).Infof("starting lrnInformer")

	if !cache.WaitForCacheSync(stopCh, l.nodeInformer.HasSynced, l.podsInformer.HasSynced) {
		klog.Fatalf("lrnInformer timed out waiting for node and pods caches to sync")
	}

	if features.DefaultKoordletFeatureGate.Enabled(features.LRNReport) && reportLRNInterval > 0 {
		go l.lrnInformer.Run(stopCh)
		if !cache.WaitForCacheSync(stopCh, l.lrnInformer.HasSynced) {
			klog.Fatalf("lrnInformer timed out waiting for LRN cache to sync")
		}

		go wait.Until(l.syncLRN, reportLRNInterval, stopCh)
		klog.V(4).Infof("lrnInformer start to sync")
	} else {
		klog.Infof("LRN Report is disabled, feature gate %v, interval %v",
			features.DefaultKoordletFeatureGate.Enabled(features.LRNReport), reportLRNInterval.String())
	}

	klog.V(2).Infof("lrnInformer started")
}

func (l *lrnInformer) HasSynced() bool {
	if !features.DefaultKoordletFeatureGate.Enabled(features.LRNReport) {
		return true
	}
	synced := l.lrnInformer != nil && l.lrnInformer.HasSynced()
	klog.V(5).Infof("lrnInformer has synced %v", synced)
	return synced
}

func (l *lrnInformer) GetLRN(name string) *schedulingv1alpha1.LogicalResourceNode {
	lrn, err := l.lrnLister.Get(name)
	if err != nil {
		klog.Errorf("failed to get LRN %s, err: %v", name, err)
		return nil
	}
	return lrn.DeepCopy()
}

func (l *lrnInformer) syncLRN() {
	node := l.nodeInformer.GetNode()
	if node == nil || node.Status.Allocatable == nil || node.Status.Capacity == nil {
		klog.V(4).Infof("abort to sync LRN since node status is invalid, node %v", node)
		return
	}

	recordLRNNodeMetrics(node)

	lrnList, err := l.lrnLister.List(labels.SelectorFromSet(map[string]string{
		schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: l.nodeName,
	}))
	if err != nil {
		klog.Errorf("failed to list LRNs for node %s, err: %v", l.nodeName, err)
		return
	}

	if len(lrnList) <= 0 {
		klog.V(5).Infof("sync LRN skipped for node %s, no LRN against the node", node.Name)
		return
	}

	lrnMap := map[string]*schedulingv1alpha1.LogicalResourceNode{}
	for i := range lrnList {
		lrn := lrnList[i]
		lrnMap[lrn.Name] = lrn
		recordLRNMetrics(lrn)
	}
	recordLRNLabelsMetric(lrnMap)

	count := 0
	podMetas := l.podsInformer.GetAllPods()
	for i := range podMetas {
		pod := podMetas[i].Pod
		lrnName := getPodLRNName(pod)
		if len(lrnName) <= 0 {
			continue
		}

		lrn, ok := lrnMap[lrnName]
		if !ok {
			klog.V(4).Infof("failed to find lrn assigned for pod %s/%s, assigned lrn %s",
				pod.Namespace, pod.Name, lrnName)
			continue
		}
		recordLRNPodMetrics(lrn, pod)
		count++
	}
	klog.V(5).Infof("record lrn pod metrics, count %v", count)

	klog.V(4).Infof("record lrn metrics for node %v, lrn count %v", node.Name, len(lrnList))
}

func newLogicalResourceNodeInformer(client koordclientset.Interface, nodeName string) cache.SharedIndexInformer {
	selectorStr := labels.SelectorFromSet(map[string]string{
		schedulingv1alpha1.LabelNodeNameOfLogicalResourceNode: nodeName,
	}).String()
	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.LabelSelector = selectorStr
	}

	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				tweakListOptionsFunc(&options)
				return client.SchedulingV1alpha1().LogicalResourceNodes().List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionsFunc(&options)
				return client.SchedulingV1alpha1().LogicalResourceNodes().Watch(context.TODO(), options)
			},
		},
		&schedulingv1alpha1.LogicalResourceNode{},
		time.Hour*12,
		cache.Indexers{},
	)
}

func getPodLRNName(pod *corev1.Pod) string {
	if pod != nil && pod.Labels != nil {
		return pod.Labels[schedulingv1alpha1.LabelLogicalResourceNodePodAssign]
	}
	return ""
}

func getDevicesOnLRN(lrn *schedulingv1alpha1.LogicalResourceNode) *schedulingv1alpha1.LogicalResourceNodeDevices {
	if lrn == nil || lrn.Annotations == nil {
		return nil
	}

	lrnDevicesStr, ok := lrn.Annotations[schedulingv1alpha1.AnnotationLogicalResourceNodeDevices]
	if !ok || len(lrnDevicesStr) <= 0 {
		return nil
	}

	devices := &schedulingv1alpha1.LogicalResourceNodeDevices{}
	err := json.Unmarshal([]byte(lrnDevicesStr), &devices)
	if err != nil {
		klog.Errorf("failed to unmarshal devices for LRN %s, err: %v", lrn.Name, err)
		return nil
	}
	return devices
}

func recordLRNNodeMetrics(node *corev1.Node) {
	// TODO: move into upstream node informer with an option
	// record node allocatable resources especially for the node components which cannot access the ksm
	metrics.RecordNodeResourceAllocatableCPUCores(float64(node.Status.Allocatable.Cpu().MilliValue()) / 1000)
	metrics.RecordNodeResourceAllocatableMemoryTotalBytes(float64(node.Status.Allocatable.Memory().Value()))
	metrics.RecordNodeResourceCapacityCPUCores(float64(node.Status.Capacity.Cpu().MilliValue()) / 1000)
	metrics.RecordNodeResourceCapacityMemoryTotalBytes(float64(node.Status.Capacity.Memory().Value()))
	// NOTE: accelerators currently only includes `nvidia.com/gpu`
	acceleratorAllocatableValue := float64(node.Status.Allocatable.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value())
	metrics.RecordNodeResourceAllocatableAcceleratorTotal(acceleratorAllocatableValue)
	acceleratorCapacityValue := float64(node.Status.Capacity.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value())
	metrics.RecordNodeResourceCapacityAcceleratorTotal(acceleratorCapacityValue)

	klog.V(6).Infof("record lrn metrics for node %s", node.Name)
}

func recordLRNMetrics(lrn *schedulingv1alpha1.LogicalResourceNode) {
	// record node allocatable resources especially for the node components which cannot access the ksm
	if lrn == nil || lrn.Status.Allocatable == nil {
		klog.V(4).Infof("abort to record lrn metrics since lrn is invalid, lrn %v", lrn)
		return
	}
	name := lrn.Name

	// cpu, memory
	metrics.RecordLRNAllocatableCPUCores(name, float64(lrn.Status.Allocatable.Cpu().MilliValue())/1000)
	metrics.RecordLRNAllocatableMemoryTotalBytes(name, float64(lrn.Status.Allocatable.Memory().Value()))

	// accelerator
	// NOTE: accelerators currently only includes `nvidia.com/gpu`
	acceleratorValue := float64(lrn.Status.Allocatable.Name(apiext.ResourceNvidiaGPU, resource.DecimalSI).Value())
	metrics.RecordLRNAllocatableAcceleratorTotal(name, acceleratorValue)
	lrnDevices := getDevicesOnLRN(lrn)
	if lrnDevices != nil {
		for deviceType, deviceInfos := range *lrnDevices {
			for _, info := range deviceInfos {
				metrics.RecordLRNAccelerators(name, string(deviceType), strconv.FormatInt(int64(info.Minor), 10))
			}
		}
	}

	klog.V(6).Infof("record lrn metrics for lrn %s", lrn.Name)
}

func recordLRNLabelsMetric(lrnMap map[string]*schedulingv1alpha1.LogicalResourceNode) {
	metrics.ResetNodeLRNs()

	lrnLabels := map[string]string{}
	for _, lrn := range lrnMap {
		for k, v := range lrn.Labels {
			lrnLabels[k] = v
		}
	}
	metrics.RefreshNodeLRNsLabels(lrnLabels)
	klog.V(6).Infof("refresh lrn labels metrics for lrn labels %v", lrnLabels)

	for _, lrn := range lrnMap {
		metrics.RecordNodeLRNs(lrn.Name, lrn.Labels)
	}
	klog.V(5).Infof("record node_lrn metrics, lrn num %v, lrn labels %v", len(lrnMap), len(lrnLabels))
}

func recordLRNPodMetrics(lrn *schedulingv1alpha1.LogicalResourceNode, pod *corev1.Pod) {
	// record node allocatable resources especially for the node components which cannot access the ksm
	if lrn == nil || lrn.Status.Allocatable == nil {
		klog.V(4).Infof("abort to record lrn metrics since lrn is invalid, lrn %v", lrn)
		return
	}

	name := lrn.Name
	metrics.RecordLRNPods(name, pod)

	// record (regular) container metrics
	containerStatusMap := map[string]*corev1.ContainerStatus{}
	for i := range pod.Status.ContainerStatuses {
		containerStatus := &pod.Status.ContainerStatuses[i]
		containerStatusMap[containerStatus.Name] = containerStatus
	}
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		containerStatus, ok := containerStatusMap[c.Name]
		if !ok {
			klog.V(5).Infof("skip record lrn container metric, container %s/%s/%s status not exist",
				pod.Namespace, pod.Name, c.Name)
			continue
		}
		metrics.RecordLRNContainers(name, containerStatus, pod)
		klog.V(6).Infof("record lrn container metrics for lrn %s, container %s/%s/%s",
			lrn.Name, pod.Namespace, pod.Name, c.Name)
	}

	klog.V(6).Infof("record lrn pod metrics for lrn %s, pod %s/%s", lrn.Name, pod.Namespace, pod.Name)
}

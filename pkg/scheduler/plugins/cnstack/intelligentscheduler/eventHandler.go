package intelligentscheduler

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog/v2"
)

func handleAddOrUpdateVgs(cr unstructured.Unstructured, ic *intelligentCache) {
	// 原先不存在，则add；若存在，则update
	var vgs CRDs.VirtualGpuSpecification
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(cr.Object, &vgs)
	if err != nil {
		klog.Error("Failed to convert fetched virtual gpu spec to CRD")
		return
	}
	ic.addOrUpdateVgsInfo(&vgs)
}

func handleAddOrUpdateVgi(cr unstructured.Unstructured, ic *intelligentCache) {
	var vgi CRDs.VirtualGpuInstance
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(cr.Object, &vgi)
	if err != nil {
		klog.Error("Failed to convert fetched virtual gpu instance to CRD")
	}
	ic.addOrUpdateVgiInfo(&vgi)
}

func updateNodeInfoToIntelligentCache(node *corev1.Node, ic *intelligentCache, oversellRate int) {
	ic.lock.Lock()
	defer ic.lock.Unlock()
	nodeInfo, ok := ic.intelligentNodes[node.Name]
	if !ok {
		klog.Infof("NodeInfo for node %s is not exist in intelligentCache, creating", node.Name)
		nodeInfo, err := NewNodeInfo(node, oversellRate)
		if err != nil {
			klog.Errorf("Failed to create new nodeInfo for node %s, err: %v", node.Name, err)
		}
		ic.intelligentNodes[node.Name] = nodeInfo
		klog.Infof("Succeed add intelligent nodeInfo [%s] to intelligentCache", node.Name)
	} else {
		nodeInfo.Reset(node, oversellRate)
	}
}

func handleNodeAddEvent(ic *intelligentCache, newNode *corev1.Node, oversellRate int) {
	result, reason := isIntelligentNode(newNode)
	if result {
		updateNodeInfoToIntelligentCache(newNode, ic, oversellRate)
	} else {
		klog.Infof("Node [%s] is not intelligent schedule node, it will be skipped, reason: %s", newNode.Name, reason)
	}
}

func handleNodeUpdateEvent(ic *intelligentCache, oldNode *corev1.Node, newNode *corev1.Node, oversellRate int) {
	result, _ := isIntelligentNode(newNode)
	if result {
		updateNodeInfoToIntelligentCache(newNode, ic, oversellRate)
	} else {
		ic.deleteNode(oldNode)
	}
}

func handleNodeDeleteEvent(ic *intelligentCache, node *corev1.Node) {
	ic.deleteNode(node)
}

func handleAddPod(client dynamic.Interface, pod *corev1.Pod) {
	err := patchOwnerRefToConfigMap(client, pod)
	if err != nil {
		klog.Errorf("Failed to patch owner ref [pod %s/%s] to configmap: %v", pod.Namespace, pod.Name, err)
		return
	}
}

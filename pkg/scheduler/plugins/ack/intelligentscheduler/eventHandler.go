package intelligentscheduler

import (
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/ack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	//_, ok := ic.virtualGpuSpecifications[vgs.Spec.NickName]
	//if !ok {
	//	ic.virtualGpuSpecifications[vgs.Spec.NickName] = &vgs
	//} else {
	//
	//}
}

func handleAddOrUpdateVgi(cr unstructured.Unstructured, ic *intelligentCache) {
	var vgi CRDs.VirtualGpuInstance
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(cr.Object, &vgi)
	if err != nil {
		klog.Error("Failed to convert fetched virtual gpu instance to CRD")
	}
	ic.addOrUpdateVgiInfo(&vgi)
}

func handleAddOrUpdateNode(node *corev1.Node, ic *intelligentCache) {
	// check node is a device sharing node
	if !isIntelligentNode(node) {
		klog.V(6).Infof("node %v is not intelligent scheduled node,skip to handle it", node.Name)
		return
	}
	devices := getNodeGPUCount(node)
	if devices == 0 {
		return
	}
	ic.addOrUpdateNode(node)
}

func updateNode(ic *intelligentCache, oldNode *corev1.Node, newNode *corev1.Node) {
	//if isIntelligentNode(newNode) == isIntelligentNode(oldNode) {
	//	return
	//}
	if isIntelligentNode(newNode) {
		handleAddOrUpdateNode(newNode, ic)
	} else {
		if isIntelligentNode(oldNode) {
			ic.deleteNode(oldNode)
		}
	}
}

func deleteNode(ic *intelligentCache, node *corev1.Node) {
	ic.deleteNode(node)
}

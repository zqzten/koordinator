package intelligentscheduler

import (
	"context"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/cnstack/intelligentscheduler/CRDs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func handleAddOrUpdateNode(node *corev1.Node, ic *intelligentCache, oversellRate int) {
	// check node is a device sharing node
	if !isIntelligentNode(node) {
		klog.V(6).Infof("node %v is not intelligent scheduled node,skip to handle it", node.Name)
		return
	}
	devices := getNodeGPUCount(node)
	if devices == 0 {
		return
	}
	ic.addOrUpdateNode(node, oversellRate)
}

func updateNode(ic *intelligentCache, oldNode *corev1.Node, newNode *corev1.Node, oversellRate int) {
	if isIntelligentNode(newNode) {
		handleAddOrUpdateNode(newNode, ic, oversellRate)
	} else {
		if isIntelligentNode(oldNode) {
			ic.deleteNode(oldNode)
		}
	}
}

func deleteNode(ic *intelligentCache, node *corev1.Node) {
	ic.deleteNode(node)
}

func handleAddPod(client dynamic.Interface, pod *corev1.Pod) {
	err := patchOwnerRefToConfigMap(client, pod)
	if err != nil {
		klog.Errorf("Failed to patch owner ref [pod %s/%s] to configmap: %v", pod.Namespace, pod.Name, err)
		return
	}
}

func handleDeletePod(client dynamic.Interface, pod *corev1.Pod) {
	podName := pod.Name
	podNamespace := pod.Namespace
	name := podName + "/" + podNamespace
	vgiGvr := schema.GroupVersionResource{
		Group:    IntelligentGroupName,
		Version:  IntelligentVersion,
		Resource: VgiResourceName,
	}
	vgiCrs, err := client.Resource(vgiGvr).Namespace(NameSpace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to list virtual gpu instance when handle pod deletion: %v", err)
	}
	for _, vgiCr := range vgiCrs.Items {
		klog.Infof("unstructured vgi: [%v]", vgiCr)
		var vgi CRDs.VirtualGpuInstance
		er := runtime.DefaultUnstructuredConverter.FromUnstructured(vgiCr.Object, &vgi)
		if er != nil {
			klog.Errorf("Failed to convert fetched virtual gpu instance to vgi")
		}
		if vgi.Status.Pod == name {
			deleteErr := client.Resource(vgiGvr).Namespace(NameSpace).Delete(context.Background(), vgi.GetName(), metav1.DeleteOptions{})
			if deleteErr != nil {
				klog.Errorf("Failed to delete virtual gpu instance when handle pod deletion: %v", deleteErr)
			}
			klog.Infof("Deleted virtual gpu instance %v", vgi.Name)
		}
	}
}

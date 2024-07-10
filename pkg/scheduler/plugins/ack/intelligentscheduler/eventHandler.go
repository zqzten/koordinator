package intelligentscheduler

//func AddOrUpdateNode(node *corev1.Node, ic *intelligentCache) {
//	// check node is a device sharing node
//	if !isIntelligentNode(node) {
//		klog.V(6).Infof("node %v is not intelligent scheduled node,skip to handle it", node.Name)
//	}
//	devices := getNodeGPUCount(node)
//	if devices == 0 {
//		return
//	}
//	ns := ic.getNode(node.Name)
//	ns.Reset(node) //TODO 实现Reset
//	klog.Infof("succeed to add node(%v) device to node resource cache", node.Name)
//}
//
//// TODO 实现下面这个函数
//func AddOrUpdateNode(resourceNames []corev1.ResourceName, node *corev1.Node, nc *NodeCache) {
//	// check node is a device sharing node
//	if !runtime.IsMyNode(node, resourceNames...) {
//		klog.V(6).Infof("node %v has no resources %v,skip to handle it", node.Name, resourceNames)
//		return
//	}
//	devices := getNodeGPUCount(node)
//	if devices == 0 {
//		return
//	}
//	ns := nc.GetNodeState(node.Name)
//	ns.Reset(node, resourceNames)
//	klog.Infof("succeed to add node(%v) device to node resource cache", node.Name)
//}

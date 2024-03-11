package extension

import corev1 "k8s.io/api/core/v1"

type QoSClass string

const (
	// QoSLSR is the latency-sensitive-reserved type for alibaba-cloud qos class.
	QoSLSR QoSClass = "LSR"
	// QoSLS is the latency-sensitive type for alibaba-cloud qos class.
	QoSLS QoSClass = "LS"
	// QoSBE is the best effort type for alibaba-cloud qos class.
	QoSBE QoSClass = "BE"
	//QoSSystem is a alibaba-cloud qos class for daemmonset pods(Like controlPlane)
	QoSSystem QoSClass = "SYSTEM"

	// QoSNone is none.
	QoSNone QoSClass = ""
)

func GetPodQoSClass(pod *corev1.Pod) QoSClass {
	if pod == nil {
		return QoSNone
	}
	if qos, ok := pod.Annotations[AnnotationPodQOSClass]; ok {
		return GetPodQoSClassByName(qos)
	}
	// If pod does not have label, default as none-qos pod.
	return QoSNone
}

func GetPodQoSClassByName(qos string) QoSClass {
	qosClass := QoSClass(qos)
	switch qosClass {
	case QoSLSR, QoSLS, QoSBE, QoSSystem:
		return qosClass
	}
	return QoSNone
}

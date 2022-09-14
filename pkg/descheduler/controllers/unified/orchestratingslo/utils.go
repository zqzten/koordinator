package orchestratingslo

import (
	sigmaapi "gitlab.alibaba-inc.com/sigma/sigma-k8s-api/pkg/api"
	v1 "k8s.io/api/core/v1"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
)

func IsRunningAndReady(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning && podutil.IsPodReady(pod)
}

func IsWaitOnlinePod(pod *v1.Pod) bool {
	return pod.Labels[sigmaapi.LabelPodRegisterNamingState] == "wait_online"
}

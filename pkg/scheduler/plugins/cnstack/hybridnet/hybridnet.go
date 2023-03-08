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

package hybridnet

import (
	"context"
	"fmt"

	v1 "github.com/alibaba/hybridnet/pkg/apis/networking/v1"
	"github.com/alibaba/hybridnet/pkg/client/clientset/versioned"
	informer "github.com/alibaba/hybridnet/pkg/client/informers/externalversions"
	listerv1 "github.com/alibaba/hybridnet/pkg/client/listers/networking/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

const (
	Name       = "Hybridnet"
	PodNameKey = "drain.descheduler.koordinator.sh/pod-name"
	LabelPod   = "networking.alibaba.com/pod"
)

var statefulWorkloadKindSet = sets.NewString("StatefulSet")

type hybridnet struct {
	ipInstanceLister listerv1.IPInstanceLister
	networkLister    listerv1.NetworkLister
}

func HybridnetCondition(handle framework.Handle) bool {
	crdList, err := handle.ClientSet().Discovery().ServerResourcesForGroupVersion(v1.GroupVersion.String())
	if err != nil {
		klog.V(6).Infof("resources %s not found in discovery, err: %s", v1.GroupVersion, err)
		return false
	}
	return len(crdList.APIResources) > 0
}

// New initializes a new plugin and returns it.
func New(obj runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	klog.V(1).Info("hybridnet start")

	hn := &hybridnet{}
	kubeConfig := handle.KubeConfig()
	kubeConfig.ContentType = runtime.ContentTypeJSON
	kubeConfig.AcceptContentTypes = runtime.ContentTypeJSON
	client, err := versioned.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	schedSharedInformerFactory := informer.NewSharedInformerFactory(client, 0)
	hn.ipInstanceLister = schedSharedInformerFactory.Networking().V1().IPInstances().Lister()
	hn.networkLister = schedSharedInformerFactory.Networking().V1().Networks().Lister()
	schedSharedInformerFactory.Start(context.Background().Done())
	schedSharedInformerFactory.WaitForCacheSync(context.Background().Done())
	return hn, nil
}

// Name returns name of the plugin. It is used in logs, etc.
func (hn *hybridnet) Name() string {
	return Name
}

func (hn *hybridnet) Filter(ctx context.Context, state *framework.CycleState, pod *corev1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	if !reservationutil.IsReservePod(pod) {
		klog.V(3).Info("is not reserve pod")
		return framework.NewStatus(framework.Success, "")
	}
	if !ownByStatefulWorkload(pod) {
		klog.V(3).Info("reserve pod not ip retain")
		return framework.NewStatus(framework.Success, "")
	}
	if pod == nil || pod.Labels == nil || len(pod.Labels[PodNameKey]) == 0 {
		klog.V(3).Info("reserve pod could not found owner pod")
		return framework.NewStatus(framework.Success, "")
	}
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}

	var networkName string
	selector, _ := labels.Parse(fmt.Sprintf("%v=%v", LabelPod, pod.Labels[PodNameKey]))
	if list, err := hn.ipInstanceLister.IPInstances(pod.Namespace).List(selector); err == nil {
		for _, ip := range list {
			// ignore terminating ipInstance
			if ip.DeletionTimestamp == nil {
				networkName = ip.Spec.Network
			}
		}
	} else if err != nil {
		return framework.NewStatus(framework.Error, "ip instance error")
	}
	if len(networkName) == 0 {
		klog.V(3).Info("reserve pod could not found network")
		return framework.NewStatus(framework.Success, "")
	}

	var networkNodeSelector map[string]string
	if network, err := hn.networkLister.Get(networkName); err == nil {
		networkNodeSelector = network.Spec.NodeSelector
	} else if err != nil {
		return framework.NewStatus(framework.Error, "network error")
	}

	s := labels.SelectorFromSet(networkNodeSelector)
	if s.Matches(labels.Set(node.Labels)) {
		return framework.NewStatus(framework.Success, "")
	}
	return framework.NewStatus(framework.UnschedulableAndUnresolvable, "reserve pod not match ip retain")
}

func ownByStatefulWorkload(pod *corev1.Pod) bool {
	ref := metav1.GetControllerOf(pod)
	if ref == nil {
		return false
	}

	return statefulWorkloadKindSet.Has(ref.Kind)
}

//go:build !github
// +build !github

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

package statesinformer

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
)

var enableKataCollection = false
var batchAgentPort = 10029

func init() {
	flag.BoolVar(&enableKataCollection, "enable-kata-collection", enableKataCollection, "Whether you need to enable kata resource collection")
	flag.IntVar(&batchAgentPort, "batch-agent-port", batchAgentPort, "HTTP port exposed by batch-agent")
}

func (r *nodeMetricInformer) fillExtensionMap(info *slov1alpha1.PodMetricInfo, pod *corev1.Pod) {
	if enableKataCollection {
		r.fillPseudoKataExtensionMap(info, pod)
	}
}

// TODO: UT! UT! UT! We want more UT!
func (r *nodeMetricInformer) fillPseudoKataExtensionMap(info *slov1alpha1.PodMetricInfo, pod *corev1.Pod) {
	if !extunified.IsODPSPseudoKataPod(pod) {
		klog.V(6).Infof("skip non kata pod, namespace/name: %v/%v", pod.Namespace, pod.Name)
		return
	}

	klog.V(5).Infof("start fillPseudoKataExtensionMap, pod name: %v", pod.Name)

	nodeMetric, err := r.nodeMetricLister.Get(r.nodeName)
	if err != nil {
		klog.Errorf("failed to get %s nodeMetric, err: %v", r.nodeName, err)
		return
	}

	var oldPodMetric *slov1alpha1.PodMetricInfo
	for _, metric := range nodeMetric.Status.PodsMetric {
		if metric.Name == info.Name && metric.Namespace == info.Namespace {
			oldPodMetric = metric.DeepCopy()
			break
		}
	}

	if oldPodMetric != nil {
		info.Extensions = oldPodMetric.Extensions
	}

	kataResource, err := kataResourceQueryByIP()
	if err != nil || kataResource == nil {
		klog.Errorf("failed to query kata resource from batch-agent, err: %v", err)
		return
	}
	klog.V(5).Infof("query kata resource successfully, result: %+v", kataResource)

	if kataResource.VirtualResource != nil {
		resources := corev1.ResourceList{}
		for k, v := range *kataResource.VirtualResource {
			resources[corev1.ResourceName(k)] = *resource.NewQuantity(v, resource.DecimalSI)
		}
		slov1alpha1.SetVirtualResource(info, resources)
	}

	if kataResource.ReservedResource != nil {
		reserved := corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewMilliQuantity(kataResource.ReservedResource.CPU*1000, resource.DecimalSI),
			corev1.ResourceMemory: *resource.NewQuantity(int64(kataResource.ReservedResource.Memory), resource.BinarySI),
		}
		slov1alpha1.SetKataReservedResource(info, reserved)
		info.PodUsage.ResourceList = quotav1.Add(info.PodUsage.ResourceList, reserved)
	}

	klog.V(5).Infof("complete fillPseudoKataExtensionMap, pod name: %v, pod metric: %+v", pod.Name, info)
}

const kataQueryParameter = "kataResourceAggregated"

type KataResourceReserved struct {
	CPU    int64  `json:"cpu,omitempty"`
	Memory uint64 `json:"memory,omitempty"`
}

type KataVirtualResource map[string]int64

type KataAggregatedResource struct {
	ReservedResource *KataResourceReserved `json:"kataResourceReserved,omitempty"`
	VirtualResource  *KataVirtualResource  `json:"virtualResource,omitempty"`
}

type apiResponse struct {
	Status string                  `json:"status,omitempty"`
	Data   *KataAggregatedResource `json:"data,omitempty"`
}

func kataResourceQueryByIP() (*KataAggregatedResource, error) {
	batchAgentAddr := fmt.Sprintf("http://localhost:%d/api/v1/query", batchAgentPort)
	URL, err := url.Parse(batchAgentAddr)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("query", kataQueryParameter)
	URL.RawQuery = params.Encode()
	resp, err := http.Get(URL.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// example response:
	//{
	//	"status":"success",
	//	"data":{
	//		"kataResourceReserved":{
	//			"cpu":1,
	//			"memory":43088622879
	//		},
	//		"virtualResource":{
	//			"SInstance":200
	//		},
	//		"podResourceUsage":{
	//			"cpu":10,
	//			"memory":28174295776,
	//			"memoryWtichCache":28174295776,
	//			"memoryTotal":477115461632
	//		}
	//	}
	//}

	var result apiResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("error getting kata resource from batch-agent: %v", batchAgentAddr)
	}

	return result.Data, nil
}

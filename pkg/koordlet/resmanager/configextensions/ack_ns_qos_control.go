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

package configextensions

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	slov1aplhpa1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	slocontrollerconfig "github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
)

const (
	pluginName = "namespaceQOSConfigMap"
	configName = "ack-slo-pod-config"
)

const (
	greyControlFeatureMemoryQOS greyControlFeature = "memory-qos"
	greyControlFeatureCPUBurst  greyControlFeature = "cpu-burst"
)

// greyControlFeatureList defines the feature list for grey control config parsing
var greyControlFeatureList = []greyControlFeature{
	greyControlFeatureMemoryQOS,
	greyControlFeatureCPUBurst,
}

func init() {
	_ = RegisterQOSGreyCtrlPlugin(pluginName, &namespaceQOSControl{})
}

// greyControlFeature indicates the feature name in the grey control configMap, e.g. "memoryQOS"
type greyControlFeature string

type greyControlContext struct {
	ConfigMap           *corev1.ConfigMap
	NamespaceControlMap map[greyControlFeature]greyControlNamespaceList
}

type greyControlNamespaceList struct {
	whiteList map[string]struct{}
	blackList map[string]struct{}
}

// greyControlNamespaceListConfig is the struct serialized as value for each feature in grey control configMap
type greyControlNamespaceListConfig struct {
	EnabledNamespaces  []string `json:"enabledNamespaces,omitempty"`
	DisabledNamespaces []string `json:"disabledNamespaces,omitempty"`
}

type namespaceQOSControl struct {
	greyControlConfigMapInformer cache.SharedIndexInformer

	// greyControlContext stores the latest grey-control configs
	greyControlContext        greyControlContext
	greyControlContextRWMutex sync.RWMutex
}

func (c *namespaceQOSControl) Setup(kubeClient clientset.Interface) error {
	greyControlConfigMapInformer := newGreyControlConfigMapInformer(kubeClient)
	greyControlConfigMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			configMap, ok := obj.(*corev1.ConfigMap)
			if !ok {
				klog.Errorf("unable to convert object to *corev1.ConfigMap %T", obj)
				return
			}
			c.updateGreyControlContext(configMap)
			klog.V(4).Infof("create grey-control configMap %v", configMap)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			_, oldOK := oldObj.(*corev1.ConfigMap)
			newConfigMap, newOK := newObj.(*corev1.ConfigMap)
			if !oldOK || !newOK {
				klog.Errorf("unable to convert object to *corev1.ConfigMap, old %T, new %T", oldObj, newObj)
				return
			}
			updated := c.updateGreyControlContext(newConfigMap)
			if updated {
				klog.V(4).Infof("update grey-control configMap %v", newConfigMap)
			} else {
				klog.V(5).Infof("skip update grey-control configMap %v", newConfigMap)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if _, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj); err != nil {
				klog.Errorf("unable to convert deleted object %T, err: %s", obj, err)
				return
			}
			c.updateGreyControlContext(nil)
			klog.V(4).Infof("delete grey-control configMap")
		},
	})
	c.greyControlConfigMapInformer = greyControlConfigMapInformer
	return nil
}

func (c *namespaceQOSControl) Run(stopCh <-chan struct{}) {
	go c.greyControlConfigMapInformer.Run(stopCh)
}

func (c *namespaceQOSControl) InjectPodPolicy(pod *corev1.Pod, policyType QOSPolicyType, podPolicy *interface{}) (bool, error) {
	switch policyType {
	case QOSPolicyCPUBurst:
		return c.injectCPUBurst(pod, podPolicy)
	case QOSPolicyMemoryQOS:
		return c.injectMemoryQOS(pod, podPolicy)
	default:
		return false, fmt.Errorf("unknown qos policy type %v", policyType)
	}
}

func (c *namespaceQOSControl) injectCPUBurst(pod *corev1.Pod, nsConfigIf *interface{}) (bool, error) {
	if nsConfigIf == nil {
		return false, nil
	} else if _, ok := (*nsConfigIf).(*slov1aplhpa1.CPUBurstConfig); !ok {
		return false, fmt.Errorf("pod cpu burst config format illegal %v", nsConfigIf)
	}
	namespaceCfg := c.getGreyControlByFeature(greyControlFeatureCPUBurst)
	if nsCPUBurstCfg, exist := getBurstPolicyByNamespace(pod.Namespace, namespaceCfg); exist {
		klog.V(5).Infof("use namespace cpu burst config for pod %s/%s, detail %v",
			pod.Namespace, pod.Name, *nsCPUBurstCfg)
		*nsConfigIf = nsCPUBurstCfg
		return true, nil
	}
	return false, nil
}

func (c *namespaceQOSControl) injectMemoryQOS(pod *corev1.Pod, nsPolicy *interface{}) (bool, error) {
	if nsPolicy == nil {
		return false, nil
	}
	namespaceCfg := c.getGreyControlByFeature(greyControlFeatureMemoryQOS)
	if nsMemoryQOSCfg, exist := getMemoryQOSPolicyByNamespace(pod.Namespace, namespaceCfg); exist {
		klog.V(5).Infof("use namespace cpu burst config for pod %s/%s, detail %v",
			pod.Namespace, pod.Name, *nsMemoryQOSCfg)
		*nsPolicy = nsMemoryQOSCfg
		return true, nil
	}
	return false, nil
}

func (c *namespaceQOSControl) updateGreyControlContext(cm *corev1.ConfigMap) bool {
	c.greyControlContextRWMutex.Lock()
	defer c.greyControlContextRWMutex.Unlock()
	if reflect.DeepEqual(c.greyControlContext.ConfigMap, cm) {
		return false
	}
	c.greyControlContext.ConfigMap = cm
	c.greyControlContext.NamespaceControlMap = parseGreyControlConfig(cm)
	klog.V(5).Infof("update greyControlContext: namespaceControlMap %v", c.greyControlContext.NamespaceControlMap)
	return true
}

func (c *namespaceQOSControl) getGreyControlByFeature(feature greyControlFeature) *greyControlNamespaceList {
	c.greyControlContextRWMutex.RLock()
	defer c.greyControlContextRWMutex.RUnlock()
	if c.greyControlContext.NamespaceControlMap == nil {
		return nil
	}
	if namespaceList, ok := c.greyControlContext.NamespaceControlMap[feature]; !ok {
		return nil
	} else {
		return &namespaceList
	}
}

func newGreyControlConfigMapInformer(client clientset.Interface) cache.SharedIndexInformer {
	tweakListOptionFunc := func(opt *metav1.ListOptions) {
		opt.FieldSelector = "metadata.name=" + configName
	}
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (apiruntime.Object, error) {
				tweakListOptionFunc(&options)
				return client.CoreV1().ConfigMaps(slocontrollerconfig.ConfigNameSpace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionFunc(&options)
				return client.CoreV1().ConfigMaps(slocontrollerconfig.ConfigNameSpace).Watch(context.TODO(), options)
			},
		},
		&corev1.ConfigMap{},
		time.Hour*12,
		cache.Indexers{},
	)
}

func parseGreyControlConfig(cm *corev1.ConfigMap) map[greyControlFeature]greyControlNamespaceList {
	// not set greyControlNamespaceList when the feature has no config or failed to parse namespaces, in order to hint
	// reconciliation follow nodeEnable fast
	if cm == nil || cm.Data == nil {
		klog.V(5).Info("parse grey control config and get nil data, set empty namespace list")
		return nil
	}
	isEmpty := true
	namespaceControlMap := map[greyControlFeature]greyControlNamespaceList{}
	for _, feature := range greyControlFeatureList {
		cfgStr, ok := cm.Data[string(feature)]
		if !ok {
			continue
		}
		cfg := &greyControlNamespaceListConfig{}
		err := json.Unmarshal([]byte(cfgStr), cfg)
		if err != nil {
			klog.V(4).Infof("failed to parse grey control config for feature %s, err: %s", feature, err)
			continue
		}
		nsList := greyControlNamespaceList{
			whiteList: map[string]struct{}{},
			blackList: map[string]struct{}{},
		}
		for _, ns := range cfg.EnabledNamespaces {
			nsList.whiteList[ns] = struct{}{}
			isEmpty = false
		}
		for _, ns := range cfg.DisabledNamespaces {
			nsList.blackList[ns] = struct{}{}
			isEmpty = false
		}
		namespaceControlMap[feature] = nsList
	}
	if isEmpty {
		return nil
	}
	return namespaceControlMap
}

func getBurstPolicyByNamespace(namespace string,
	namespaceCfg *greyControlNamespaceList) (*slov1aplhpa1.CPUBurstConfig, bool) {
	if namespaceCfg == nil {
		return nil, false
	}
	nsBurstCfg := slov1aplhpa1.CPUBurstConfig{}
	exist := false
	if _, ok := namespaceCfg.whiteList[namespace]; ok {
		nsBurstCfg.Policy = slov1aplhpa1.CPUBurstAuto
		exist = true
	}
	if _, ok := namespaceCfg.blackList[namespace]; ok {
		nsBurstCfg.Policy = slov1aplhpa1.CPUBurstNone
		exist = true
	}
	return &nsBurstCfg, exist
}

func getMemoryQOSPolicyByNamespace(namespace string,
	namespaceCfg *greyControlNamespaceList) (*slov1aplhpa1.PodMemoryQOSConfig, bool) {
	if namespaceCfg == nil {
		return nil, false
	}
	nsMemoryQOSCfg := slov1aplhpa1.PodMemoryQOSConfig{}
	exist := false
	if _, ok := namespaceCfg.whiteList[namespace]; ok {
		nsMemoryQOSCfg.Policy = slov1aplhpa1.PodMemoryQOSPolicyAuto
		exist = true
	}
	if _, ok := namespaceCfg.blackList[namespace]; ok {
		nsMemoryQOSCfg.Policy = slov1aplhpa1.PodMemoryQOSPolicyNone
		exist = true
	}
	return &nsMemoryQOSCfg, exist
}

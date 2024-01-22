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

package nodeslo

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
)

func init() {
	_ = RegisterNodeSLOMergedExtender(unified.CPUStableConfigKey, &CPUStablePlugin{})
}

type CPUStablePlugin struct{}

func (c *CPUStablePlugin) MergeNodeSLOExtension(oldCfgMap configuration.ExtensionCfgMap, configMap *corev1.ConfigMap, recorder record.EventRecorder) (configuration.ExtensionCfgMap, error) {
	_, err := parseCPUStableCfg(oldCfgMap)
	if err != nil {
		klog.V(4).Infof("failed to parse old CPUStableCfg, reset and use new, err: %s", err)
	}
	mergedExtCfg, err := calculateCPUStableCfg(oldCfgMap, configMap)
	if err != nil {
		klog.V(0).Infof("failed to get CPUStableCfg, err: %s", err)
		recorder.Eventf(configMap, "Warning", config.ReasonSLOConfigUnmarshalFailed, "failed to unmarshal CPUStableCfg, err: %s", err)
	}
	return mergedExtCfg, err
}

func (c *CPUStablePlugin) GetNodeSLOExtension(node *corev1.Node, cfg *configuration.ExtensionCfgMap) (string, interface{}, error) {
	oldCfg, err := parseCPUStableCfg(*cfg)
	if err != nil {
		return unified.CPUStableExtKey, nil, err
	}
	if oldCfg == nil {
		return unified.CPUStableExtKey, nil, err
	}
	ext, err := getCPUStableStrategy(node, oldCfg)
	if err != nil {
		klog.Warningf("getNodeSLOSpec(): failed to get CPUStableStrategy for node %s,error: %v", node.Name, err)
	}
	return unified.CPUStableExtKey, ext, nil
}

// getCPUStableStrategy gets CPUStable strategy from the CPUStableCfg for the node.
func getCPUStableStrategy(node *corev1.Node, cfg *unified.CPUStableCfg) (*unified.CPUStableStrategy, error) {
	nodeLabels := labels.Set(node.Labels)
	for _, nodeStrategy := range cfg.NodeStrategies {
		selector, err := metav1.LabelSelectorAsSelector(nodeStrategy.NodeSelector)
		if err != nil {
			klog.Errorf("failed to parse node selector %v, err: %v", nodeStrategy.NodeSelector, err)
			continue
		}
		if selector.Matches(nodeLabels) {
			return nodeStrategy.CPUStableStrategy.DeepCopy(), nil
		}
	}
	return cfg.ClusterStrategy.DeepCopy(), nil
}

// calculateCPUStableCfg gets extension config from configmap and return old if error happen.
func calculateCPUStableCfg(oldExtCfg configuration.ExtensionCfgMap, configMap *corev1.ConfigMap) (configuration.ExtensionCfgMap, error) {
	mergedCfgMap := *oldExtCfg.DeepCopy()
	cfgStr, ok := configMap.Data[unified.CPUStableConfigKey]
	if !ok {
		delete(mergedCfgMap.Object, unified.CPUStableExtKey)
		return mergedCfgMap, nil
	}
	cpuStableCfg := unified.CPUStableCfg{}
	if err := json.Unmarshal([]byte(cfgStr), &cpuStableCfg); err != nil {
		klog.Errorf("failed to unmarshal config %s, err: %s", unified.CPUStableConfigKey, err)
		return oldExtCfg, err
	}

	newCfg := configuration.ExtensionCfg{}

	if cpuStableCfg.ClusterStrategy != nil {
		newCfg.ClusterStrategy = cpuStableCfg.ClusterStrategy
	}

	newCfg.NodeStrategies = make([]configuration.NodeExtensionStrategy, 0)
	for _, nodeStrategy := range cpuStableCfg.NodeStrategies {
		if nodeStrategy.CPUStableStrategy != nil {
			newCfg.NodeStrategies = append(newCfg.NodeStrategies, configuration.NodeExtensionStrategy{
				NodeCfgProfile: nodeStrategy.NodeCfgProfile,
				NodeStrategy:   nodeStrategy.CPUStableStrategy,
			})
		}
	}
	if mergedCfgMap.Object == nil {
		mergedCfgMap.Object = make(map[string]configuration.ExtensionCfg)
	}
	mergedCfgMap.Object[unified.CPUStableExtKey] = newCfg

	return mergedCfgMap, nil
}

// parseCPUStableCfg parses CPUStableCfg from ExtensionCfgMap.
func parseCPUStableCfg(cfgMap configuration.ExtensionCfgMap) (*unified.CPUStableCfg, error) {
	if cfgMap.Object == nil {
		return nil, nil
	}
	extCfg, exist := cfgMap.Object[unified.CPUStableExtKey]
	if !exist {
		return nil, nil
	}

	cfg := &unified.CPUStableCfg{
		NodeStrategies: make([]unified.NodeCPUStableStrategy, 0, len(extCfg.NodeStrategies)),
	}
	if extCfg.ClusterStrategy != nil {
		clusterStrategy, ok := extCfg.ClusterStrategy.(*unified.CPUStableStrategy)
		if !ok {
			return nil, fmt.Errorf("CPUStableCfg ClusterStrategy convert failed")
		} else {
			cfg.ClusterStrategy = clusterStrategy
		}
	}

	for _, nodeStrategy := range extCfg.NodeStrategies {
		nodeCPUStableStrategy, ok := nodeStrategy.NodeStrategy.(*unified.CPUStableStrategy)
		if !ok {
			return nil, fmt.Errorf("CPUStableCfg NodeStrategy convert failed")
		}
		cfg.NodeStrategies = append(cfg.NodeStrategies, unified.NodeCPUStableStrategy{
			NodeCfgProfile:    nodeStrategy.NodeCfgProfile,
			CPUStableStrategy: nodeCPUStableStrategy,
		})
	}
	return cfg, nil
}

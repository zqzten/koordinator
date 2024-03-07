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
	_ = RegisterNodeSLOMergedExtender(unified.ACSSystemConfigKey, &ACSSystemConfigPlugin{})
}

type ACSSystemConfigPlugin struct{}

func (c *ACSSystemConfigPlugin) MergeNodeSLOExtension(oldCfgMap configuration.ExtensionCfgMap, configMap *corev1.ConfigMap, recorder record.EventRecorder) (configuration.ExtensionCfgMap, error) {
	_, err := parseACSSystemCfg(oldCfgMap)
	if err != nil {
		klog.V(4).Infof("failed to parse old ACSSystemCfg, reset and use new, err: %s", err)
	}
	mergedExtCfg, err := calculateACSSystemCfg(oldCfgMap, configMap)
	if err != nil {
		klog.V(0).Infof("failed to get ACSSystemCfg, err: %s", err)
		recorder.Eventf(configMap, "Warning", config.ReasonSLOConfigUnmarshalFailed, "failed to unmarshal ACSSystemCfg, err: %s", err)
	}
	return mergedExtCfg, err
}

func (c *ACSSystemConfigPlugin) GetNodeSLOExtension(node *corev1.Node, cfg *configuration.ExtensionCfgMap) (string, interface{}, error) {
	oldCfg, err := parseACSSystemCfg(*cfg)
	if err != nil {
		return unified.ACSSystemExtKey, nil, err
	}
	if oldCfg == nil {
		return unified.ACSSystemExtKey, nil, err
	}
	ext, err := getACSSystemStrategy(node, oldCfg)
	if err != nil {
		klog.Warningf("getNodeSLOSpec(): failed to get ACSSystemStrategy for node %s,error: %v", node.Name, err)
	}
	return unified.ACSSystemExtKey, ext, nil
}

// getACSSystemStrategy gets ACSSystem strategy from the getACSSystemCfg for the node.
func getACSSystemStrategy(node *corev1.Node, cfg *unified.ACSSystemCfg) (*unified.ACSSystemStrategy, error) {
	nodeLabels := labels.Set(node.Labels)
	for _, nodeStrategy := range cfg.NodeStrategies {
		selector, err := metav1.LabelSelectorAsSelector(nodeStrategy.NodeSelector)
		if err != nil {
			klog.Errorf("failed to parse node selector %v, err: %v", nodeStrategy.NodeSelector, err)
			continue
		}
		if selector.Matches(nodeLabels) {
			return nodeStrategy.ACSSystemStrategy.DeepCopy(), nil
		}
	}
	return cfg.ClusterStrategy.DeepCopy(), nil
}

// calculateACSSystemCfg gets extension config from configmap and return old if error happen.
func calculateACSSystemCfg(oldExtCfg configuration.ExtensionCfgMap, configMap *corev1.ConfigMap) (configuration.ExtensionCfgMap, error) {
	mergedCfgMap := *oldExtCfg.DeepCopy()
	cfgStr, ok := configMap.Data[unified.ACSSystemConfigKey]
	if !ok {
		delete(mergedCfgMap.Object, unified.ACSSystemExtKey)
		return mergedCfgMap, nil
	}
	acsSystemCfg := unified.ACSSystemCfg{}
	if err := json.Unmarshal([]byte(cfgStr), &acsSystemCfg); err != nil {
		klog.Errorf("failed to unmarshal config %s, err: %s", unified.ACSSystemConfigKey, err)
		return oldExtCfg, err
	}

	newCfg := configuration.ExtensionCfg{}

	if acsSystemCfg.ClusterStrategy != nil {
		newCfg.ClusterStrategy = acsSystemCfg.ClusterStrategy
	}

	newCfg.NodeStrategies = make([]configuration.NodeExtensionStrategy, 0)
	for _, nodeStrategy := range acsSystemCfg.NodeStrategies {
		if nodeStrategy.ACSSystemStrategy != nil {
			newCfg.NodeStrategies = append(newCfg.NodeStrategies, configuration.NodeExtensionStrategy{
				NodeCfgProfile: nodeStrategy.NodeCfgProfile,
				NodeStrategy:   nodeStrategy.ACSSystemStrategy,
			})
		}
	}
	if mergedCfgMap.Object == nil {
		mergedCfgMap.Object = make(map[string]configuration.ExtensionCfg)
	}
	mergedCfgMap.Object[unified.ACSSystemExtKey] = newCfg

	return mergedCfgMap, nil
}

// parseACSSystemCfg parses ACSSystemCfg from ExtensionCfgMap.
func parseACSSystemCfg(cfgMap configuration.ExtensionCfgMap) (*unified.ACSSystemCfg, error) {
	if cfgMap.Object == nil {
		return nil, nil
	}
	extCfg, exist := cfgMap.Object[unified.ACSSystemExtKey]
	if !exist {
		return nil, nil
	}

	cfg := &unified.ACSSystemCfg{
		NodeStrategies: make([]unified.NodeACSSystemStrategy, 0, len(extCfg.NodeStrategies)),
	}
	if extCfg.ClusterStrategy != nil {
		clusterStrategy, ok := extCfg.ClusterStrategy.(*unified.ACSSystemStrategy)
		if !ok {
			return nil, fmt.Errorf("ACSSystemCfg ClusterStrategy convert failed")
		} else {
			cfg.ClusterStrategy = clusterStrategy
		}
	}

	for _, nodeStrategy := range extCfg.NodeStrategies {
		nodeACSSystemStrategy, ok := nodeStrategy.NodeStrategy.(*unified.ACSSystemStrategy)
		if !ok {
			return nil, fmt.Errorf("ACSSystemCfg NodeStrategy convert failed")
		}
		cfg.NodeStrategies = append(cfg.NodeStrategies, unified.NodeACSSystemStrategy{
			NodeCfgProfile:    nodeStrategy.NodeCfgProfile,
			ACSSystemStrategy: nodeACSSystemStrategy,
		})
	}
	return cfg, nil
}

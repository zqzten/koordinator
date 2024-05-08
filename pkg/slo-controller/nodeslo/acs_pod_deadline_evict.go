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
	// TODO should define as MustResigter with panic if failed
	RegisterNodeSLOMergedExtender(unified.DeadlineEvictConfigKey, &PodDeadlineEvictPlugin{})
}

type PodDeadlineEvictPlugin struct {
}

func (r *PodDeadlineEvictPlugin) MergeNodeSLOExtension(oldExtCfg configuration.ExtensionCfgMap, configMap *corev1.ConfigMap, recorder record.EventRecorder) (configuration.ExtensionCfgMap, error) {
	_, err := parseDeadlineEvictCfg(oldExtCfg)
	if err != nil {
		klog.V(5).Infof("failed to parse old DeadlineEvictCfg, reset and use new, err: %s", err)
	}
	mergedExtCfg, err := calculateDeadlineEvictCfg(oldExtCfg, configMap)
	if err != nil {
		klog.V(5).Infof("failed to get DeadlineEvictCfg, err: %s", err)
		recorder.Eventf(configMap, "Warning", config.ReasonSLOConfigUnmarshalFailed, "failed to unmarshal DeadlineEvictConfig, err: %s", err)
	}
	return mergedExtCfg, err
}

func (r *PodDeadlineEvictPlugin) GetNodeSLOExtension(node *corev1.Node, cfgMap *configuration.ExtensionCfgMap) (string, interface{}, error) {
	oldCfg, err := parseDeadlineEvictCfg(*cfgMap)
	if err != nil {
		return unified.DeadlineEvictExtKey, nil, err
	}
	if oldCfg == nil {
		return unified.DeadlineEvictExtKey, nil, err
	}
	ext, err := getDeadlineEvictConfigSpec(node, oldCfg)
	if err != nil {
		klog.Warningf("getNodeSLOSpec(): failed to get DeadlineEvictConfig spec for node %s,error: %v", node.Name, err)
	}
	return unified.DeadlineEvictExtKey, ext, nil
}

func getDeadlineEvictConfigSpec(node *corev1.Node, cfg *unified.DeadlineEvictCfg) (*unified.DeadlineEvictStrategy, error) {
	nodeLabels := labels.Set(node.Labels)
	for _, nodeStrategy := range cfg.NodeStrategies {
		selector, err := metav1.LabelSelectorAsSelector(nodeStrategy.NodeSelector)
		if err != nil {
			klog.Errorf("failed to parse node selector %v, err: %v", nodeStrategy.NodeSelector, err)
			continue
		}
		if selector.Matches(nodeLabels) {
			return nodeStrategy.DeadlineEvictStrategy.DeepCopy(), nil
		}
	}
	return cfg.ClusterStrategy.DeepCopy(), nil
}

// calculateDeadlineEvictCfg get extension config from configmap and return old if error happen
func calculateDeadlineEvictCfg(oldExtCfg configuration.ExtensionCfgMap, configMap *corev1.ConfigMap) (configuration.ExtensionCfgMap, error) {
	mergedCfgMap := *oldExtCfg.DeepCopy()
	cfgStr, ok := configMap.Data[unified.DeadlineEvictConfigKey]
	if !ok {
		delete(mergedCfgMap.Object, unified.DeadlineEvictExtKey)
		return mergedCfgMap, nil
	}
	deadlineEvictCfg := unified.DeadlineEvictCfg{}
	if err := json.Unmarshal([]byte(cfgStr), &deadlineEvictCfg); err != nil {
		klog.Errorf("failed to unmarshal config %s, err: %s", unified.DeadlineEvictConfigKey, err)
		return oldExtCfg, err
	}

	newCfg := configuration.ExtensionCfg{}

	if deadlineEvictCfg.ClusterStrategy != nil {
		newCfg.ClusterStrategy = deadlineEvictCfg.ClusterStrategy
	}

	newCfg.NodeStrategies = make([]configuration.NodeExtensionStrategy, 0)
	for _, nodeStrategy := range deadlineEvictCfg.NodeStrategies {
		if nodeStrategy.DeadlineEvictStrategy != nil {
			newCfg.NodeStrategies = append(newCfg.NodeStrategies, configuration.NodeExtensionStrategy{
				NodeCfgProfile: nodeStrategy.NodeCfgProfile,
				NodeStrategy:   nodeStrategy.DeadlineEvictStrategy,
			})
		}
	}
	if mergedCfgMap.Object == nil {
		mergedCfgMap.Object = make(map[string]configuration.ExtensionCfg)
	}
	mergedCfgMap.Object[unified.DeadlineEvictExtKey] = newCfg

	return mergedCfgMap, nil
}

// get DeadlineEvictCfg from ExtensionCfgMap
func parseDeadlineEvictCfg(cfgMap configuration.ExtensionCfgMap) (*unified.DeadlineEvictCfg, error) {
	if cfgMap.Object == nil {
		return nil, nil
	}
	extCfg, exist := cfgMap.Object[unified.DeadlineEvictExtKey]
	if !exist {
		return nil, nil
	}

	cfg := &unified.DeadlineEvictCfg{
		NodeStrategies: make([]unified.NodeDeadlineEvictStrategy, 0, len(extCfg.NodeStrategies)),
	}
	if extCfg.ClusterStrategy != nil {
		clusterStrategy, ok := extCfg.ClusterStrategy.(*unified.DeadlineEvictStrategy)
		if !ok {
			return nil, fmt.Errorf("DeadlineEvictCfg ClusterStrategy convert failed")
		} else {
			cfg.ClusterStrategy = clusterStrategy
		}
	}

	for _, nodeStrategy := range extCfg.NodeStrategies {
		nodeDeadlineEvictStrategy, ok := nodeStrategy.NodeStrategy.(*unified.DeadlineEvictStrategy)
		if !ok {
			return nil, fmt.Errorf("DeadlineEvictCfg NodeStrategy convert failed")
		}
		cfg.NodeStrategies = append(cfg.NodeStrategies, unified.NodeDeadlineEvictStrategy{
			NodeCfgProfile:        nodeStrategy.NodeCfgProfile,
			DeadlineEvictStrategy: nodeDeadlineEvictStrategy,
		})
	}
	return cfg, nil
}

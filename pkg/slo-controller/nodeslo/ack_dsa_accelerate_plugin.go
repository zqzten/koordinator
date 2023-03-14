package nodeslo

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func init() {
	RegisterNodeSLOMergedExtender(ackapis.DsaAccelerateExtKey, &DsaAcceleratePlugin{})
}

type DsaAcceleratePlugin struct {
}

func (r *DsaAcceleratePlugin) MergeNodeSLOExtension(cfgMap extension.ExtensionCfgMap, configMap *corev1.ConfigMap, recorder record.EventRecorder) (extension.ExtensionCfgMap, error) {

	_, err := parseDsaAccelerateCfg(cfgMap)
	if err != nil {
		klog.V(5).Infof("failed to parse DsaAccelerateCfg, err: %s", err)
		return cfgMap, err
	}
	mergedCfg, err := calculateDsaAccelerateCfgMerged(cfgMap, configMap)
	if err != nil {
		klog.V(5).Infof("failed to get DsaAccelerate, err: %s", err)
		recorder.Eventf(configMap, "Warning", config.ReasonSLOConfigUnmarshalFailed, "failed to unmarshal DsaAccelerate, err: %s", err)
	}
	return mergedCfg, nil
}

func (r *DsaAcceleratePlugin) GetNodeSLOExtension(node *corev1.Node, cfgMap *extension.ExtensionCfgMap) (string, interface{}, error) {
	oldCfg, err := parseDsaAccelerateCfg(*cfgMap)
	if err != nil {
		return ackapis.DsaAccelerateExtKey, nil, err
	}
	if oldCfg == nil {
		return ackapis.DsaAccelerateExtKey, nil, err
	}
	ext, err := getDsaAccelerateConfigSpec(node, oldCfg)
	if err != nil {
		klog.Warningf("getNodeSLOSpec(): failed to get DsaAccelerateConfig spec for node %s,error: %v", node.Name, err)
	}
	return ackapis.DsaAccelerateExtKey, ext, nil
}

func getDsaAccelerateConfigSpec(node *corev1.Node, cfg *ackapis.DsaAccelerateCfg) (*ackapis.DsaAccelerateStrategy, error) {

	nodeLabels := labels.Set(node.Labels)
	for _, nodeStrategy := range cfg.NodeStrategies {
		selector, err := metav1.LabelSelectorAsSelector(nodeStrategy.NodeSelector)
		if err != nil {
			klog.Errorf("failed to parse node selector %v, err: %v", nodeStrategy.NodeSelector, err)
			continue
		}
		if selector.Matches(nodeLabels) {
			return nodeStrategy.DsaAccelerateStrategy.DeepCopy(), nil
		}

	}
	return cfg.ClusterStrategy.DeepCopy(), nil
}

func calculateDsaAccelerateCfgMerged(oldCfgMap extension.ExtensionCfgMap, configMap *corev1.ConfigMap) (extension.ExtensionCfgMap, error) {
	mergedCfgMap := *oldCfgMap.DeepCopy()
	cfgStr, ok := configMap.Data[ackapis.DsaAccelerateConfigKey]
	if !ok {
		delete(mergedCfgMap.Object, ackapis.DsaAccelerateExtKey)
		return mergedCfgMap, nil
	}
	dsaCfg := ackapis.DsaAccelerateCfg{}
	if err := json.Unmarshal([]byte(cfgStr), &dsaCfg); err != nil {
		klog.Errorf("failed to unmarshal config %s, err: %s", ackapis.DsaAccelerateConfigKey, err)
		return oldCfgMap, err
	}
	// merge ClusterStrategy
	clusterMerged := &ackapis.DsaAccelerateStrategy{}
	if dsaCfg.ClusterStrategy != nil {
		mergedStrategyInterface, _ := util.MergeCfg(clusterMerged, dsaCfg.ClusterStrategy)
		clusterMerged = mergedStrategyInterface.(*ackapis.DsaAccelerateStrategy)
	}
	newCfg := extension.ExtensionCfg{}
	newCfg.ClusterStrategy = clusterMerged

	for index, nodeStrategy := range dsaCfg.NodeStrategies {
		// merge with clusterStrategy
		clusterCfgCopy := clusterMerged.DeepCopy()
		if nodeStrategy.DsaAccelerateStrategy != nil {
			mergedStrategyInterface, _ := util.MergeCfg(clusterCfgCopy, nodeStrategy.DsaAccelerateStrategy)
			newCfg.NodeStrategies[index].NodeStrategy = mergedStrategyInterface.(*ackapis.DsaAccelerateStrategy)
		} else {
			newCfg.NodeStrategies[index].NodeStrategy = clusterCfgCopy
		}
	}
	if mergedCfgMap.Object == nil {
		mergedCfgMap.Object = make(map[string]extension.ExtensionCfg)
	}
	mergedCfgMap.Object[ackapis.DsaAccelerateExtKey] = newCfg

	return mergedCfgMap, nil
}

// give oldCfg no matter nil or not
func parseDsaAccelerateCfg(cfgMap extension.ExtensionCfgMap) (*ackapis.DsaAccelerateCfg, error) {
	if cfgMap.Object == nil {
		return nil, nil
	}
	strategyIf, exist := cfgMap.Object[ackapis.DsaAccelerateExtKey]
	if !exist {
		return nil, nil
	}

	strategyStr, err := json.Marshal(strategyIf)
	if err != nil {
		return nil, fmt.Errorf("DsaAccelerateCfg interface json marshal failed err %s", err.Error())
	}

	cfg := &ackapis.DsaAccelerateCfg{}
	if err := json.Unmarshal(strategyStr, cfg); err != nil {
		return nil, fmt.Errorf("DsaAccelerateCfg json unmarshal convert failed err %s", err.Error())
	}

	return cfg, nil
}

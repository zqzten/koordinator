package nodeslo

import (
	"encoding/json"
	"fmt"
	"reflect"

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
	RegisterNodeSLOMergedExtender(ackapis.MemoryLocalityExtKey, &MemoryLocalityPlugin{})
}

type MemoryLocalityPlugin struct {
}

func (r *MemoryLocalityPlugin) MergeNodeSLOExtension(cfgMap extension.ExtensionCfgMap, configMap *corev1.ConfigMap, recorder record.EventRecorder) (extension.ExtensionCfgMap, error) {
	_, err := parseMemoryLocalityCfg(cfgMap)
	if err != nil {
		klog.V(5).Infof("failed to parse MemoryLocalityCfg, err: %s", err)
		return cfgMap, err
	}
	mergedCfgMap, err := calculateMemoryLocalityCfgMerged(cfgMap, configMap)
	if err != nil {
		klog.V(5).Infof("failed to get MemoryLocality, err: %s", err)
		recorder.Eventf(configMap, "Warning", config.ReasonSLOConfigUnmarshalFailed, "failed to unmarshal MemoryLocality, err: %s", err)
	}
	return mergedCfgMap, nil
}

func (r *MemoryLocalityPlugin) GetNodeSLOExtension(node *corev1.Node, cfg *extension.ExtensionCfgMap) (string, interface{}, error) {
	oldCfg, err := parseMemoryLocalityCfg(*cfg)
	if err != nil {
		return ackapis.MemoryLocalityExtKey, nil, err
	}
	if oldCfg == nil {
		return ackapis.MemoryLocalityExtKey, nil, err
	}
	ext, err := getMemoryLocalityConfigSpec(node, oldCfg)
	if err != nil {
		klog.Warningf("getNodeSLOSpec(): failed to get MemoryLocalityConfig spec for node %s,error: %v", node.Name, err)
	}
	return ackapis.MemoryLocalityExtKey, ext, nil
}

func getMemoryLocalityConfigSpec(node *corev1.Node, cfg *ackapis.MemoryLocalityCfg) (*ackapis.MemoryLocalityStrategy, error) {

	nodeLabels := labels.Set(node.Labels)
	for _, nodeStrategy := range cfg.NodeStrategies {
		selector, err := metav1.LabelSelectorAsSelector(nodeStrategy.NodeSelector)
		if err != nil {
			klog.Errorf("failed to parse node selector %v, err: %v", nodeStrategy.NodeSelector, err)
			continue
		}
		if selector.Matches(nodeLabels) {
			return nodeStrategy.MemoryLocalityStrategy.DeepCopy(), nil
		}

	}
	return cfg.ClusterStrategy.DeepCopy(), nil
}

func calculateMemoryLocalityCfgMerged(oldCfgMap extension.ExtensionCfgMap, configMap *corev1.ConfigMap) (extension.ExtensionCfgMap, error) {
	mergedCfgMap := *oldCfgMap.DeepCopy()
	cfgStr, ok := configMap.Data[extension.ResourceQOSConfigKey]
	if !ok {
		delete(mergedCfgMap.Object, ackapis.MemoryLocalityExtKey)
		return mergedCfgMap, nil
	}
	mlCfg := ackapis.MemoryLocalityCfg{}
	if err := json.Unmarshal([]byte(cfgStr), &mlCfg); err != nil {
		klog.Errorf("failed to unmarshal config %s, err: %s", ackapis.MemoryLocalityConfigKey, err)
		return oldCfgMap, err
	}

	empty := true
	if isMemoryLocalityCfgEmpty(mlCfg.ClusterStrategy) {
		if len(mlCfg.NodeStrategies) != 0 {
			for _, nodeStrategy := range mlCfg.NodeStrategies {
				ret := isMemoryLocalityCfgEmpty(nodeStrategy.MemoryLocalityStrategy)
				empty = ret && empty
			}
		} else {
			empty = true
		}
	} else {
		empty = false
	}
	if empty {
		delete(mergedCfgMap.Object, ackapis.MemoryLocalityExtKey)
		return mergedCfgMap, nil
	}

	// merge ClusterStrategy
	clusterMerged := &ackapis.MemoryLocalityStrategy{}
	if mlCfg.ClusterStrategy != nil {
		mergedStrategyInterface, _ := util.MergeCfg(clusterMerged, mlCfg.ClusterStrategy)
		clusterMerged = mergedStrategyInterface.(*ackapis.MemoryLocalityStrategy)
	}
	newCfg := extension.ExtensionCfg{}
	newCfg.ClusterStrategy = clusterMerged

	for index, nodeStrategy := range mlCfg.NodeStrategies {
		// merge with clusterStrategy
		clusterCfgCopy := clusterMerged.DeepCopy()
		if nodeStrategy.MemoryLocalityStrategy != nil {
			mergedStrategyInterface, _ := util.MergeCfg(clusterCfgCopy, nodeStrategy.MemoryLocalityStrategy)
			newCfg.NodeStrategies[index].NodeStrategy = mergedStrategyInterface.(*ackapis.MemoryLocalityStrategy)
		} else {
			newCfg.NodeStrategies[index].NodeStrategy = clusterCfgCopy
		}
	}
	if mergedCfgMap.Object == nil {
		mergedCfgMap.Object = make(map[string]extension.ExtensionCfg)
	}
	mergedCfgMap.Object[ackapis.MemoryLocalityExtKey] = newCfg

	return mergedCfgMap, nil
}

// give oldCfg no matter nil or not
func parseMemoryLocalityCfg(cfgMap extension.ExtensionCfgMap) (*ackapis.MemoryLocalityCfg, error) {
	if cfgMap.Object == nil {
		return nil, nil
	}
	strategyIf, exist := cfgMap.Object[ackapis.MemoryLocalityExtKey]
	if !exist {
		return nil, nil
	}

	strategyStr, err := json.Marshal(strategyIf)
	if err != nil {
		return nil, fmt.Errorf("MemoryLocalityCfg interface json marshal failed")
	}

	cfg := &ackapis.MemoryLocalityCfg{}
	if err := json.Unmarshal(strategyStr, cfg); err != nil {
		return nil, fmt.Errorf("MemoryLocalityCfg json unmarshal convert failed")
	}

	return cfg, nil
}

func isMemoryLocalityClassCfgEmpty(qosCfg *ackapis.MemoryLocalityQOS) bool {
	if qosCfg == nil {
		return true
	}
	if qosCfg.MemoryLocality == nil || reflect.DeepEqual(*qosCfg.MemoryLocality, ackapis.MemoryLocality{}) {
		qosCfg = nil
		return true
	}
	return false
}

// run when only resource qos is set
func isMemoryLocalityCfgEmpty(cfg *ackapis.MemoryLocalityStrategy) bool {
	if cfg == nil {
		return true
	}
	if reflect.DeepEqual(*cfg, ackapis.MemoryLocalityStrategy{}) ||
		(isMemoryLocalityClassCfgEmpty(cfg.LSRClass) &&
			isMemoryLocalityClassCfgEmpty(cfg.LSClass) &&
			isMemoryLocalityClassCfgEmpty(cfg.BEClass) &&
			isMemoryLocalityClassCfgEmpty(cfg.SystemClass) &&
			isMemoryLocalityClassCfgEmpty(cfg.CgroupRoot)) {
		return true
	}
	return false
}

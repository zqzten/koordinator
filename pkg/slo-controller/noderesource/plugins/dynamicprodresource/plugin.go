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

package dynamicprodresource

import (
	"context"
	"fmt"
	"strconv"
	"time"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/klog/v2"
	clocks "k8s.io/utils/clock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/koordinator-sh/koordinator/apis/configuration"
	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/unified"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/config"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/metrics"
	"github.com/koordinator-sh/koordinator/pkg/slo-controller/noderesource/framework"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const PluginName = "DynamicProdResource"

var (
	// ResourceNames are the resource names which the plugin need to check.
	ResourceNames = []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
	// Labels are the node labels which the plugin need to check and update.
	// TBD: May support individual labels when the prod allocatable is scaled with webhook.
	Labels = []string{
		extunified.LabelCPUOverQuota,
		extunified.LabelMemoryOverQuota,
	}
)

var Clock clocks.WithTickerAndDelayedExecution = clocks.RealClock{} // for testing

var Client client.Client

type Plugin struct{}

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Setup(opt *framework.Option) error {
	Client = opt.Client
	return nil
}

func (p *Plugin) NeedSyncMeta(strategy *configuration.ColocationStrategy, oldNode, newNode *corev1.Node) (bool, string) {
	// NOTE: Skip for VK nodes since the dynamic prod overcommitment does not support VK.
	if uniext.IsVirtualKubeletNode(newNode) {
		return false, ""
	}

	cfg, err := config.ParseDynamicProdResourceConfig(strategy)
	// config parse error
	if err != nil {
		return false, fmt.Sprintf("failed to parse prod overcommit strategy, err: %s", err)
	}
	// merge with node-level config if exists
	cfg = getNodeDynamicProdResourceConfig(cfg, newNode)

	// policy disables updates
	if policy := *cfg.ProdOvercommitPolicy; len(policy) <= 0 || policy == extunified.ProdOvercommitPolicyNone ||
		policy == extunified.ProdOvercommitPolicyDryRun {
		klog.V(5).Infof("prod overcommit policy of node %s is %s, skip sync node meta", newNode.Name, policy)
		return false, ""
	}

	// over-quota prod resource diff is bigger than ResourceDiffThreshold
	oldAllocatable := getAllocatableWithOverQuota(oldNode)
	newAllocatable := getAllocatableWithOverQuota(newNode)
	for _, resourceName := range ResourceNames {
		if util.IsResourceDiff(oldAllocatable, newAllocatable, resourceName, *strategy.ResourceDiffThreshold) {
			klog.V(4).Infof("node %v resource with over-quota %v diff bigger than %v, need sync meta",
				newNode.Name, resourceName, *strategy.ResourceDiffThreshold)
			return true, fmt.Sprintf("prod %s over-quota resource diff is big than threshold", resourceName)
		}
	}

	return false, ""
}

func (p *Plugin) Execute(strategy *configuration.ColocationStrategy, node *corev1.Node, nr *framework.NodeResource) error {
	// TODO: rename the NodePreparePlugin method to Prepare instead of Execute
	return p.Prepare(strategy, node, nr)
}

func (p *Plugin) Prepare(strategy *configuration.ColocationStrategy, node *corev1.Node, nr *framework.NodeResource) error {
	// NOTE: Skip for VK nodes since the dynamic prod overcommitment does not support VK.
	if uniext.IsVirtualKubeletNode(node) {
		return nil
	}

	cfg, err := config.ParseDynamicProdResourceConfig(strategy)
	if err != nil {
		klog.V(4).Infof("failed to prepare prod overcommit resource for node %s, parse config failed, err: %s", node.Name, err)
		return err
	}
	// merge with node-level config if exists
	cfg = getNodeDynamicProdResourceConfig(cfg, node)

	switch policy := *cfg.ProdOvercommitPolicy; policy {
	case extunified.ProdOvercommitPolicyNone:
		klog.V(5).Infof("prod overcommit policy of node %s is %s, skip updating", node.Name, policy)
		return nil
	case extunified.ProdOvercommitPolicyDryRun:
		cpuRatio, memRatio := getOverQuotaFromNodeResource(nr)
		// record metrics and logs
		metrics.RecordNodeProdResourceEstimatedOvercommitRatio(node, string(corev1.ResourceCPU), strToFloat64(cpuRatio))
		metrics.RecordNodeProdResourceEstimatedOvercommitRatio(node, string(corev1.ResourceMemory), strToFloat64(memRatio))
		klog.V(4).Infof("prod overcommit policy of node %s is %s, skip updating, over-quota ratio [cpu:%s, memory:%s]",
			node.Name, policy, cpuRatio, memRatio)
		return nil
	case extunified.ProdOvercommitPolicyStatic, extunified.ProdOvercommitPolicyAuto:
		// record metrics and logs before updating node
		cpuRatio, memRatio := getOverQuotaFromNodeResource(nr)
		metrics.RecordNodeProdResourceEstimatedOvercommitRatio(node, string(corev1.ResourceCPU), strToFloat64(cpuRatio))
		metrics.RecordNodeProdResourceEstimatedOvercommitRatio(node, string(corev1.ResourceMemory), strToFloat64(memRatio))

		if err = prepareNodeOverQuota(node, nr); err != nil {
			klog.V(4).Infof("failed to prepare prod overcommit resource for node %s, policy %s, err: %s",
				node.Name, policy, err)
			return err
		}

		klog.V(5).Infof("prod overcommit policy of node %s is %s, cpu is [%s], memory is [%s]",
			node.Name, policy, nr.Labels[extunified.LabelCPUOverQuota], nr.Labels[extunified.LabelMemoryOverQuota])

		err = p.disableKubeNodeLabelManagement(node)
		if err != nil {
			klog.ErrorS(err, "failed to disable kube node label for node", "node", node.Name)
			return err
		}
	default:
		klog.V(4).Infof("prod overcommit policy of node %s is unsupported [%s], abort updating", node.Name, policy)
		return nil
	}

	return nil
}

func (p *Plugin) Reset(node *corev1.Node, message string) []framework.ResourceItem {
	// TBD: The Reset is called when the colocation is disabled. Currently the plugin just do nothing and keep the
	//  node unchanged. It is not supported if the overcommit prod resource need to restore to the initial state.
	return nil
}

// Calculate calculates Prod resources according to the overcommit policy.
func (p *Plugin) Calculate(strategy *configuration.ColocationStrategy, node *corev1.Node, podList *corev1.PodList,
	metrics *framework.ResourceMetrics) ([]framework.ResourceItem, error) {
	// NOTE: Skip for VK nodes since the dynamic prod overcommitment does not support VK.
	if uniext.IsVirtualKubeletNode(node) {
		return nil, nil
	}

	cfg, err := config.ParseDynamicProdResourceConfig(strategy)
	if err != nil {
		klog.V(4).Infof("failed to calculate prod overcommit resource for node %s, parse config failed, err: %s", node.Name, err)
		return nil, err
	}
	// merge with node-level config if exists
	cfg = getNodeDynamicProdResourceConfig(cfg, node)

	if cfg.ProdOvercommitPolicy == nil || *cfg.ProdOvercommitPolicy == extunified.ProdOvercommitPolicyNone {
		klog.V(5).Infof("prod overcommit policy of node %s is none, skip resource calculating", node.Name)
		return nil, nil
	}

	// if the policy is static, set the default percents
	if *cfg.ProdOvercommitPolicy == extunified.ProdOvercommitPolicyStatic {
		return p.setDefault(cfg, node)
	}

	// otherwise, policy should be auto or dryRun, which need calculation.
	// if the node metric is abnormal, do degraded calculation
	if p.isDegradeNeeded(strategy, metrics.NodeMetric) {
		return p.degradeCalculate(cfg, node)
	}

	return p.calculate(cfg, node, podList, metrics)
}

func (p *Plugin) isDegradeNeeded(strategy *configuration.ColocationStrategy, nodeMetric *slov1alpha1.NodeMetric) bool {
	if nodeMetric == nil || nodeMetric.Status.UpdateTime == nil {
		klog.V(4).Infof("invalid NodeMetric: %v for prod overcommit resource, need degradation", nodeMetric)
		return true
	}

	if nodeMetric.Status.ProdReclaimableMetric == nil ||
		nodeMetric.Status.ProdReclaimableMetric.Resource.ResourceList == nil {
		klog.V(4).Infof("node %s need degradation for prod overcommit resource, err: prod reclaimable is invalid, %v",
			nodeMetric.Name, nodeMetric.Status.ProdReclaimableMetric)
		return true
	}

	now := Clock.Now()
	// TBD: may use individual degrade period for prod overcommitment
	if now.After(nodeMetric.Status.UpdateTime.Add(time.Duration(*strategy.DegradeTimeMinutes) * time.Minute)) {
		klog.V(4).Infof("node %s need degradation for prod overcommit resource, err: the last update of NodeMetric period is %v",
			nodeMetric.Name, now.Sub(nodeMetric.Status.UpdateTime.Time).String())
		return true
	}

	return false
}

func (p *Plugin) disableKubeNodeLabelManagement(node *corev1.Node) error {
	objName := fmt.Sprintf("machine-%s", node.Name)
	obj := &unified.Machine{}
	err := Client.Get(context.TODO(), types.NamespacedName{Name: objName}, obj)
	if err != nil {
		if errors.IsNotFound(err) {
			// skip when the Machine CR is missing or the CRD is not registered
			klog.V(5).Infof("node %s has no Machine obj or no Machine CRD, skip updating machine, err: %s",
				node.Name, err)
			return nil
		}
		return fmt.Errorf("cannot get machine obj, err: %w", err)
	}

	ls := obj.Spec.LabelSpec.Labels
	if ls == nil {
		klog.V(5).Infof("machine %s has no label spec, skip updating", objName)
		return nil
	}

	needUpdate := false
	for _, label := range Labels {
		v, ok := ls[label]
		if ok {
			needUpdate = true
			klog.V(5).Infof("machine %s has label %s specified, old value %s, need to disable it", objName, v)
		}
	}

	if !needUpdate {
		klog.V(5).Infof("machine %s has no label spec of node over quota, skip updating", objName)
		return nil
	}

	newObj := obj.DeepCopy()
	for _, label := range Labels {
		delete(newObj.Spec.LabelSpec.Labels, label)
	}

	patch := client.MergeFrom(obj)
	if err = Client.Patch(context.Background(), newObj, patch); err != nil {
		klog.V(4).Infof("failed to patch machine for disabling the label specification",
			"node", node.Name, "machine", objName, "err", err)
		return err
	}
	klog.V(4).InfoS("successfully patch machine for disabling the label specification",
		"node", node.Name, "machine", objName)

	return nil
}

func (p *Plugin) degradeCalculate(cfg *extunified.DynamicProdResourceConfig, node *corev1.Node) ([]framework.ResourceItem, error) {
	// If degraded, update with the default overcommit ratio.
	klog.V(4).Infof("node %s need degradation, set the default", node.Name)
	return p.setDefault(cfg, node)
}

func (p *Plugin) setDefault(cfg *extunified.DynamicProdResourceConfig, node *corev1.Node) ([]framework.ResourceItem, error) {
	klog.V(4).Infof("node %s is going to set the default ratio, policy %s", node.Name, *cfg.ProdOvercommitPolicy)

	overQuotaLabels := map[string]string{}

	if cfg.ProdCPUOvercommitDefaultPercent != nil {
		if !isValidOvercommitPercent(cfg.ProdCPUOvercommitDefaultPercent, cfg.ProdCPUOvercommitMinPercent, cfg.ProdCPUOvercommitMaxPercent) {
			klog.V(5).Infof("failed to set cpu overcommit for node %s, invalid percent %v, not in [%v, %v]",
				node.Name, *cfg.ProdCPUOvercommitDefaultPercent,
				valueOfInt64PtrOrNil(cfg.ProdCPUOvercommitMinPercent), valueOfInt64PtrOrNil(cfg.ProdCPUOvercommitMaxPercent))
			return nil, fmt.Errorf("invalid cpu default percent %v", *cfg.ProdCPUOvercommitDefaultPercent)
		}

		overQuotaLabels[extunified.LabelCPUOverQuota] = percentToOverQuotaRatio(*cfg.ProdCPUOvercommitDefaultPercent)
	}

	if cfg.ProdMemoryOvercommitDefaultPercent != nil {
		if !isValidOvercommitPercent(cfg.ProdMemoryOvercommitDefaultPercent, cfg.ProdMemoryOvercommitMinPercent, cfg.ProdMemoryOvercommitMaxPercent) {
			klog.V(5).Infof("failed to set memory overcommit for node %s, invalid percent %v, not in [%v, %v]",
				node.Name, *cfg.ProdMemoryOvercommitDefaultPercent,
				valueOfInt64PtrOrNil(cfg.ProdMemoryOvercommitMinPercent), valueOfInt64PtrOrNil(cfg.ProdMemoryOvercommitMaxPercent))
			return nil, fmt.Errorf("invalid memory default percent %v", *cfg.ProdMemoryOvercommitDefaultPercent)
		}
		overQuotaLabels[extunified.LabelMemoryOverQuota] = percentToOverQuotaRatio(*cfg.ProdMemoryOvercommitDefaultPercent)
	}

	if len(overQuotaLabels) <= 0 {
		return nil, nil
	}

	return []framework.ResourceItem{
		{
			Labels: overQuotaLabels,
		},
	}, nil
}

// calculate calculates the dynamic prod resource allocatable with the formula below:
// ProdAllocatable := max(min(NodeAllocatable + ProdReclaimable, NodeAllocatable * maxRatio), NodeAllocatable * minRatio).
func (p *Plugin) calculate(cfg *extunified.DynamicProdResourceConfig, node *corev1.Node, podList *corev1.PodList,
	resourceMetrics *framework.ResourceMetrics) ([]framework.ResourceItem, error) {
	klog.V(5).Infof("calculate prod overcommitment for node %s, policy %s", node.Name, *cfg.ProdOvercommitPolicy)

	// get node cpu and memory allocatable
	nodeAllocatable := quotav1.Mask(node.Status.Allocatable, ResourceNames)
	minAllocatable := getResourceWithRatio(nodeAllocatable, cfg.ProdCPUOvercommitMinPercent, cfg.ProdMemoryOvercommitMinPercent)
	maxAllocatable := getResourceWithRatio(nodeAllocatable, cfg.ProdCPUOvercommitMaxPercent, cfg.ProdMemoryOvercommitMaxPercent)
	// ProdAllocatableEstimated := NodeAllocatable + ProdReclaimable
	prodReclaimable := resourceMetrics.NodeMetric.Status.ProdReclaimableMetric.Resource.ResourceList
	prodAllocatableEstimated := quotav1.Add(nodeAllocatable, prodReclaimable)
	// ProdAllocatable := max(min(ProdAllocatableEstimated, NodeAllocatable * maxRatio), NodeAllocatable * minRatio)
	prodAllocatable := quotav1.Max(minResourceIgnoreNotExist(prodAllocatableEstimated, maxAllocatable), minAllocatable)

	// generate results
	overQuotaLabels := map[string]string{
		extunified.LabelCPUOverQuota:    percentToOverQuotaRatio(prodAllocatable.Cpu().MilliValue() * 100 / nodeAllocatable.Cpu().MilliValue()),
		extunified.LabelMemoryOverQuota: percentToOverQuotaRatio(prodAllocatable.Memory().Value() * 100 / nodeAllocatable.Memory().Value()),
	}
	cpuMsg := fmt.Sprintf("prodAllocatable[CPU(Milli-Core)]:%v = max(min(NodeAllocatable:%v + ProdReclaimable:%v, maxAllocatable:%v), minAllocatable:%v)",
		prodAllocatable.Cpu().MilliValue(), nodeAllocatable.Cpu().MilliValue(), prodReclaimable.Cpu().MilliValue(),
		maxAllocatable.Cpu().MilliValue(), minAllocatable.Cpu().MilliValue())
	memMsg := fmt.Sprintf("prodAllocatable[Memory(GB)]:%v = max(min(NodeAllocatable:%v + ProdReclaimable:%v, maxAllocatable:%v), minAllocatable:%v)",
		prodAllocatable.Memory().ScaledValue(resource.Giga), nodeAllocatable.Memory().ScaledValue(resource.Giga), prodReclaimable.Memory().ScaledValue(resource.Giga),
		maxAllocatable.Memory().ScaledValue(resource.Giga), minAllocatable.Memory().ScaledValue(resource.Giga))

	metrics.RecordNodeProdResourceReclaimable(node, string(corev1.ResourceCPU), metrics.UnitCore, float64(prodReclaimable.Cpu().MilliValue())/1000)
	metrics.RecordNodeProdResourceReclaimable(node, string(corev1.ResourceMemory), metrics.UnitByte, float64(prodReclaimable.Memory().Value()))
	metrics.RecordNodeProdResourceEstimatedAllocatable(node, string(corev1.ResourceCPU), metrics.UnitCore, float64(prodAllocatable.Cpu().MilliValue())/1000)
	metrics.RecordNodeProdResourceEstimatedAllocatable(node, string(corev1.ResourceMemory), metrics.UnitByte, float64(prodAllocatable.Memory().Value()))
	klog.V(5).InfoS("calculate prod overcommit resource for node", "node", node.Name,
		"cpu ratio", overQuotaLabels[extunified.LabelCPUOverQuota], "cpu msg", cpuMsg,
		"memory ratio", overQuotaLabels[extunified.LabelMemoryOverQuota], "memory msg", memMsg)

	return []framework.ResourceItem{
		{
			Labels: overQuotaLabels,
		},
	}, nil
}

func getNodeDynamicProdResourceConfig(cfg *extunified.DynamicProdResourceConfig, node *corev1.Node) *extunified.DynamicProdResourceConfig {
	if node == nil || node.Annotations == nil {
		return cfg
	}

	nodeCfg, err := extunified.GetNodeDynamicProdConfig(node.Annotations)
	if err != nil {
		klog.V(4).Infof("failed to parse node dynamic prod config for node %s, err: %s", node.Name, err)
		return cfg
	}
	if nodeCfg == nil {
		return cfg
	}

	merged := cfg.DeepCopy()
	mergedIf, err := util.MergeCfg(merged, &nodeCfg.DynamicProdResourceConfig)
	if err != nil {
		klog.V(4).Infof("failed to merge node dynamic prod config for node %s, err: %s", node.Name, err)
		return cfg
	}
	return mergedIf.(*extunified.DynamicProdResourceConfig)
}

func getAllocatableWithOverQuota(node *corev1.Node) corev1.ResourceList {
	if node == nil || node.Status.Allocatable == nil {
		return nil
	}
	if node.Labels == nil {
		return node.Status.Allocatable
	}

	allocatable := node.Status.Allocatable.DeepCopy()
	cpuOverQuotaPercent, memoryOverQuotaPercent, _ := extunified.GetResourceOverQuotaSpec(node)
	if cpu, ok := allocatable[corev1.ResourceCPU]; ok {
		allocatable[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpu.MilliValue()*cpuOverQuotaPercent/100, resource.DecimalSI)
	}
	if memory, ok := allocatable[corev1.ResourceMemory]; ok {
		allocatable[corev1.ResourceMemory] = *resource.NewQuantity(memory.Value()*memoryOverQuotaPercent/100, resource.BinarySI)
	}
	return allocatable
}

func getOverQuotaFromNodeResource(nr *framework.NodeResource) (cpuRatio, memRatio string) {
	if nr == nil || nr.Labels == nil {
		return "", ""
	}
	return nr.Labels[extunified.LabelCPUOverQuota], nr.Labels[extunified.LabelMemoryOverQuota]
}

func strToFloat64(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v
}

func prepareNodeOverQuota(node *corev1.Node, nr *framework.NodeResource) error {
	if node.Labels == nil {
		node.Labels = map[string]string{}
	}
	if nr.Labels == nil {
		return nil
	}

	for _, k := range Labels {
		s := nr.Labels[k]
		if len(s) <= 0 {
			continue
		}
		if err := isValidOverQuotaRatio(s); err != nil {
			return fmt.Errorf("%s is not valid %s, err: %v", s, k, err)
		}
		node.Labels[k] = s
	}

	return nil
}

func isValidOvercommitPercent(cur, minimal, maximal *int64) bool {
	return cur != nil && (minimal == nil || *cur >= *minimal) && (maximal == nil || *cur <= *maximal)
}

func isValidOverQuotaRatio(s string) error {
	_, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return err
	}
	return nil
}

func percentToOverQuotaRatio(percent int64) string {
	// assert the maximum precision should be 2 since the argument is a percentage
	// e.g.
	// - input:125, output:"1.25"
	// - input:150, output:"1.5"
	return strconv.FormatFloat(float64(percent)/100, 'f', -1, 32)
}

// valueOfInt64PtrOrNil returns the pointer value if the given int64 pointer is not nil, and returns -1 if the pointer
// is nil.
func valueOfInt64PtrOrNil(v *int64) int64 {
	if v == nil {
		return -1
	}
	return *v
}

func getResourceWithRatio(rl corev1.ResourceList, cpuPercent, memoryPercent *int64) corev1.ResourceList {
	result := corev1.ResourceList{}
	if cpuPercent != nil {
		result[corev1.ResourceCPU] = util.MultiplyMilliQuant(*rl.Cpu(), float64(*cpuPercent)/100)
	}
	if memoryPercent != nil {
		result[corev1.ResourceMemory] = util.MultiplyQuant(*rl.Memory(), float64(*memoryPercent)/100)
	}
	return result
}

// minResourceIgnoreNotExist returns the result of Min(a, b) for each named resource, keeping non-exist resource in b.
//
// e.g. a = {"cpu": "1", "memory": "4Gi"}, b = {"cpu": "2", "memory": "2Gi", "nvidia.com/gpu": "1"}
//
//	=> {"cpu": "1", "memory": "2Gi", "nvidia.com/gpu": "1"}
func minResourceIgnoreNotExist(a, b corev1.ResourceList) corev1.ResourceList {
	result := corev1.ResourceList{}
	for key, value := range a {
		if other, found := b[key]; found {
			if value.Cmp(other) >= 0 {
				result[key] = other.DeepCopy()
				continue
			}
		}
		result[key] = value.DeepCopy()
	}
	for key, value := range b {
		if _, found := result[key]; !found {
			result[key] = value.DeepCopy()
		}
	}
	return result
}

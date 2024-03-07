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

package acssysreconcile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/audit"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	sysutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
	"github.com/koordinator-sh/koordinator/pkg/util/sloconfig"
)

const (
	ACSSystemConfigReconcileName = "ACSSystemConfigReconcile"
)

type systemConfig struct {
	reconcileInterval time.Duration
	statesInformer    statesinformer.StatesInformer
	executor          resourceexecutor.ResourceUpdateExecutor
}

var _ framework.QOSStrategy = &systemConfig{}

func New(opt *framework.Options) framework.QOSStrategy {
	return &systemConfig{
		reconcileInterval: time.Duration(60*opt.Config.ReconcileIntervalSeconds) * time.Second, // reconcile every 60 seconds by default
		statesInformer:    opt.StatesInformer,
		executor:          resourceexecutor.NewResourceUpdateExecutor(),
	}
}

func (r *systemConfig) Enabled() bool {
	return features.DefaultMutableKoordletFeatureGate.Enabled(features.ACSSystemConfig) && r.reconcileInterval > 0
}

func (r *systemConfig) Setup(context *framework.Context) {
}

func (r *systemConfig) Run(stopCh <-chan struct{}) {
	r.init(stopCh)
	go wait.Until(r.reconcile, r.reconcileInterval, stopCh)
}

func (s *systemConfig) init(stopCh <-chan struct{}) {
	s.executor.Run(stopCh)
}

func (s *systemConfig) reconcile() {
	nodeSLO := s.statesInformer.GetNodeSLO()
	acsSystemStrategy, err := getNodeACSSystemStrategy(nodeSLO)

	if err != nil || nodeSLO == nil || acsSystemStrategy == nil || acsSystemStrategy.Enable == nil {
		klog.Warningf("nodeSLO or acsSystemStrategy is nil, skip reconcile acsSystemConfig!")
		return
	}

	node := s.statesInformer.GetNode()
	if node == nil {
		klog.Warningf("acsSystemStrategy config failed, got nil node")
		return
	}

	if !*acsSystemStrategy.Enable {
		acsSystemStrategy = sloconfig.NoneACSSystemStrategy()
		klog.V(5).Infof("node %s has not enable acs system config, use default system config", node.Name)
	}

	var resources []resourceexecutor.ResourceUpdater
	resources = append(resources, caculateACSSystemConfig(acsSystemStrategy)...)

	s.executor.UpdateBatch(true, resources...)
	klog.V(5).Infof("finish to reconcile acs system config!")
}

func getNodeACSSystemStrategy(nodeSLO *slov1alpha1.NodeSLO) (*unified.ACSSystemStrategy, error) {
	if nodeSLO == nil {
		return nil, fmt.Errorf("nodeSLO is nil")
	}

	acsSystemStrategyIf, exist := nodeSLO.Spec.Extensions.Object[unified.ACSSystemExtKey]
	if !exist {
		return nil, fmt.Errorf("acs system strategy is nil")
	}
	acsSystemStrategyStr, err := json.Marshal(acsSystemStrategyIf)
	if err != nil {
		return nil, err
	}
	acsSystemStrategy := sloconfig.DefaultACSSystemStrategy()
	if err = json.Unmarshal(acsSystemStrategyStr, acsSystemStrategy); err != nil {
		return nil, err
	}

	return acsSystemStrategy, nil
}

func caculateACSSystemConfig(strategy *unified.ACSSystemStrategy) []resourceexecutor.ResourceUpdater {
	var resources []resourceexecutor.ResourceUpdater

	// TODO: validate resouces' value by updater
	if sysutil.ValidateResourceValue(strategy.SchedSchedStats, "", sysutil.SchedSchedStats) {
		if supported, msg := sysutil.SchedSchedStats.IsSupported(""); !supported {
			klog.V(5).Infof("node does not support sched_schedstats feature, msg: %s", msg)
		} else {
			valueStr := strconv.FormatInt(*strategy.SchedSchedStats, 10)
			file := sysutil.SchedSchedStats.Path("")
			eventHelper := audit.V(3).Node().Reason("acsSystemConfig reconcile").Message("update sched_schedstats config to : %v", valueStr)
			resource, err := resourceexecutor.NewCommonDefaultUpdater(file, file, valueStr, eventHelper)
			if err != nil {
				return resources
			}
			resources = append(resources, resource)
		}
	}

	if sysutil.ValidateResourceValue(strategy.SchedAcpu, "", sysutil.SchedAcpu) {
		if supported, msg := sysutil.SchedAcpu.IsSupported(""); !supported {
			klog.V(5).Infof("node does not support sched_acpu feature, msg: %s", msg)
		} else {
			valueStr := strconv.FormatInt(*strategy.SchedAcpu, 10)
			file := sysutil.SchedAcpu.Path("")
			eventHelper := audit.V(3).Node().Reason("acsSystemConfig reconcile").Message("update sched_acpu config to : %v", valueStr)
			resource, err := resourceexecutor.NewCommonDefaultUpdater(file, file, valueStr, eventHelper)
			if err != nil {
				return resources
			}
			resources = append(resources, resource)
		}
	}

	return resources
}

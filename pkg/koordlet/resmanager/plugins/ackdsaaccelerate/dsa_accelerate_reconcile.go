package ackdsa

import (
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/audit"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	dsautil "github.com/koordinator-sh/koordinator/pkg/koordlet/resmanager/plugins/ackdsaaccelerate/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
)

const (
	dsaAccelerateReconcileInterval = "1m"
)

var (
	DsaAccelerateFeatureName featuregate.Feature = "DsaAccelerate"
	DsaAccelerateFeatureSpec                     = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Alpha}
	DsaAcceleratePlugin                          = &plugin{
		config: NewDefaultConfig(),
	}
)

type plugin struct {
	metricCache    metriccache.MetricCache
	statesInformer statesinformer.StatesInformer
	resexecutor    resourceexecutor.ResourceUpdateExecutor
	config         *Config
}

type Config struct {
	DsaAccelerateReconcileInterval time.Duration
}

func NewDefaultConfig() *Config {
	duration, _ := time.ParseDuration(dsaAccelerateReconcileInterval)
	return &Config{
		DsaAccelerateReconcileInterval: duration,
	}
}

func (d *Config) InitFlags(fs *flag.FlagSet) {
	fs.DurationVar(&d.DsaAccelerateReconcileInterval, "dsa-accelerate-reconcile-interval",
		d.DsaAccelerateReconcileInterval, "reconcile dsa accelerate setting interval")
}

func (d *plugin) InitFlags(fs *flag.FlagSet) {
	d.config.InitFlags(fs)
}

func (d *plugin) Setup(client clientset.Interface, metricCache metriccache.MetricCache,
	statesInformer statesinformer.StatesInformer) {
	d.metricCache = metricCache
	d.statesInformer = statesInformer
	d.resexecutor = resourceexecutor.NewResourceUpdateExecutor()
}

func (d *plugin) Run(stopCh <-chan struct{}) {
	go wait.Until(d.reconcile, d.config.DsaAccelerateReconcileInterval, stopCh)
	d.resexecutor.Run(stopCh)
}

func (d *plugin) reconcile() {
	nodeSLO := d.statesInformer.GetNodeSLO()
	if nodeSLO == nil {
		klog.Warningf("nodeSLO is nil, skip reconcile DsaAccelerate!")
		return
	}

	if nodeSLO.Spec.Extensions == nil || nodeSLO.Spec.Extensions.Object == nil {
		return
	}

	// skip if host not support DsaAccelerate
	// only check once
	support, err := dsautil.IsSupportDsa()
	if err != nil {
		klog.Warningf("check support dsa failed, err: %v", err)
	}
	if !support {
		return
	}

	dsaCfg, err := parseDsaAccelerateSpec(nodeSLO)
	if err != nil {
		klog.Warningf("failed to get dsa config err %s, skip reconcile DsaAccelerate!", err.Error())
	}

	dsaCfg, err = dsautil.MergeDefaultDsaAccelerateConfig(dsaCfg)
	if err != nil {
		klog.Warningf("merge default dsa acclerate config err %s, skip reconcile DsaAccelerate!", err.Error())
	}

	// https://aliyuque.antfin.com/op0cg2/xggqal/st0cnh0g37qxt6p5
	if dsaCfg.Enable != nil && *dsaCfg.Enable {
		var resources []resourceexecutor.ResourceUpdater
		resources = append(resources, writeMigrateFile("1")...)
		if dsaCfg.PollingMode != nil && *dsaCfg.PollingMode {
			resources = append(resources, writeDmaMigrateModeFile("1")...)
		} else {
			resources = append(resources, writeDmaMigrateModeFile("0")...)
		}
		d.resexecutor.UpdateBatch(true, resources...)
		err := dsautil.ManageDsa(true)
		if err != nil {
			klog.Warningf("enable dsa failed, err: %v", err)
		}
		klog.V(5).Infof("already enabled dsa")
	} else {
		var resources []resourceexecutor.ResourceUpdater
		resources = append(resources, writeMigrateFile("0")...)
		d.resexecutor.UpdateBatch(true, resources...)
		err := dsautil.ManageDsa(false)
		if err != nil {
			klog.Warningf("disable dsa failed, err: %v", err)
		}
		klog.V(5).Infof("already disabled dsa")
	}
	klog.V(5).Infof("finish to reconcile dsa accelerate config!")
}

func parseDsaAccelerateSpec(nodeSLO *slov1alpha1.NodeSLO) (*ackapis.DsaAccelerateStrategy, error) {
	dsaIf, exist := nodeSLO.Spec.Extensions.Object[ackapis.DsaAccelerateExtKey]
	if !exist {
		return nil, fmt.Errorf("dsa accelerate config is nil")
	}
	dsaStr, err := json.Marshal(dsaIf)
	if err != nil {
		return nil, err
	}
	dsaCfg := &ackapis.DsaAccelerateStrategy{}
	if err := json.Unmarshal(dsaStr, dsaCfg); err != nil {
		return nil, err
	}
	return dsaCfg, nil
}

func writeMigrateFile(str string) []resourceexecutor.ResourceUpdater {
	var resources []resourceexecutor.ResourceUpdater
	eventHelper := audit.V(5).Node().Reason("DsaAccelerate reconcile").Message("update batch_migrate_enabled and dma_migrate_enabled to : %v", str)
	batchMigrate, _ := resourceexecutor.NewCommonDefaultUpdater(dsautil.GetDsaBatchMigrateEnabledPath(), dsautil.GetDsaBatchMigrateEnabledPath(), str, eventHelper)
	dmaMigrate, _ := resourceexecutor.NewCommonDefaultUpdater(dsautil.GetDsaDmaMigrateEnablePath(), dsautil.GetDsaDmaMigrateEnablePath(), str, eventHelper)
	resources = append(resources, batchMigrate, dmaMigrate)
	return resources
}

func writeDmaMigrateModeFile(str string) []resourceexecutor.ResourceUpdater {
	var resources []resourceexecutor.ResourceUpdater
	eventHelper := audit.V(5).Node().Reason("DsaAccelerate reconcile").Message("update dma_migrate_polling to : %v", str)
	migrateMode, _ := resourceexecutor.NewCommonDefaultUpdater(dsautil.GetDsaDmaMigrateModePath(), dsautil.GetDsaDmaMigrateModePath(), str, eventHelper)
	resources = append(resources, migrateMode)
	return resources
}

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

package ackcgroupcrd

import (
	"context"
	"encoding/json"
	"flag"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"

	slov1alpha1 "github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/executor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	cgroupscrdutil "github.com/koordinator-sh/koordinator/pkg/koordlet/resmanager/plugins/ackcgroupcrd/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	"github.com/koordinator-sh/koordinator/pkg/util"
	sysutil "github.com/koordinator-sh/koordinator/pkg/util/system"

	resourcesv1alpha1 "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/resources/v1alpha1"
)

const (
	cgroupsControllerConfNs   = "kube-system"
	cgroupsControllerConfName = "resource-controller-config"
)

var (
	FeatureName featuregate.Feature = "CgroupCRD"
	FeatureSpec                     = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Beta}
	Plugin                          = &plugin{
		config: NewDefaultConfig(),
	}
)

type plugin struct {
	metricCache    metriccache.MetricCache
	statesInformer statesinformer.StatesInformer
	cmInformer     cache.SharedIndexInformer
	executor       *executor.ResourceUpdateExecutor

	started               bool
	controllerConf        *cgroupscrdutil.CgroupsControllerConfig
	controllerConfRWMutex sync.RWMutex

	config *Config
}

type Config struct {
	CgroupCRDReconcileIntervalSeconds int
}

func NewDefaultConfig() *Config {
	return &Config{
		CgroupCRDReconcileIntervalSeconds: 1,
	}
}

func (c *Config) InitFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.CgroupCRDReconcileIntervalSeconds, "cgroup-crd-reconcile-interval-seconds",
		c.CgroupCRDReconcileIntervalSeconds, "reconcile cgroup crd interval by seconds")
}

func (c *plugin) InitFlags(fs *flag.FlagSet) {
	c.config.InitFlags(fs)
	return
}

func (c *plugin) Setup(client clientset.Interface, metricCache metriccache.MetricCache,
	statesInformer statesinformer.StatesInformer) {
	configMapInformer := newConfigMapInformer(client, cgroupsControllerConfNs, cgroupsControllerConfName)
	configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			configMap, ok := obj.(*corev1.ConfigMap)
			if ok {
				c.updateConfig(configMap)
				klog.Infof("create cgroup controller config %v", c.getConfig())
			} else {
				klog.Warningf("cgroups controller config map informer add func parse failed, %T", obj)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldConfigMap, oldOK := oldObj.(*corev1.ConfigMap)
			newConfigMap, newOK := newObj.(*corev1.ConfigMap)
			if !oldOK || !newOK {
				klog.Warningf("unable to convert object during cgroups controller config update, old %T, new %T",
					oldObj, newObj)
				return
			}
			if reflect.DeepEqual(oldConfigMap.Data, newConfigMap.Data) {
				klog.V(5).Infof("cgroups controller config map has not changed")
				return
			}
			c.updateConfig(newConfigMap)
			klog.Infof("update cgroups controller %v", c.getConfig())
		},
	})
	c.metricCache = metricCache
	c.statesInformer = statesInformer
	c.cmInformer = configMapInformer
	c.executor = executor.NewResourceUpdateExecutor("CgroupsCrdExecutor", c.config.CgroupCRDReconcileIntervalSeconds*60)
}

func (c *plugin) Run(stopCh <-chan struct{}) {
	go wait.Until(c.reconcile, time.Duration(c.config.CgroupCRDReconcileIntervalSeconds)*time.Second, stopCh)
}

func (c *plugin) reconcile() {
	nodeSLO := c.statesInformer.GetNodeSLO()
	if nodeSLO == nil {
		// do nothing if nodeSLO == nil
		klog.Warningf("nodeSLO is nil")
		return
	}

	if !c.started {
		ch1 := make(chan struct{})
		go c.cmInformer.Run(ch1)
		if !cache.WaitForCacheSync(ch1, c.cmInformer.HasSynced) {
			klog.Warningf("time out waiting for cgroup crd config map to sync")
		}
		if err := sysutil.CheckAndTryEnableResctrlCat(); err != nil {
			klog.Warningf("check resctrl cat failed, err: %s", err)
		}
		c.executor.Run(ch1)
		c.started = true
	}
	nodeInfo, err := c.metricCache.GetNodeCPUInfo(&metriccache.QueryParam{})
	if err != nil {
		klog.Warningf("get node info failed during update by cgroup crd, error %v", err)
		return
	}
	if nodeInfo.TotalInfo.NumberCPUs <= 0 {
		klog.Warningf("cpu number %v is illegal, detail %v", nodeInfo.TotalInfo.NumberCPUs, nodeInfo)
		return
	}

	// generate pod plan by (podMeta annotation + cgroups-controller-configmap) or (nodeSLO)
	podPlans := make([]*cgroupscrdutil.PodReconcilePlan, 0)
	config := c.getConfig()
	nodeCgroups, err := slov1alpha1.GetNodeCgroups(&nodeSLO.Spec)
	if err != nil {
		klog.Warningf("get cgroup from node slo failed, error %v", err)
		return
	}
	for _, podMeta := range c.statesInformer.GetAllPods() {
		if cgroupscrdutil.NeedReconcileByAnnotation(podMeta.Pod, config) || cgroupscrdutil.NeedReconcileByConfigmap(config) {
			// pod has specified annotation, generate plan by annotation
			podPlan := cgroupscrdutil.GeneratePlanByPod(podMeta, config, int(nodeInfo.TotalInfo.NumberCPUs))
			podPlans = append(podPlans, podPlan)
		}
		if podCgroups := cgroupscrdutil.GetPodCgroupsFromNode(podMeta.Pod, nodeCgroups); podCgroups != nil {
			// NodeSLO has PodCgroups to reconcile
			podPlan := cgroupscrdutil.GeneratePlanByCgroups(podMeta, podCgroups)
			podPlans = append(podPlans, podPlan)
		}
	}
	for _, podPlan := range podPlans {
		c.executePodPlan(podPlan)
	}
}

func newConfigMapInformer(client clientset.Interface, namespace, name string) cache.SharedIndexInformer {
	tweakListOptionsFunc := func(opt *metav1.ListOptions) {
		opt.FieldSelector = "metadata.name=" + name
	}
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				tweakListOptionsFunc(&options)
				return client.CoreV1().ConfigMaps(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				tweakListOptionsFunc(&options)
				return client.CoreV1().ConfigMaps(namespace).Watch(context.TODO(), options)
			},
		},
		&corev1.ConfigMap{},
		time.Hour*12,
		cache.Indexers{},
	)
}

func (c *plugin) getConfig() *cgroupscrdutil.CgroupsControllerConfig {
	c.controllerConfRWMutex.RLock()
	defer c.controllerConfRWMutex.RUnlock()
	if c.controllerConf != nil {
		return c.controllerConf.DeepCopy()
	}
	return nil
}

func (c *plugin) updateConfig(newConfigMap *corev1.ConfigMap) {
	cmJsonStr, err := json.Marshal(newConfigMap.Data)
	if err != nil {
		klog.Warningf("convert config map data to json failed during update cgroups controller conf, "+
			"origin data: %v, error: %v", newConfigMap.Data, err)
		return
	}
	newConf := &cgroupscrdutil.CgroupsControllerConfig{}
	err = json.Unmarshal(cmJsonStr, newConf)
	if err != nil {
		klog.Warningf("convert json to config struct failed during update cgroups controller conf, "+
			"origin string: %v, error: %v", string(cmJsonStr), err)
		return
	}

	if cpuSetMapJsonStr, ok := newConfigMap.Data[cgroupscrdutil.ConfigDefaultCpuSetKey]; ok && strings.TrimSpace(cpuSetMapJsonStr) != "" {
		confCPUSetMap := map[string]string{}
		if err := json.Unmarshal([]byte(cpuSetMapJsonStr), &confCPUSetMap); err != nil {
			klog.Warningf("convert config cpuset map data to json failed during update cgroups controller conf, "+
				"origin data: %v, error: %v", cpuSetMapJsonStr, err)
		} else {
			newConf.CPUSet = confCPUSetMap
		}
	}

	if cpuAcctMapJsonStr, ok := newConfigMap.Data[cgroupscrdutil.ConfigDefaultCpuAcctKey]; ok && strings.TrimSpace(cpuAcctMapJsonStr) != "" {
		confCPUAcctMap := map[string]string{}
		if err := json.Unmarshal([]byte(cpuAcctMapJsonStr), &confCPUAcctMap); err != nil {
			klog.Warningf("convert config cpu acct data to json failed during update cgroups controller conf, "+
				"origin data: %v, error: %v", cpuAcctMapJsonStr, err)
		} else {
			newConf.CPUAcct = confCPUAcctMap
		}
	}

	c.controllerConfRWMutex.Lock()
	defer c.controllerConfRWMutex.Unlock()
	c.controllerConf = newConf
}

func (c *plugin) executePodPlan(podPlan *cgroupscrdutil.PodReconcilePlan) {
	if podPlan == nil || podPlan.PodMeta == nil {
		return
	}

	// prepare container stat map
	containerStatMap := make(map[string]*corev1.ContainerStatus, len(podPlan.PodMeta.Pod.Status.ContainerStatuses))
	for i := range podPlan.PodMeta.Pod.Status.ContainerStatuses {
		containerStat := &podPlan.PodMeta.Pod.Status.ContainerStatuses[i]
		containerStatMap[containerStat.Name] = containerStat
	}

	podDir := util.GetPodCgroupDirWithKube(podPlan.PodMeta.CgroupDir)

	// pod cpu limit
	podCPULimitOpt, targetPodCfsQuota, err := cgroupscrdutil.GetPodCPULimitOpt(podPlan)
	if err != nil {
		klog.Warningf("prepare pod %v/%v cpu limit operation failed during cgroups crd reconcile",
			podPlan.PodMeta.Pod.Namespace, podPlan.PodMeta.Pod.Name)
	}

	// pod memory limit
	podMemoryLimitOpt, targetPodMemLimit, err := cgroupscrdutil.GetPodMemLimitOpt(podPlan)
	if err != nil {
		klog.Warningf("prepare pod %v/%v memory limit operation failed during cgroups crd reconcile",
			podPlan.PodMeta.Pod.Namespace, podPlan.PodMeta.Pod.Name)
	}

	if podCPULimitOpt == cgroupscrdutil.ResourceLimitScaleUp {
		c.execPodCfsQuota(podPlan, podDir, targetPodCfsQuota)
	}
	if podMemoryLimitOpt == cgroupscrdutil.ResourceLimitScaleUp {
		c.execPodMemLimit(podPlan, podDir, targetPodMemLimit)
	}

	containerDirLLCMap := make(map[string]*resourcesv1alpha1.LLCinfo, 0)

	for containerName, containerPlan := range podPlan.ContainerPlan {
		containerStat, exist := containerStatMap[containerName]
		if !exist || containerStat == nil {
			klog.Infof("container %v not exist in pod %v/%v status",
				containerName, podPlan.PodMeta.Pod.Namespace, podPlan.PodMeta.Pod.Name)
			continue
		}

		// generate container cgroup dir
		containerDir, err := util.GetContainerCgroupPathWithKube(podPlan.PodMeta.CgroupDir, containerStat)
		if err != nil {
			klog.Warningf("parse container %v dir of pod %v/%v failed, error %v",
				containerStat.Name, podPlan.PodMeta.Pod.Namespace, podPlan.PodMeta.Pod.Name, err)
			continue
		}
		klog.V(6).Infof("start exec container %v, dir %v plan %v", containerName, containerDir, containerPlan)

		if containerPlan.CPULimitMilli != nil {
			var targetCfsQuota int
			if *containerPlan.CPULimitMilli <= 0 { // if CPULimitMilli == -1, unset container cfs_quota
				targetCfsQuota = -1
			} else {
				targetCfsQuota = int(*containerPlan.CPULimitMilli * sysutil.CFSBasePeriodValue / 1000)
			}
			c.execContainerCfsQuota(podPlan, containerDir, targetCfsQuota)
		}
		if containerPlan.MemoryLimitBytes != nil {
			c.execContainerMemLimit(podPlan, containerDir, int(*containerPlan.MemoryLimitBytes))
		}
		if containerPlan.Blkio != nil && !containerPlan.Blkio.IsEmpty() {
			c.execContainerBlkio(podPlan, containerDir, containerPlan.Blkio)
		}
		if containerPlan.CPUSet != nil {
			c.execContainerCPUSet(podPlan, containerDir, *containerPlan.CPUSet)
		}
		if containerPlan.LLCInfo != nil {
			containerDirLLCMap[containerDir] = containerPlan.LLCInfo
		}
		if len(containerPlan.CPUSetSpec) != 0 {
			c.execContainerCPUSetMap(podPlan, containerDir, containerPlan.CPUSetSpec)
		}
		if len(containerPlan.CPUAcctSpec) != 0 {
			c.execContainerCPUAcctMap(podPlan, containerDir, containerPlan.CPUAcctSpec)
		}
	}

	if podCPULimitOpt == cgroupscrdutil.ResourceLimitScaleDown {
		c.execPodCfsQuota(podPlan, podDir, targetPodCfsQuota)
	}
	if podMemoryLimitOpt == cgroupscrdutil.ResourceLimitScaleDown {
		c.execPodMemLimit(podPlan, podDir, targetPodMemLimit)
	}

	if podPlan.PodPlan != nil && len(podPlan.PodPlan.CPUAcctSpec) != 0 {
		// only bvt in cpuacct is needed for pod
		c.execPodCPUAcctMap(podPlan, podDir, podPlan.PodPlan.CPUAcctSpec)
	}

	if len(containerDirLLCMap) > 0 {
		c.execContainerLLC(containerDirLLCMap)
	}
}

func (c *plugin) execPodCfsQuota(plan *cgroupscrdutil.PodReconcilePlan, podDir string, targetVal int) {
	owner := generateUpdateOwner(&plan.Owner)
	updatePlan := executor.NewCommonCgroupResourceUpdater(owner, podDir, sysutil.CPUCFSQuota, strconv.Itoa(targetVal))
	c.executor.UpdateBatchByCache(updatePlan)
}

func (c *plugin) execPodMemLimit(plan *cgroupscrdutil.PodReconcilePlan, podDir string, targetVal int) {
	owner := generateUpdateOwner(&plan.Owner)
	swLimitupdatePlan := executor.NewCommonCgroupResourceUpdater(owner, podDir, sysutil.MemorySWLimit, strconv.Itoa(targetVal))
	limitUpdatePlan := executor.NewCommonCgroupResourceUpdater(owner, podDir, sysutil.MemoryLimit, strconv.Itoa(targetVal))
	c.executor.UpdateBatchByCache(swLimitupdatePlan, limitUpdatePlan)
}

func (c *plugin) execContainerCfsQuota(plan *cgroupscrdutil.PodReconcilePlan, containerDir string,
	targetVal int) {
	owner := generateUpdateOwner(&plan.Owner)
	updatePlan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.CPUCFSQuota, strconv.Itoa(targetVal))
	c.executor.UpdateBatchByCache(updatePlan)
}

func (c *plugin) execContainerMemLimit(plan *cgroupscrdutil.PodReconcilePlan, containerDir string,
	targetVal int) {
	owner := generateUpdateOwner(&plan.Owner)
	swUpdatePlan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.MemorySWLimit, strconv.Itoa(targetVal))
	limitupdatePlan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.MemoryLimit, strconv.Itoa(targetVal))
	// do it hack, update memory.memsw.limit_in_bytes twice in case of order failure
	c.executor.UpdateBatchByCache(swUpdatePlan, limitupdatePlan, swUpdatePlan)
}

func (c *plugin) execContainerBlkio(plan *cgroupscrdutil.PodReconcilePlan, containerDir string,
	blkio *resourcesv1alpha1.Blkio) {
	owner := generateUpdateOwner(&plan.Owner)
	updatePlans := make([]executor.ResourceUpdater, 0)
	if len(blkio.DeviceReadBps) != 0 {
		content := cgroupscrdutil.GenerateBklioContent(blkio.DeviceReadBps)
		if content != "" {
			plan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.BlkioReadBps, content)
			updatePlans = append(updatePlans, plan)
		}
	}
	if len(blkio.DeviceWriteBps) != 0 {
		content := cgroupscrdutil.GenerateBklioContent(blkio.DeviceWriteBps)
		if content != "" {
			plan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.BlkioWriteBps, content)
			updatePlans = append(updatePlans, plan)
		}
	}
	if len(blkio.DeviceReadIOps) != 0 {
		if content := cgroupscrdutil.GenerateBklioContent(blkio.DeviceReadIOps); content != "" {
			plan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.BlkioReadIops, content)
			updatePlans = append(updatePlans, plan)
		}
	}
	if len(blkio.DeviceWriteIOps) != 0 {
		if content := cgroupscrdutil.GenerateBklioContent(blkio.DeviceWriteIOps); content != "" {
			plan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.BlkioWriteIops, content)
			updatePlans = append(updatePlans, plan)
		}
	}
	c.executor.UpdateBatchByCache(updatePlans...)
}

func (c *plugin) execContainerCPUSet(plan *cgroupscrdutil.PodReconcilePlan, containerDir string,
	cpusetStr string) {
	owner := generateUpdateOwner(&plan.Owner)
	updatePlan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, sysutil.CPUSet, cpusetStr)
	c.executor.UpdateBatchByCache(updatePlan)
}

func (c *plugin) execContainerLLC(containerDirLLCMap map[string]*resourcesv1alpha1.LLCinfo) {
	nodeInfo, err := c.metricCache.GetNodeCPUInfo(&metriccache.QueryParam{})
	if err != nil {
		klog.Warningf("get node info failed during update by cgroup crd, error %v", err)
		return
	}

	cacheQOSMap := make(map[string]struct{}, 0)
	llcSchemaPlans := make([]executor.ResourceUpdater, 0, len(containerDirLLCMap))
	llcTasksPlans := make([]executor.ResourceUpdater, 0, len(containerDirLLCMap))
	for containerDir, llcInfo := range containerDirLLCMap {
		cacheQOSGroup := cgroupscrdutil.GenResctrlGroup(llcInfo.LLCPriority)
		// update l3 schema if cache qos not exist
		if _, exist := cacheQOSMap[cacheQOSGroup]; !exist {
			if err = sysutil.InitCatGroupIfNotExist(cacheQOSGroup); err != nil {
				klog.Errorf("init cat group dir %v failed, error %v", cacheQOSGroup, err)
			}
			if schemaPlans := generateLLCSchemaUpdater(nodeInfo, llcInfo); len(schemaPlans) > 0 {
				llcSchemaPlans = append(llcSchemaPlans, schemaPlans...)
			}
			cacheQOSMap[cacheQOSGroup] = struct{}{}
		}

		// update container tasks
		if tasksPlan := generateLLCTaskUpdater(llcInfo.LLCPriority, containerDir); tasksPlan != nil {
			llcTasksPlans = append(llcTasksPlans, tasksPlan)
		}
	}
	c.executor.UpdateBatchByCache(llcSchemaPlans...)
	c.executor.UpdateBatch(llcTasksPlans...)
}

func (c *plugin) execContainerCPUSetMap(plan *cgroupscrdutil.PodReconcilePlan, containerDir string,
	cpuSetMap map[string]string) {
	updatePlans := make([]executor.ResourceUpdater, 0, len(cpuSetMap))
	owner := generateUpdateOwner(&plan.Owner)
	for fileName, fileContent := range cpuSetMap {
		cgroupFile := sysutil.CgroupFile{ResourceFileName: fileName, Subfs: sysutil.CgroupCPUSetDir, IsAnolisOS: false}
		plan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, cgroupFile, fileContent)
		updatePlans = append(updatePlans, plan)
	}
	c.executor.UpdateBatchByCache(updatePlans...)
}

func (c *plugin) execContainerCPUAcctMap(plan *cgroupscrdutil.PodReconcilePlan, containerDir string,
	cpuAcctMap map[string]string) {
	updatePlans := make([]executor.ResourceUpdater, 0, len(cpuAcctMap))
	owner := generateUpdateOwner(&plan.Owner)
	for fileName, fileContent := range cpuAcctMap {
		isAlnolisOS := false
		if fileName == sysutil.CPUBVTWarpNsName {
			isAlnolisOS = true
		}
		cgroupFile := sysutil.CgroupFile{ResourceFileName: fileName, Subfs: sysutil.CgroupCPUacctDir, IsAnolisOS: isAlnolisOS}
		plan := executor.NewCommonCgroupResourceUpdater(owner, containerDir, cgroupFile, fileContent)
		updatePlans = append(updatePlans, plan)
	}
	c.executor.UpdateBatchByCache(updatePlans...)
}

func (c *plugin) execPodCPUAcctMap(plan *cgroupscrdutil.PodReconcilePlan, podDir string,
	cpuAcctMap map[string]string) {
	updatePlans := make([]executor.ResourceUpdater, 0, len(cpuAcctMap))
	owner := generateUpdateOwner(&plan.Owner)
	for fileName, fileContent := range cpuAcctMap {
		isAlnolisOS := false
		if fileName == sysutil.CPUBVTWarpNsName {
			isAlnolisOS = true
		}
		cgroupFile := sysutil.CgroupFile{ResourceFileName: fileName, Subfs: sysutil.CgroupCPUacctDir, IsAnolisOS: isAlnolisOS}
		plan := executor.NewCommonCgroupResourceUpdater(owner, podDir, cgroupFile, fileContent)
		updatePlans = append(updatePlans, plan)
	}
	c.executor.UpdateBatchByCache(updatePlans...)
}

func generateLLCSchemaUpdater(nodeCPUInfo *metriccache.NodeCPUInfo, llcInfo *resourcesv1alpha1.LLCinfo) []executor.ResourceUpdater {
	cacheQOSGroup := cgroupscrdutil.GenResctrlGroup(llcInfo.LLCPriority)

	cbmStr := nodeCPUInfo.BasicInfo.CatL3CbmMask
	if len(cbmStr) <= 0 {
		klog.Warning("failed to get cat l3 cbm, cbm is empty")
		return nil
	}
	cbmValue, err := strconv.ParseUint(cbmStr, 16, 32)
	if err != nil {
		klog.Warningf("failed to parse cat l3 cbm %s, err: %v", cbmStr, err)
		return nil
	}
	cbm := uint(cbmValue)

	// get the number of l3 caches; it is larger than 0
	l3Num := int(nodeCPUInfo.TotalInfo.NumberL3s)
	if l3Num <= 0 {
		klog.Warningf("failed to get the number of l3 caches, invalid value %v", l3Num)
		return nil
	}

	// calculate updating l3 schema
	l3Percent, err := strconv.ParseInt(llcInfo.L3Percent, 10, 64)
	if err != nil {
		klog.Warningf("failed to parse l3 percent for group %v, err: %v", cacheQOSGroup, err)
		return nil
	}
	l3SchemataDelta, err := sysutil.CalculateCatL3MaskValue(cbm, 0, l3Percent)
	if err != nil {
		klog.Warningf("failed to calculate l3 cat schemata for group %v, err: %v", cacheQOSGroup, err)
		return nil
	}
	l3Resource := executor.CalculateL3SchemataResource(cacheQOSGroup, l3SchemataDelta, l3Num)

	// calculate mem bandwidth policy
	mbPercent, err := strconv.ParseInt(llcInfo.MBPercent, 10, 64)
	if err != nil {
		klog.Infof("parse llc mb percent failed %v, error %v", llcInfo.MBPercent, err)
		return nil
	}
	mbSchemataDelta := calculateCatMbSchemata(int(mbPercent))
	// calculate updating resource
	mbResource := executor.CalculateMbSchemataResource(cacheQOSGroup, mbSchemataDelta, l3Num)

	return []executor.ResourceUpdater{l3Resource, mbResource}
}

func generateLLCTaskUpdater(cacheQOS string, containerDir string) executor.ResourceUpdater {
	cacheQOSGroup := cgroupscrdutil.GenResctrlGroup(cacheQOS)
	curTasksInResctrl, err := sysutil.ReadResctrlTasksMap(cacheQOSGroup)
	if err != nil {
		klog.Warningf("get tasks from resctrl %v failed, error %v", cacheQOS, err)
		return nil
	}
	newTaskIds := make([]int, 0)
	containerTaskDir := sysutil.GetCgroupFilePath(containerDir, sysutil.CPUTask)
	containerTasks, err := sysutil.GetCgroupCurTasks(containerTaskDir)
	if err != nil {
		klog.Warningf("get tasks from %s failed during handle cache qos %v, error %v",
			containerTaskDir, cacheQOS, err)
		return nil
	}
	for _, id := range containerTasks {
		if _, exist := curTasksInResctrl[id]; !exist {
			newTaskIds = append(newTaskIds, id)
		}
	}

	return executor.CalculateL3TasksResource(cacheQOSGroup, newTaskIds)
}

func generateUpdateOwner(planOwner *cgroupscrdutil.PlanOwner) *executor.OwnerRef {
	result := &executor.OwnerRef{
		Namespace: planOwner.Namespace,
		Name:      planOwner.Name,
	}
	switch planOwner.Type {
	case cgroupscrdutil.CgroupsCrdType:
		result.Type = executor.OthersType
	default:
		result.Type = executor.PodType
	}
	return result
}

func calculateCatMbSchemata(memBwPercent int) string {
	// memory bandwidth if valid in range (0, 100]
	result := util.MinInt64(util.MaxInt64(int64(memBwPercent), 1), 100)
	return strconv.FormatInt(result, 10)
}

package ackml

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/metriccache"
	mlutil "github.com/koordinator-sh/koordinator/pkg/koordlet/resmanager/plugins/ackmemorylocality/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/resourceexecutor"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	koordletutil "github.com/koordinator-sh/koordinator/pkg/koordlet/util"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

const (
	MemoryLocalityReconcileInterval = "1m"
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
}

var (
	scheme                                        = apiruntime.NewScheme()
	MemoryLocalityFeatureName featuregate.Feature = "MemoryLocality"
	MemoryLocalityFeatureSpec                     = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Alpha}
	MemoryLocalityPlugin                          = &plugin{
		config: NewDefaultConfig(),
	}
)

type plugin struct {
	metricCache    metriccache.MetricCache
	statesInformer statesinformer.StatesInformer
	resexecutor    resourceexecutor.ResourceUpdateExecutor
	eventRecorder  record.EventRecorder
	kubeClient     clientset.Interface
	cgroupReader   resourceexecutor.CgroupReader
	config         *Config
}

type Config struct {
	MemoryLocalityReconcileInterval time.Duration
}

func NewDefaultConfig() *Config {
	duration, _ := time.ParseDuration(MemoryLocalityReconcileInterval)
	return &Config{
		MemoryLocalityReconcileInterval: duration,
	}
}

func (m *Config) InitFlags(fs *flag.FlagSet) {
	fs.DurationVar(&m.MemoryLocalityReconcileInterval, "memory-locality-reconcile-interval",
		m.MemoryLocalityReconcileInterval, "reconcile memory locality setting interval")
}

func (m *plugin) InitFlags(fs *flag.FlagSet) {
	m.config.InitFlags(fs)
}

func (m *plugin) Setup(client clientset.Interface, metricCache metriccache.MetricCache,
	statesInformer statesinformer.StatesInformer) {

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&clientcorev1.EventSinkImpl{Interface: client.CoreV1().Events("")})
	nodeName := os.Getenv("NODE_NAME")
	m.eventRecorder = eventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: "koordlet", Host: nodeName})

	m.metricCache = metricCache
	m.statesInformer = statesInformer
	m.resexecutor = resourceexecutor.NewResourceUpdateExecutor()
	m.kubeClient = client
	m.cgroupReader = resourceexecutor.NewCgroupReader()
}

func (m *plugin) Run(stopCh <-chan struct{}) {
	go wait.Until(m.reconcile, m.config.MemoryLocalityReconcileInterval, stopCh)
	m.resexecutor.Run(stopCh)
}

// reconcile creates and inits config if the host support MemoryLocality
func (m *plugin) reconcile() {
	nodeSLO := m.statesInformer.GetNodeSLO()
	if nodeSLO == nil {
		klog.Warningf("nodeSLO is nil, skip reconcile MemoryLocality!")
		return
	}

	// skip if host not support MemoryLocality
	// only check once
	support, err := mlutil.IsSupportMemoryLocality()
	if err != nil {
		klog.Warningf("check support memory locality failed, err: %v", err)
	}
	if !support {
		return
	}

	podsMeta := m.statesInformer.GetAllPods()
	for _, podMeta := range podsMeta {
		pod := podMeta.Pod
		// only Running and Pending pods are considered
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}

		isCpuset, err := util.IsPodCPUSet(pod.Annotations)
		if err != nil {
			klog.Warningf("failed to check if pod is CPUSet")
			continue
		}
		// only pod supporting cpuset are considered
		if !isCpuset {
			continue
		}

		// get pod-level config by pod.Annotations
		podCfg, err := mlutil.GetPodMemoryLocality(pod)
		if err != nil { // give up migration when parse error
			klog.Warningf("failed to parse memory locality config, pod %s, err: %s", util.GetPodKey(pod), err)
			continue
		}

		// get node-level config by cm/ack-slo-config
		if (podCfg == nil || podCfg.Policy == nil) && nodeSLO.Spec.Extensions != nil && nodeSLO.Spec.Extensions.Object != nil {
			mlIf, exist := nodeSLO.Spec.Extensions.Object[ackapis.MemoryLocalityExtKey]
			if !exist {
				podCfg = mlutil.GetPodMemoryLocalityByQoSClass(pod, util.DefaultMemoryLocalityStrategy())
			} else {
				mlStr, err := json.Marshal(mlIf)
				if err != nil {
					klog.Warningf("failed to marshal interface, skipped memory locality, pod %s, err: %s", util.GetPodKey(pod), err)
					continue
				}
				mlCfg := ackapis.MemoryLocalityStrategy{}
				if err := json.Unmarshal(mlStr, &mlCfg); err != nil {
					klog.Warningf("failed to unmarshal interface, skipped memory locality, pod %s, err: %s", util.GetPodKey(pod), err)
					continue
				}
				podCfg = mlutil.GetPodMemoryLocalityByQoSClass(pod, &mlCfg)
			}
		}

		if podCfg == nil {
			return
		}

		if err = mlutil.CheckIfCfgVaild(podCfg); err != nil {
			klog.Warningf("invalid memory locality configuration, skipped memory locality, pod %s, err: %s", util.GetPodKey(pod), err)
			continue
		}
		podCfg = mlutil.MergeDefaultMemoryLocality(podCfg)

		klog.V(5).Infof("begin to process pod %s with memory locality %+v", util.GetPodKey(pod), podCfg)
		switch *(podCfg.Policy) {
		case ackapis.MemoryLocalityPolicyNone:
			oldStatus, err := mlutil.GetMemoryLocalityStatus(pod.Annotations)
			if err != nil {
				klog.V(5).Infof("failed to get memory locality status, pod %s, err %s", pod.Name, err.Error())
				continue
			}
			if oldStatus == nil {
				continue
			}
			pod, err = m.setMemoryLocalityStatus(pod, &mlutil.MemoryLocalityStatus{Phase: mlutil.MemoryLocalityStatusClosed.Pointer(), LastResult: nil})
			if err != nil {
				klog.Warningf("failed to set memory locality status, pod %s, new status %s, err %s",
					pod.Name, mlutil.MemoryLocalityStatusClosed, err.Error())
			}
			continue
		case ackapis.MemoryLocalityPolicyBesteffort:
			res, err := m.processPodMemoryMigrate(podMeta, *podCfg)
			if err != nil {
				klog.Warningf("failed to migrate memory, pod %s, err: %s", util.GetPodKey(pod), err)
				continue
			}
			if res == nil {
				continue
			}
			klog.V(5).Infof("migrate the memory of pod %v with policy %v successfully", util.GetPodKey(pod), podCfg.Policy)
		default:
			klog.V(5).Infof("unexpected memory locality policy, pod %s, policy %v", util.GetPodKey(pod), podCfg.Policy)
		}
	}

}

// TODO: only works in queue now. The order should be considered.
func (m *plugin) processPodMemoryMigrate(podMeta *statesinformer.PodMeta, cfg ackapis.MemoryLocality) (*mlutil.ContainerInfoMap, error) {
	// ensure all annotations has updated
	pod, err := m.kubeClient.CoreV1().Pods(podMeta.Pod.Namespace).Get(context.TODO(), podMeta.Pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// step 1: check if completed and migrateIntervalMinutes has reached
	mls, err := mlutil.GetMemoryLocalityStatus(pod.Annotations)
	if err != nil {
		klog.V(5).Infof("failed to get memory locality status, pod %s, err %s", pod.Name, err.Error())
	}
	if mls != nil {
		phase := mls.Phase
		if phase != nil && *phase == mlutil.MemoryLocalityStatusCompleted && *cfg.MigrateIntervalMinutes == 0 {
			return nil, nil
		}
		mmr := mls.LastResult
		if mmr != nil && *cfg.MigrateIntervalMinutes > 0 &&
			time.Now().Before(mmr.CompletedTime.Time.Add(time.Duration(*cfg.MigrateIntervalMinutes)*time.Minute)) {
			return nil, nil
		}
	}

	containerInfoMap := mlutil.ContainerInfoMap{}
	// step 2: get local numalist
	// get the CPUSet info
	// cpuset [0, 1, 2, 3, 4, 5, 46, 47, 48].
	containerInfoMap = m.getPodCGroupCpuList(podMeta, containerInfoMap)
	if len(containerInfoMap) == 0 {
		return nil, fmt.Errorf("pod is a CPUSet Type but has no cpu info")
	}

	nodeCPUInfo, err := m.metricCache.GetNodeCPUInfo(&metriccache.QueryParam{})
	if err != nil {
		return nil, err
	}
	if nodeCPUInfo == nil {
		return nil, fmt.Errorf("get empty node cpu info")
	}
	// scan the cpu to check where the numa locate
	for _, containerInfo := range containerInfoMap {
		if containerInfo.Invalid {
			continue
		}
		nodeList := map[int]struct{}{}
		for _, proInfo := range nodeCPUInfo.ProcessorInfos {
			for _, cpuId := range containerInfo.Cpulist {
				if proInfo.CPUID == int32(cpuId) {
					nodeList[int(proInfo.NodeID)] = struct{}{}
				}
			}
		}
		containerInfo.Numalist = nodeList
	}

	// step 3: get tasks
	containerInfoMap = m.getPodCgroupTaskIds(podMeta, containerInfoMap)

	// step 4: check cur locality ratio and compared with target
	containerInfoMap = m.getPodCgroupNumaStat(podMeta, containerInfoMap)
	var preLocalityCnt, preRemoteCnt uint64
	for _, containerInfo := range containerInfoMap {
		if containerInfo.Invalid {
			continue
		}
		for _, numaStat := range containerInfo.NumaStat {
			if _, ok := containerInfo.Numalist[numaStat.NumaId]; ok {
				preLocalityCnt += numaStat.PagesNum
			} else {
				preRemoteCnt += numaStat.PagesNum
			}
		}
	}

	if preLocalityCnt == 0 && preRemoteCnt == 0 {
		return nil, fmt.Errorf("failed to get memory allocation or no memory use")
	}

	preLocalityRatio := int64(math.Ceil(float64(preLocalityCnt) / float64(preLocalityCnt+preRemoteCnt) * 100))
	if preLocalityRatio > *cfg.TargetLocalityRatio {
		message := fmt.Sprintf("already reach target in precheck, cur local memory ratio %v, rest remote memory pages %v", preLocalityRatio, preRemoteCnt)
		newMmr := &mlutil.MemoryMigrateResult{}
		newMmr.CompletedTime = metav1.Now()
		newMmr.CompletedTimeLocalityRatio = preLocalityRatio
		newMmr.CompletedTimeRemotePages = int64(preRemoteCnt)
		newMmr.Result = mlutil.MigratedSkipped
		var newMls *mlutil.MemoryLocalityStatus
		if *cfg.MigrateIntervalMinutes == 0 {
			// create event first to check if result has changed
			newMls = &mlutil.MemoryLocalityStatus{Phase: mlutil.MemoryLocalityStatusCompleted.Pointer(), LastResult: newMmr}
			m.createEvent(pod, mlutil.MigratedSkipped, message, preLocalityRatio, true)
		} else {
			newMls = &mlutil.MemoryLocalityStatus{Phase: mlutil.MemoryLocalityStatusInProgress.Pointer(), LastResult: newMmr}
			m.createEvent(pod, mlutil.MigratedSkipped, message, preLocalityRatio, false)
		}
		pod, err = m.setMemoryLocalityStatus(pod, newMls)
		if err != nil {
			klog.Warningf("failed to set memory locality status, pod %s, new status %+v, err %s",
				pod.Name, newMls, err.Error())
		}
		return nil, nil
	}

	pod, err = m.setMemoryLocalityStatus(pod, &mlutil.MemoryLocalityStatus{Phase: mlutil.MemoryLocalityStatusInProgress.Pointer(), LastResult: nil})
	if err != nil {
		klog.Warningf("failed to set memory locality status, pod %s, new status %s, err %s",
			pod.Name, mlutil.MemoryLocalityStatusInProgress, err.Error())
	}

	// step 5: migratepages task(pid) fromnumalist(remote numalist) tonumalist(locality numalist)
	completedCnt, skippedCnt, failedCnt := 0, 0, 0
	msgset := []string{}
	for name, containerInfo := range containerInfoMap {
		if containerInfo.Invalid {
			continue
		}
		containerInfo.Result = mlutil.MigrateBetweenNuma(containerInfo.Numalist, containerInfo.TaskIds)
		switch containerInfo.Result.Status {
		case mlutil.MigratedCompleted:
			completedCnt = completedCnt + 1
			msgset = append(msgset, fmt.Sprintf("Container %s completed: %s", name, containerInfo.Result.Message))
		case mlutil.MigratedSkipped:
			skippedCnt = skippedCnt + 1
			msgset = append(msgset, fmt.Sprintf("Container %s skipped: %s", name, containerInfo.Result.Message))
		case mlutil.MigratedFailed:
			failedCnt = failedCnt + 1
			msgset = append(msgset, fmt.Sprintf("Container %s failed: %s", name, containerInfo.Result.Message))
		}
	}

	containerInfoMap = m.getPodCgroupNumaStat(podMeta, containerInfoMap)
	var postLocalityCnt, postRemoteCnt uint64
	for _, containerInfo := range containerInfoMap {
		if containerInfo.Invalid {
			continue
		}
		for _, numaStat := range containerInfo.NumaStat {
			if _, ok := containerInfo.Numalist[numaStat.NumaId]; ok {
				postLocalityCnt += numaStat.PagesNum
			} else {
				postRemoteCnt += numaStat.PagesNum
			}
		}
	}

	var postLocalityRatio int64
	if postLocalityCnt == 0 && postRemoteCnt == 0 {
		postLocalityRatio = -1
		klog.Warningf("failed to get memory allocation, pod %s", pod.Name)
		msgset = append(msgset, "Total: failed to get memory allocation after memory locality")
	} else {
		postLocalityRatio = int64(math.Ceil(float64(postLocalityCnt) / float64(postLocalityCnt+postRemoteCnt) * 100))
		msgset = append(msgset, fmt.Sprintf("Total: %d container(s) completed, %d failed, %d skipped; cur local memory ratio %v, rest remote memory pages %v",
			completedCnt, failedCnt, skippedCnt, postLocalityRatio, postRemoteCnt))
	}

	res := mlutil.MigratePagesResult{}
	if completedCnt > 0 {
		res.Status = mlutil.MigratedCompleted
	} else if failedCnt > 0 {
		res.Status = mlutil.MigratedFailed
	} else {
		res.Status = mlutil.MigratedSkipped
	}
	res.Message = strings.Join(msgset, "\n")
	m.createEvent(pod, res.Status, res.Message, postLocalityRatio, *cfg.MigrateIntervalMinutes == 0)

	// set annotation
	newMmr := &mlutil.MemoryMigrateResult{}
	newMmr.CompletedTime = metav1.Now()
	newMmr.CompletedTimeLocalityRatio = postLocalityRatio
	newMmr.CompletedTimeRemotePages = int64(postRemoteCnt)
	newMmr.Result = res.Status
	newMls := &mlutil.MemoryLocalityStatus{Phase: nil, LastResult: newMmr}
	if *cfg.MigrateIntervalMinutes == 0 {
		newMls.Phase = mlutil.MemoryLocalityStatusCompleted.Pointer()
	}
	pod, err = m.setMemoryLocalityStatus(pod, newMls)
	if err != nil {
		klog.Warningf("failed to set memory locality status, pod %s, new status %+v, err %s",
			pod.Name, newMls, err.Error())
	}
	return &containerInfoMap, nil
}

func (m *plugin) setMemoryLocalityStatus(pod *corev1.Pod, status *mlutil.MemoryLocalityStatus) (*corev1.Pod, error) {
	if pod == nil || status == nil || (status.Phase == nil && status.LastResult == nil) {
		return pod, fmt.Errorf("failed to set pod memory locality status")
	}
	oldStatus, err := mlutil.GetMemoryLocalityStatus(pod.Annotations)
	if err != nil {
		// oldStatus is nil
		klog.V(5).Infof("failed to get memory locality status, pod %s, err %s", pod.Name, err.Error())
	}
	newStatus := status.DeepCopy()
	if oldStatus != nil {
		merged, err := util.MergeCfg(oldStatus.DeepCopy(), status)
		if err != nil {
			return pod, err
		}
		newStatus = merged.(*mlutil.MemoryLocalityStatus)
	}
	if reflect.DeepEqual(newStatus, oldStatus) {
		return pod, nil
	}
	var updatedPod *corev1.Pod
	retErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		newPod, err := m.kubeClient.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			klog.Warningf("pod %s not found, skip set memory locality status %v", pod.Name, status)
			return nil
		} else if err != nil {
			return err
		}
		data, err := json.Marshal(newStatus)
		if err != nil {
			return err
		}
		newPod.Annotations[mlutil.AnnotationMemoryLocalityStatus] = string(data)
		updatedPod, err = m.kubeClient.CoreV1().Pods(pod.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		return nil
	})
	if updatedPod == nil {
		updatedPod = pod
	}
	return updatedPod, retErr
}

func (m *plugin) createEvent(pod *corev1.Pod, reason mlutil.MigratePagesStatus, msg string, localityRatio int64, forceCreate bool) {
	// migrate only once
	if forceCreate {
		m.eventRecorder.Event(pod, corev1.EventTypeNormal, string(reason), msg)
		return
	}
	oldMmr, err := mlutil.GetMemoryLocalityStatus(pod.Annotations)
	if err != nil {
		klog.V(5).Infof("failed to get memory locality status, pod %s, err %s", pod.Name, err.Error())
	}
	// first time
	if oldMmr == nil || oldMmr.LastResult == nil {
		m.eventRecorder.Event(pod, corev1.EventTypeNormal, string(reason), msg)
		return
	}
	oldResult := *oldMmr.LastResult
	// reason has changed
	if oldResult.Result != reason {
		m.eventRecorder.Event(pod, corev1.EventTypeNormal, string(reason), msg)
		return
	}
	// localityRatio has changed over 10%
	if localityRatio != -1 && math.Abs(float64(oldMmr.LastResult.CompletedTimeLocalityRatio-localityRatio)) >= 10 {
		m.eventRecorder.Event(pod, corev1.EventTypeNormal, string(reason), msg)
	}
}

// return map[numaId]pagesNum of the whole pod
func (m *plugin) getPodCgroupNumaStat(podMeta *statesinformer.PodMeta, containerInfoMap mlutil.ContainerInfoMap) mlutil.ContainerInfoMap {
	pod := podMeta.Pod
	containerMap := make(map[string]*corev1.Container, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		containerMap[container.Name] = container
	}
	for _, containerStat := range pod.Status.ContainerStatuses {
		// reconcile containers
		containerInfo, exist := containerInfoMap[containerStat.Name]
		if !exist {
			containerInfoMap[containerStat.Name] = &mlutil.ContainerInfo{}
			containerInfo = containerInfoMap[containerStat.Name]
		}
		container, exist := containerMap[containerStat.Name]
		if !exist {
			klog.Warningf("container %s/%s/%s lost during reconcile memory locality", pod.Namespace,
				pod.Name, containerStat.Name)
			containerInfo.Invalid = true
			continue
		}
		containerDir, err := koordletutil.GetContainerCgroupPathWithKube(podMeta.CgroupDir, &containerStat)
		if err != nil {
			klog.V(4).Infof("failed to get pod container cgroup path for container %s/%s/%s, err: %s",
				pod.Namespace, pod.Name, container.Name, err)
			containerInfo.Invalid = true
			continue
		}
		cstats, err := m.cgroupReader.ReadMemoryNumaStat(containerDir)
		if err != nil {
			klog.Warningf("failed to get pod container cgroup numa stat for container %s/%s/%s, err: %s",
				pod.Namespace, pod.Name, container.Name, err)
			containerInfo.Invalid = true
			continue
		}
		containerInfo.NumaStat = cstats
	}
	return containerInfoMap
}

func (m *plugin) getPodCGroupCpuList(podMeta *statesinformer.PodMeta, containerInfoMap mlutil.ContainerInfoMap) mlutil.ContainerInfoMap {
	pod := podMeta.Pod
	if len(pod.Status.ContainerStatuses) == 0 {
		return containerInfoMap
	}
	containerMap := make(map[string]*corev1.Container, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		containerMap[container.Name] = container
	}
	for _, containerStat := range pod.Status.ContainerStatuses {
		// reconcile containers
		containerInfo, exist := containerInfoMap[containerStat.Name]
		if !exist {
			containerInfoMap[containerStat.Name] = &mlutil.ContainerInfo{}
			containerInfo = containerInfoMap[containerStat.Name]
		}
		container, exist := containerMap[containerStat.Name]
		if !exist {
			klog.Warningf("container %s/%s/%s lost during reconcile memory locality", pod.Namespace,
				pod.Name, containerStat.Name)
			containerInfo.Invalid = true
			continue
		}
		containerDir, err := koordletutil.GetContainerCgroupPathWithKube(podMeta.CgroupDir, &containerStat)
		if err != nil {
			klog.V(4).Infof("failed to get pod container cgroup path for container %s/%s/%s, err: %s",
				pod.Namespace, pod.Name, container.Name, err)
			containerInfo.Invalid = true
			continue
		}
		cpus, err := m.cgroupReader.ReadCPUSet(containerDir)
		if err != nil {
			klog.Warningf("failed to get pod cpu set %s/%s/%s, err: %s",
				pod.Namespace, pod.Name, container.Name, err)
			containerInfo.Invalid = true
			continue
		}
		containerInfo.Cpulist = cpus.ToSlice()

	}
	return containerInfoMap
}

func (m *plugin) getPodCgroupTaskIds(podMeta *statesinformer.PodMeta, containerInfoMap mlutil.ContainerInfoMap) mlutil.ContainerInfoMap {
	pod := podMeta.Pod
	containerMap := make(map[string]*corev1.Container, len(pod.Spec.Containers))
	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		containerMap[container.Name] = container
	}
	for _, containerStat := range pod.Status.ContainerStatuses {
		// reconcile containers
		containerInfo, exist := containerInfoMap[containerStat.Name]
		if !exist {
			containerInfoMap[containerStat.Name] = &mlutil.ContainerInfo{}
			containerInfo = containerInfoMap[containerStat.Name]
		}
		container, exist := containerMap[containerStat.Name]
		if !exist {
			klog.Warningf("container %s/%s/%s lost during reconcile memory locality", pod.Namespace,
				pod.Name, containerStat.Name)
			containerInfo.Invalid = true
			continue
		}

		containerDir, err := koordletutil.GetContainerCgroupPathWithKube(podMeta.CgroupDir, &containerStat)
		if err != nil {
			klog.V(4).Infof("failed to get pod container cgroup path for container %s/%s/%s, err: %s",
				pod.Namespace, pod.Name, container.Name, err)
			containerInfo.Invalid = true
			continue
		}
		ids, err := m.cgroupReader.ReadCPUTasks(containerDir)
		if err != nil {
			klog.Warningf("failed to get pod container cgroup task ids for container %s/%s/%s, err: %s",
				pod.Namespace, pod.Name, container.Name, err)
			containerInfo.Invalid = true
			continue
		}
		containerInfo.TaskIds = ids
	}

	return containerInfoMap
}

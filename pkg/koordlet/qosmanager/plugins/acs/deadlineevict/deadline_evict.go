package deadlineevict

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	"github.com/koordinator-sh/koordinator/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DeadlineEvictName = "DeadlineEvict"
)

var (
	timeNow = time.Now
)

type deadlineEvict struct {
	evictInterval  time.Duration
	statesInformer statesinformer.StatesInformer
	// TOOD refactor Evictor in framework pkg as an interface which can be extended with different implementation
	evictor *acsEvictor
}

func New(opt *framework.Options) framework.QOSStrategy {
	return &deadlineEvict{
		evictInterval:  time.Second * 10, // TODO support specified args
		statesInformer: opt.StatesInformer,
		evictor:        newACSEvictor(opt.KubeClient),
	}
}

func (d *deadlineEvict) Enabled() bool {
	return features.DefaultKoordletFeatureGate.Enabled(features.DeadlineEvict) && d.evictInterval > 0
}

func (d *deadlineEvict) Setup(*framework.Context) {
}

func (d *deadlineEvict) Run(stopCh <-chan struct{}) {
	go wait.Until(d.deadlineEvict, d.evictInterval, stopCh)
}

func (d *deadlineEvict) deadlineEvict() {
	klog.V(5).Infof("deadline evict cycle start")

	pods := d.statesInformer.GetAllPods()
	nodeSLO := d.statesInformer.GetNodeSLO()

	if nodeSLO == nil {
		klog.V(4).Infof("node slo is nil, skip deadline evict")
		return
	}

	nodeStrategy, err := parseNodeStrategy(nodeSLO)
	if err != nil {
		klog.Errorf("failed to get node deadline evict config, error %v", err)
		return
	}

	counter := struct {
		nonBEPods      int
		notEnabledPods int
		noDeadlinePods int
		notExpiredPods int
		evictedPods    int
	}{}
	for _, podMeta := range pods {
		pod := podMeta.Pod
		if apiext.GetPodQoSClassRaw(pod) != apiext.QoSBE {
			klog.V(6).Infof("pod %s is not BE, skip deadline evict", util.GetPodKey(pod))
			counter.nonBEPods++
			continue
		}

		// TODO use getMergedDeadlineEvictStrategy in future for supporting pod level config
		// podDeadlineStrategy := getMergedDeadlineEvictStrategy(pod, nodeStrategy)
		podDeadlineStrategy := nodeStrategy
		if podDeadlineStrategy == nil ||
			(podDeadlineStrategy.Enable != nil && !*podDeadlineStrategy.Enable) || // disable
			podDeadlineStrategy.DeadlineDuration == nil { // not set
			podDeadlineStrategyStr, _ := json.Marshal(podDeadlineStrategy)
			klog.V(5).Infof("pod %s has not enable deadline evict, skip deadline evict, detail %v",
				util.GetPodKey(pod), string(podDeadlineStrategyStr))
			counter.notEnabledPods++
			continue
		}

		podDeadline := getPodDeadline(pod, *podDeadlineStrategy.DeadlineDuration)
		now := timeNow()
		if podDeadline == nil {
			klog.V(5).Infof("pod %s deadline is nil, skip deadline evict")
			counter.noDeadlinePods++
			continue
		} else if podDeadline.After(now) {
			klog.V(5).Infof("pod %s has not reach deadline %v, now %v, skip deadline evict", util.GetPodKey(pod), podDeadline.String(), now.String())
			counter.notExpiredPods++
			continue
		}

		// TODO export prometheus metrics
		evictMsg := fmt.Sprintf("pod is expired because best-effort instance can only run maximum %s duration",
			podDeadlineStrategy.DeadlineDuration.Duration.String())
		updated, err := d.evictor.evictPod(pod, evictMsg)
		if updated {
			klog.V(4).Infof("pod %s is evicted with deadline %v, err: %v", util.GetPodKey(pod), podDeadline.String(), err)
		} else {
			klog.V(5).Infof("pod %s has already evicted with deadline %v, err: %v", util.GetPodKey(pod), podDeadline.String(), err)
		}

		counter.evictedPods++
	}
	klog.V(4).Infof("deadline evict cycle end, total %v, counter details %+v", len(pods), counter)
}

func parseNodeStrategy(nodeSLO *v1alpha1.NodeSLO) (*unified.DeadlineEvictStrategy, error) {
	if nodeSLO == nil {
		return nil, fmt.Errorf("node slo is nil")
	}
	deadlineEvictIf, exist := nodeSLO.Spec.Extensions.Object[unified.DeadlineEvictExtKey]
	if !exist {
		return nil, fmt.Errorf("deadline evict config is nil")
	}
	deadlineEvictStr, err := json.Marshal(deadlineEvictIf)
	if err != nil {
		return nil, err
	}
	deadlineEvictCfg := &unified.DeadlineEvictStrategy{}
	if err := json.Unmarshal(deadlineEvictStr, deadlineEvictCfg); err != nil {
		return nil, err
	}
	return deadlineEvictCfg, nil
}

// getMergedDeadlineEvictStrategy merge node strategy into pod strategy if specified
func getMergedDeadlineEvictStrategy(pod *corev1.Pod, nodeStrategy *unified.DeadlineEvictStrategy) *unified.DeadlineEvictStrategy {
	podStrategy, err := unified.GetPodDeadlineEvictStrategy(pod)
	if err != nil {
		klog.Warningf("get pod %vdeadline evict strategy failed, use node strategy, err: %v", util.GetPodKey(pod), err)
		return nodeStrategy
	}
	if podStrategy == nil {
		return nodeStrategy
	} else if nodeStrategy == nil {
		return podStrategy
	}

	mergedIf, err := util.MergeCfg(nodeStrategy, podStrategy)
	if err != nil {
		klog.Warningf("merge pod %vdeadline evict strategy failed, use node strategy, err: %v", util.GetPodKey(pod), err)
		return nodeStrategy
	}

	mergedStrategy := *mergedIf.(*unified.DeadlineEvictStrategy)
	return &mergedStrategy
}

func getPodDeadline(pod *corev1.Pod, deadlineDuration metav1.Duration) *time.Time {
	if pod == nil {
		return nil
	}
	if pod.Status.StartTime == nil {
		klog.V(5).Infof("pod %s has no start time", util.GetPodKey(pod))
		return nil
	}
	durationBeforeEviction := unified.DefaultPodDurationBeforeEviction
	if podSpecified, err := unified.GetPodDurationBeforeEviction(pod); err == nil && podSpecified != nil {
		durationBeforeEviction = *podSpecified
	} else if err != nil {
		klog.V(4).Infof("get pod %v duration before eviction failed, usage default value %v, err: %v",
			util.GetPodKey(pod), durationBeforeEviction, err)
	}
	podDeadline := pod.Status.StartTime.Add(deadlineDuration.Duration - durationBeforeEviction)
	return &podDeadline
}

type acsEvictor struct {
	client clientset.Interface
}

func newACSEvictor(kubeClient clientset.Interface) *acsEvictor {
	return &acsEvictor{client: kubeClient}
}

// evictPod patch the following labels and annotations on pod
// labels:
//   alibabacloud.com/eviction: 'true'
// annotations:
//   alibabacloud.com/eviction-type: '["involuntary"]'
//   alibabacloud.com/skip-not-ready-flow-control: true
//   alibabacloud.com/eviction-message: message
func (e *acsEvictor) evictPod(pod *corev1.Pod, message string) (bool, error) {
	if pod == nil {
		return false, nil
	}
	newPod := pod.DeepCopy()
	if newPod.Labels == nil {
		newPod.Labels = make(map[string]string)
	}
	if newPod.Annotations == nil {
		newPod.Annotations = make(map[string]string)
	}
	evictionType := []string{
		unified.AnnotationEvictionTypeInvoluntary,
	}
	evictionTypeStr, err := json.Marshal(evictionType)
	if err != nil {
		return false, err
	}

	// patch force eviction annotation on pod, eviction controller will take control
	newPod.Labels[unified.LabelEvictionKey] = unified.LabelEvictionValue
	newPod.Annotations[unified.AnnotationEvictionTypeKey] = string(evictionTypeStr)
	newPod.Annotations[unified.AnnotationSkipNotReadyFlowControlKey] = unified.AnnotationSkipNotReadyFlowControlValue
	newPod.Annotations[unified.AnnotationEvictionMessageKey] = message

	if reflect.DeepEqual(newPod.Annotations, pod.Annotations) && reflect.DeepEqual(newPod.Labels, pod.Labels) {
		klog.V(6).Infof("pod %s has no change, skip patch", util.GetPodKey(pod))
		return false, nil
	}

	err = util.RetryOnConflictOrTooManyRequests(func() error {
		_, err := util.PatchPod(context.Background(), e.client, pod, newPod)
		return err
	})
	updated := err == nil
	return updated, err
}

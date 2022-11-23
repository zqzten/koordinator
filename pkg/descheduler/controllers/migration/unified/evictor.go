package unified

import (
	"context"
	"encoding/json"
	"time"

	"gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	koordsev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/evictor"
)

const (
	EvictorName = "LabelBasedEvictor"
)

const (
	LabelASIEviction             = "pod.sigma.ali/eviction"
	AnnotationASIEvictionContext = "pod.alibabacloud.com/eviction-context"
)

type EvictionContent struct {
	StartTime          string `json:"startTime"` // date format: yyyy-mm-dd hh:mm:ss
	Reason             string `json:"reason"`
	Trigger            string `json:"trigger"`
	GracePeriodSeconds int64  `json:"gracePeriodSeconds,omitempty"`
	ForceEviction      bool   `json:"forceEviction,omitempty"`
}

var _ evictor.Interface = &Evictor{}

type Evictor struct {
	client kubernetes.Interface
}

func init() {
	evictor.RegisterEvictor(EvictorName, New)
}

func New(client kubernetes.Interface) (evictor.Interface, error) {
	return &Evictor{
		client: client,
	}, nil
}

func (e *Evictor) Evict(ctx context.Context, job *koordsev1alpha1.PodMigrationJob, pod *corev1.Pod) error {
	trigger := job.Labels[LabelPodMigrateTrigger]
	if trigger == "" {
		trigger = "koord-descheduler"
	}

	reason := job.Annotations[AnnotationPodMigrateReason]
	if reason == "" {
		reason = "migrate pod by descheduler"
	}

	evictionContent := EvictionContent{
		StartTime: time.Now().String(),
		Reason:    reason,
		Trigger:   trigger,
	}
	data, err := json.Marshal(evictionContent)
	if err != nil {
		return err
	}

	patch := NewStrategicPatch()
	patch.InsertAnnotation(extension.AnnotationEvictionContext, string(data))
	patch.InsertAnnotation(AnnotationASIEvictionContext, string(data))
	patch.InsertLabel(extension.LabelPodEviction, "true")
	patch.InsertLabel(LabelASIEviction, "true")
	data, err = patch.Data(nil)
	if err != nil {
		return err
	}

	err = retry.OnError(retry.DefaultRetry, func(err error) bool {
		return errors.IsConflict(err) || errors.IsTooManyRequests(err)
	}, func() error {
		_, err := e.client.CoreV1().Pods(pod.Namespace).Patch(ctx, pod.Name, patch.PatchType, data, metav1.PatchOptions{})
		return err
	})
	if err != nil {
		klog.Errorf("Failed to evict Pod(%s/%s) by labelEvictor, err: %v", pod.Namespace, pod.Name, err)
	}
	return err
}

// HasEvictionLabel check if pod has eviction label
func HasEvictionLabel(pod *corev1.Pod) bool {
	if pod.Labels == nil {
		return false
	}
	return pod.Labels[LabelASIEviction] == "true" || pod.Labels[extension.LabelPodEviction] == "true"
}

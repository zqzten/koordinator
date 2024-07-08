package memorylocality

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	ackapis "github.com/koordinator-sh/koordinator/apis/ackplugins"
	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/util"
)

func createPod(kubeQosClass corev1.PodQOSClass) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test_pod",
			UID:         "test_pod",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test",
				},
				{
					Name: "main",
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test",
					ContainerID: fmt.Sprintf("docker://%s", "test"),
				},
				{
					Name:        "main",
					ContainerID: fmt.Sprintf("docker://%s", "main"),
				},
			},
			QOSClass: kubeQosClass,
			Phase:    corev1.PodRunning,
		},
	}
	return pod
}

func TestGetPodMemoryLocality(t *testing.T) {
	testingPod1 := createPod(corev1.PodQOSBestEffort)

	testingPod2 := createPod(corev1.PodQOSBestEffort)
	testingPod2.Annotations = map[string]string{ackapis.AnnotationPodMemoryLocality: "{\"policy\": \"bestEffort\"}"}

	testingPod3 := createPod(corev1.PodQOSBestEffort)
	testingPod3.Annotations = map[string]string{ackapis.AnnotationPodMemoryLocality: "{\"policy\": \"bestEffort\", \"MigrateIntervalMinutes\": 1}"}

	testingPod4 := createPod(corev1.PodQOSBestEffort)
	testingPod4.Annotations = map[string]string{ackapis.AnnotationPodMemoryLocality: "fake-value"}

	type args struct {
		name string
		pod  *corev1.Pod
		want *ackapis.MemoryLocality
		err  bool
	}

	tests := []args{
		{
			name: "no annotation",
			pod:  testingPod1,
			want: nil,
			err:  false,
		},
		{
			name: "policy is set",
			pod:  testingPod2,
			want: &ackapis.MemoryLocality{Policy: ackapis.MemoryLocalityPolicyBesteffort.Pointer()},
			err:  false,
		},
		{
			name: "policy and config are set",
			pod:  testingPod3,
			want: &ackapis.MemoryLocality{Policy: ackapis.MemoryLocalityPolicyBesteffort.Pointer(), MemoryLocalityConfig: ackapis.MemoryLocalityConfig{MigrateIntervalMinutes: pointer.Int64Ptr(1)}},
			err:  false,
		},
		{
			name: "wrong config",
			pod:  testingPod4,
			want: &ackapis.MemoryLocality{},
			err:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetPodMemoryLocality(tt.pod)
			if tt.err {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetPodMemoryLocalityByQoSClass(t *testing.T) {
	testingStrategy := util.DefaultMemoryLocalityStrategy()
	testingStrategy.LSClass.MemoryLocality.Policy = ackapis.MemoryLocalityPolicyBesteffort.Pointer()
	testingStrategy.LSClass.MemoryLocality.MigrateIntervalMinutes = pointer.Int64Ptr(1)

	testingPod1 := createPod(corev1.PodQOSBestEffort)

	testingPod2 := createPod(corev1.PodQOSBestEffort)
	testingPod2.Labels = map[string]string{apiext.LabelPodQoS: string(apiext.QoSLS)}

	testingPod3 := createPod(corev1.PodQOSBestEffort)
	testingPod3.Labels = map[string]string{apiext.LabelPodQoS: string(apiext.QoSSystem)}

	type args struct {
		name string
		pod  *corev1.Pod
		want *ackapis.MemoryLocality
	}

	tests := []args{
		{
			name: "will not use kube qos",
			pod:  testingPod1,
			want: nil,
		},
		{
			name: "set LS qos",
			pod:  testingPod2,
			want: testingStrategy.LSClass.MemoryLocality,
		},
		{
			name: "set system qos",
			pod:  testingPod3,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPodMemoryLocalityByQoSClass(tt.pod, testingStrategy)
			assert.Equal(t, tt.want, got)
		})
	}

}

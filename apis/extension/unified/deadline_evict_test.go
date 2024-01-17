package unified

import (
	"fmt"
	"k8s.io/utils/pointer"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetPodDeadlineEvictStrategy(t *testing.T) {
	second := metav1.Duration{
		Duration: time.Second,
	}
	cases := []struct {
		name          string
		pod           *corev1.Pod
		expectedError bool
		expected      *DeadlineEvictStrategy
	}{
		{
			name:          "nil pod",
			pod:           nil,
			expectedError: false,
			expected:      nil,
		},
		{
			name: "no annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectedError: false,
			expected:      nil,
		},
		{
			name: "annotation exists but invalid JSON",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationPodDeadlineEvictKey: "{invalid json}",
					},
				},
			},
			expectedError: true,
			expected:      nil,
		},
		{
			name: "valid annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationPodDeadlineEvictKey: fmt.Sprintf(`{"enable":true,"deadlineDuration":"1s"}`),
					},
				},
			},
			expectedError: false,
			expected: &DeadlineEvictStrategy{
				Enable: pointer.Bool(true),
				DeadlineEvictConfig: DeadlineEvictConfig{
					DeadlineDuration: &second,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deadlineEvictStrategy, err := GetPodDeadlineEvictStrategy(c.pod)
			if c.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expected, deadlineEvictStrategy)
			}
		})
	}
}

func TestGetPodDurationBeforeEviction(t *testing.T) {
	second := time.Second
	cases := []struct {
		name          string
		pod           *corev1.Pod
		expectedError bool
		expected      *time.Duration
	}{
		{
			name:          "nil pod",
			pod:           nil,
			expectedError: false,
			expected:      nil,
		},
		{
			name: "no annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectedError: false,
			expected:      nil,
		},
		{
			name: "annotation exists but invalid duration format",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationPodDurationBeforeEvictionKey: "invalid duration format",
					},
				},
			},
			expectedError: true,
			expected:      nil,
		},
		{
			name: "valid annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationPodDurationBeforeEvictionKey: time.Second.String(),
					},
				},
			},
			expectedError: false,
			expected:      &second,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			durationBeforeEviction, err := GetPodDurationBeforeEviction(c.pod)
			if c.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, c.expected, durationBeforeEviction)
			}
		})
	}
}

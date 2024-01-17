package sloconfig

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

var (
	defaultDeadlineDuration = metav1.Duration{
		Duration: time.Hour * 24,
	}
)

func init() {
	RegisterDefaultExtensionsMap(unified.DeadlineEvictExtKey, &unified.DeadlineEvictStrategy{
		Enable: pointer.Bool(false),
		DeadlineEvictConfig: unified.DeadlineEvictConfig{
			DeadlineDuration: &defaultDeadlineDuration,
		},
	})
}

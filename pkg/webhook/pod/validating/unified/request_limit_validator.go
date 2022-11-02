package unified

import (
	corev1 "k8s.io/api/core/v1"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/pkg/webhook/pod/validating"
)

func init() {
	validating.RegisterContainerFilterFunc(func(container *corev1.Container) bool {
		return !extunified.IsContainerIgnoreResource(container)
	})
}

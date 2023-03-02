package transformer

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	syncerconsts "sigs.k8s.io/cluster-api-provider-nested/virtualcluster/pkg/syncer/constants"
)

func TestTransformTenantPod(t *testing.T) {
	tests := []struct {
		name           string
		ownerReference []metav1.OwnerReference
	}{
		{
			name: "normal tenant pod",
			ownerReference: []metav1.OwnerReference{
				{
					APIVersion:         "apps/v1",
					Kind:               "ReplicaSet",
					Name:               "test-pod",
					UID:                "225fdca4-1205-492a-9d99-a23dc79f2957",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ownerReferenceJSON, err := json.Marshal(tt.ownerReference)
			assert.NoError(t, err)
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						syncerconsts.LabelOwnerReferences: string(ownerReferenceJSON),
					},
				},
			}
			TransformTenantPod(pod)
			assert.Equal(t, tt.ownerReference, pod.OwnerReferences)
		})
	}
}

package unified

import (
	"encoding/json"

	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ACSType = "virtual-cluster-node"

	AnnotationTenancyOwnerReferences = "tenancy.x-k8s.io/ownerReferences"
)

func IsACSVirtualNode(labels map[string]string) bool {
	return labels[uniext.LabelNodeType] == ACSType
}

func GetTenancyOwnerReferences(annotations map[string]string) ([]metav1.OwnerReference, error) {
	val, ok := annotations[AnnotationTenancyOwnerReferences]
	if !ok {
		return nil, nil
	}
	var ownerReferences []metav1.OwnerReference
	err := json.Unmarshal([]byte(val), &ownerReferences)
	if err != nil {
		return nil, err
	}
	return ownerReferences, nil
}

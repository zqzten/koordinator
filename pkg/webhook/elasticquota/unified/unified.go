package unified

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/ack"
	utilclient "github.com/koordinator-sh/koordinator/pkg/util/client"
	"github.com/koordinator-sh/koordinator/pkg/webhook/elasticquota"
)

func init() {
	elasticquota.GetQuotaName = GetQuotaName
}

func GetQuotaName(pod *corev1.Pod, clientImpl client.Client) string {
	quotaName := extension.GetQuotaName(pod)
	if quotaName != "" {
		return quotaName
	}

	eq := &v1alpha1.ElasticQuota{}
	err := clientImpl.Get(context.TODO(), types.NamespacedName{Namespace: pod.Namespace, Name: pod.Namespace}, eq)
	if err == nil {
		return eq.Name
	} else if !errors.IsNotFound(err) {
		klog.Errorf("Failed to Get ElasticQuota %s, err: %v", pod.Namespace, err)
	}

	eqList := &v1alpha1.ElasticQuotaList{}
	err = clientImpl.List(context.TODO(), eqList, utilclient.DisableDeepCopy)
	if err != nil {
		return extension.DefaultQuotaName
	}

	for _, eq := range eqList.Items {
		namespaces := strings.Split(eq.Annotations[ack.AnnotationQuotaNamespaces], ",")
		for _, namespace := range namespaces {
			if pod.Namespace == namespace {
				return namespace
			}
		}
	}

	return extension.DefaultQuotaName
}

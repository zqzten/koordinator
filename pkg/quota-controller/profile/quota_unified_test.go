package profile

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	schedv1alpha1 "sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	quotav1alpha1 "github.com/koordinator-sh/koordinator/apis/quota/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/util/transformer"
)

func createResourceListWithBatch(cpu, mem, batchCPU, batchMemory int64) corev1.ResourceList {
	return corev1.ResourceList{
		// use NewMilliQuantity to calculate the runtimeQuota correctly in cpu dimension
		// when the request is smaller than 1 core.
		corev1.ResourceCPU:    *resource.NewMilliQuantity(cpu*1000, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(mem, resource.BinarySI),
		extension.BatchCPU:    *resource.NewQuantity(batchCPU, resource.DecimalSI),
		extension.BatchMemory: *resource.NewQuantity(batchMemory, resource.BinarySI),
	}
}

func TestBatchQuotaProfileReconciler_Reconciler_CreateQuota(t *testing.T) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	quotav1alpha1.AddToScheme(scheme)
	schedv1alpha1.AddToScheme(scheme)

	nodes := []*corev1.Node{
		defaultCreateNode("node1", map[string]string{"topology.kubernetes.io/zone": "cn-hangzhou-a"}, createResourceListWithBatch(10, 1000, 2000, 4000)),
		defaultCreateNode("node2", map[string]string{"topology.kubernetes.io/zone": "cn-hangzhou-a"}, createResourceListWithBatch(10, 1000, 2000, 4000)),
		defaultCreateNode("node3", map[string]string{"topology.kubernetes.io/zone": "cn-hangzhou-b"}, createResourceListWithBatch(10, 1000, 2000, 4000)),
	}

	profile := &quotav1alpha1.ElasticQuotaProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: "batchProfile",
			Labels: map[string]string{
				transformer.LabelInstanceType: transformer.BestEffortInstanceType,
			},
		},
		Spec: quotav1alpha1.ElasticQuotaProfileSpec{
			QuotaName: "batchProfile-root",
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"topology.kubernetes.io/zone": "cn-hangzhou-a"},
			},
		},
	}

	r := &QuotaProfileReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
		Scheme: scheme,
	}
	// create node
	for _, node := range nodes {
		nodeCopy := node.DeepCopy()
		err := r.Client.Create(context.TODO(), nodeCopy)
		assert.NoError(t, err)
	}

	profileReq := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}}

	err := r.Client.Create(context.TODO(), profile)
	assert.NoError(t, err)
	r.Reconcile(context.TODO(), profileReq)
	quota := &schedv1alpha1.ElasticQuota{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{Namespace: profile.Namespace, Name: profile.Spec.QuotaName}, quota)
	assert.NoError(t, err)

	total := corev1.ResourceList{}
	err = json.Unmarshal([]byte(quota.Annotations[extension.AnnotationTotalResource]), &total)
	assert.NoError(t, err)

	assert.True(t, quotav1.Equals(createResourceList(4, 8000), quota.Spec.Min))
	assert.True(t, quotav1.Equals(createResourceListWithBatch(4, 8000, 4000, 8000), total))
}

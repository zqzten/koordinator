package volumebinding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	v1 "k8s.io/client-go/listers/storage/v1"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/unified/helper/eci"
)

func makeAffinityECIPod(pvcs []*corev1.PersistentVolumeClaim) *corev1.Pod {
	pod := makePod(pvcs)
	if pod == nil {
		return nil
	}
	if pod.Labels == nil {
		pod.Labels = map[string]string{}
	}
	pod.Labels[uniext.LabelECIAffinity] = uniext.ECIRequired
	return pod
}

func TestGetDefaultStorageClassIfVirtualKubelet(t *testing.T) {
	tests := []struct {
		name                string
		node                *corev1.Node
		pod                 *corev1.Pod
		storageClassName    string
		defaultStorageClass string
		want                string
	}{
		{
			name:                "Pod doesn't affinity ECI, storageClass is the same as origin",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			pod:                 makePod([]*corev1.PersistentVolumeClaim{makePVC("test-pvc", "", lvmSC.Name)}),
			storageClassName:    lvmSC.Name,
			defaultStorageClass: otherSC.Name,
			want:                lvmSC.Name,
		},
		{
			name:                "Node is not virtual kubelet, storageClass is the same as origin",
			node:                &corev1.Node{},
			pod:                 makeAffinityECIPod([]*corev1.PersistentVolumeClaim{makePVC("test-pvc", "", lvmSC.Name)}),
			storageClassName:    lvmSC.Name,
			defaultStorageClass: otherSC.Name,
			want:                lvmSC.Name,
		},
		{
			name:                "storageClass doesn't SupportLocalPV and SupportLVMOrQuotaPathOrDevice, storageClass is the same as origin",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			pod:                 makeAffinityECIPod([]*corev1.PersistentVolumeClaim{makePVC("test-pvc", "", cloudSC1.Name)}),
			storageClassName:    cloudSC1.Name,
			defaultStorageClass: otherSC.Name,
			want:                cloudSC1.Name,
		},
		{
			name:                "defaultStorageClass is nil, storageClass is the same as origin",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			pod:                 makeAffinityECIPod([]*corev1.PersistentVolumeClaim{makePVC("test-pvc", "", lvmSC.Name)}),
			storageClassName:    lvmSC.Name,
			defaultStorageClass: "",
			want:                lvmSC.Name,
		},
		{
			name:                "storageClass SupportLocalPV, storageClass is the same as default",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			pod:                 makeAffinityECIPod([]*corev1.PersistentVolumeClaim{makePVC("test-pvc", "", localPVSC.Name)}),
			storageClassName:    localPVSC.Name,
			defaultStorageClass: otherSC.Name,
			want:                otherSC.Name,
		},
		{
			name:                "storageClass SupportLVMOrQuotaPathOrDevice, storageClass is the same as default",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			pod:                 makeAffinityECIPod([]*corev1.PersistentVolumeClaim{makePVC("test-pvc", "", lvmSC.Name)}),
			storageClassName:    lvmSC.Name,
			defaultStorageClass: otherSC.Name,
			want:                otherSC.Name,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eci.DefaultECIProfile.DefaultStorageClass = tt.defaultStorageClass
			client := &fake.Clientset{}
			informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
			classInformer := informerFactory.Storage().V1().StorageClasses()
			classes := []*storagev1.StorageClass{localPVSC, lvmSC, quotaPathIOLimitSC, otherSC}
			for _, class := range classes {
				if err := classInformer.Informer().GetIndexer().Add(class); err != nil {
					t.Fatalf("Failed to add storage class to internal cache: %v", err)
				}
			}
			got := GetDefaultStorageClassIfVirtualKubelet(tt.node, tt.pod, tt.storageClassName, classInformer.Lister())
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestReplaceStorageClassIfVirtualKubelet(t *testing.T) {
	tests := []struct {
		name                string
		node                *corev1.Node
		affinityECI         bool
		pvc                 *corev1.PersistentVolumeClaim
		defaultStorageClass string
		classLister         v1.StorageClassLister
		want                string
	}{
		{
			name:                "Pod doesn't affinity ECI, storageClass is the same as origin",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			affinityECI:         false,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                lvmSC.Name,
		},
		{
			name:                "Node is not virtual kubelet, storageClass is the same as origin",
			node:                &corev1.Node{},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                lvmSC.Name,
		},
		{
			name:                "storageClass doesn't SupportLocalPV and SupportLVMOrQuotaPathOrDevice, storageClass is the same as origin",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", cloudSC1.Name),
			defaultStorageClass: otherSC.Name,
			want:                cloudSC1.Name,
		},
		{
			name:                "defaultStorageClass is nil, storageClass is the same as origin",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: "",
			want:                lvmSC.Name,
		},
		{
			name:                "storageClass SupportLocalPV, storageClass is the same as default",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", localPVSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                otherSC.Name,
		},
		{
			name:                "storageClass is '', storageClass is the same as default",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", ""),
			defaultStorageClass: otherSC.Name,
			want:                "",
		},
		{
			name:                "storageClass SupportLVMOrQuotaPathOrDevice, storageClass is the same as default",
			node:                &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{uniext.LabelNodeType: uniext.VKType}}},
			affinityECI:         true,
			pvc:                 makePVC("test-pvc", "", lvmSC.Name),
			defaultStorageClass: otherSC.Name,
			want:                otherSC.Name,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pod *corev1.Pod
			if tt.affinityECI {
				pod = makeAffinityECIPod([]*corev1.PersistentVolumeClaim{tt.pvc})
			} else {
				pod = makePod([]*corev1.PersistentVolumeClaim{tt.pvc})
			}
			eci.DefaultECIProfile.DefaultStorageClass = tt.defaultStorageClass
			client := &fake.Clientset{}
			informerFactory := informers.NewSharedInformerFactory(client, controller.NoResyncPeriodFunc())
			classInformer := informerFactory.Storage().V1().StorageClasses()
			classes := []*storagev1.StorageClass{localPVSC, lvmSC, quotaPathIOLimitSC, otherSC}
			for _, class := range classes {
				if err := classInformer.Informer().GetIndexer().Add(class); err != nil {
					t.Fatalf("Failed to add storage class to internal cache: %v", err)
				}
			}
			replacedPVC := ReplaceStorageClassIfVirtualKubelet(tt.node, pod, tt.pvc, classInformer.Lister())
			assert.Equal(t, tt.want, storagehelpers.GetPersistentVolumeClaimClass(replacedPVC))
		})
	}
}

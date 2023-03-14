package unified

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
	kruisepolicyv1alpha1 "github.com/openkruise/kruise-api/policy/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/controllerfinder"
)

func TestControllerFinder_GetPodsForRef(t *testing.T) {
	tests := []struct {
		name             string
		controllerFinder controllerfinder.Interface
		pub              *kruisepolicyv1alpha1.PodUnavailableBudget
		actualPodNum     int
		wantReplicas     int
		ownerReference   *metav1.OwnerReference
	}{
		{
			name: "from controller",
			controllerFinder: &fakeControllerFinder{
				err:      nil,
				replicas: 3,
			},
			actualPodNum: 3,
			wantReplicas: 3,
			ownerReference: &metav1.OwnerReference{
				APIVersion: controllerfinder.ControllerKindRS.Group + "/" + controllerfinder.ControllerKindRS.Version,
				Kind:       controllerfinder.ControllerKindRS.Kind,
				Name:       "test",
				UID:        "test-uid",
			},
		},
		{
			name: "same owner pods, scale from pub",
			controllerFinder: &fakeControllerFinder{
				err:      nil,
				replicas: 0,
			},
			actualPodNum: 3,
			wantReplicas: 4,
			pub: &kruisepolicyv1alpha1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pub-1",
					Annotations: map[string]string{
						kruisepolicyv1alpha1.PubProtectTotalReplicas: "4",
					},
				},
			},
			ownerReference: &metav1.OwnerReference{
				APIVersion: controllerfinder.ControllerKindDep.Group + "/" + controllerfinder.ControllerKindDep.Version,
				Kind:       controllerfinder.ControllerKindDep.Kind,
				Name:       "test",
				UID:        "test-uid",
			},
		},
		{
			name: "same owner pods, scale from actual",
			controllerFinder: &fakeControllerFinder{
				err:      nil,
				replicas: 0,
			},
			actualPodNum: 3,
			wantReplicas: 3,
			ownerReference: &metav1.OwnerReference{
				APIVersion: controllerfinder.ControllerKruiseKindCS.Group + "/" + controllerfinder.ControllerKruiseKindCS.Version,
				Kind:       controllerfinder.ControllerKruiseKindCS.Kind,
				Name:       "test",
				UID:        "test-uid",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = sev1alpha1.AddToScheme(scheme)
			_ = clientgoscheme.AddToScheme(scheme)
			_ = appsv1alpha1.AddToScheme(scheme)
			_ = appsv1beta1.AddToScheme(scheme)
			_ = kruisepolicyv1alpha1.AddToScheme(scheme)
			runtimeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			if tt.pub != nil {
				assert.NoError(t, runtimeClient.Create(context.TODO(), tt.pub))
			}
			var pods []*corev1.Pod
			for i := 0; i < tt.actualPodNum; i++ {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("test-pod-%d", i),
						Annotations: map[string]string{
							PodRelatedPubAnnotation: "pub-1",
						},
						OwnerReferences: []metav1.OwnerReference{
							*tt.ownerReference,
						},
					},
				}
				pods = append(pods, pod)
				assert.NoError(t, runtimeClient.Create(context.TODO(), pod))
			}
			if tt.controllerFinder != nil {
				if tt.controllerFinder.(*fakeControllerFinder).replicas != 0 {
					tt.controllerFinder.(*fakeControllerFinder).podsFromController = pods
				}
				tt.controllerFinder.(*fakeControllerFinder).podsFromActual = pods
			}
			s := &ControllerFinder{
				Interface: tt.controllerFinder,
				Client:    runtimeClient,
			}
			gotPods, gotReplicas, err := s.GetPodsForRef(tt.ownerReference, "default", nil, false)
			assert.NoError(t, err)
			if !reflect.DeepEqual(gotPods, pods) {
				t.Errorf("GetPodsForRef() gotPods = %v, wantPods %v", gotPods, pods)
			}
			if gotReplicas != int32(tt.wantReplicas) {
				t.Errorf("GetPodsForRef() gotReplicas = %v, wantReplicas %v", gotReplicas, tt.wantReplicas)
			}
		})
	}

}

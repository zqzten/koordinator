package unified

import (
	"context"
	"fmt"
	"testing"

	appsv1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	appsv1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
	kruisepolicyv1alpha1 "github.com/openkruise/kruise-api/policy/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sev1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/descheduler/controllers/migration/controllerfinder"
)

var (
	_ controllerfinder.Interface = &fakeControllerFinder{}
)

type fakeControllerFinder struct {
	replicas           int32
	podsFromController []*corev1.Pod
	podsFromActual     []*corev1.Pod
	err                error
	pub                *kruisepolicyv1alpha1.PodUnavailableBudget
}

func (f *fakeControllerFinder) GetPodsForRef(ref *metav1.OwnerReference, namespace string, labelSelector *metav1.LabelSelector, active bool) ([]*corev1.Pod, int32, error) {
	return f.podsFromController, f.replicas, nil
}

func (f *fakeControllerFinder) GetExpectedScaleForPod(pods *corev1.Pod) (int32, error) {
	return f.replicas, f.err
}

func (f *fakeControllerFinder) ListPodsByWorkloads(workloadUIDs []types.UID, ns string, labelSelector *metav1.LabelSelector, active bool) ([]*corev1.Pod, error) {
	return f.podsFromActual, nil
}

func TestControllerFinder_GetExpectedScaleForPod(t *testing.T) {
	tests := []struct {
		name             string
		controllerFinder controllerfinder.Interface
		pod              *corev1.Pod
		pub              *kruisepolicyv1alpha1.PodUnavailableBudget
		want             int32
		wantErr          bool
	}{
		{
			name:    "pod is nil",
			pod:     nil,
			want:    0,
			wantErr: false,
		},
		{
			name: "from controller",
			controllerFinder: &fakeControllerFinder{
				err:      nil,
				replicas: 10,
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						PodRelatedPubAnnotation: "pub-1",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "",
							Kind:       "",
							Name:       "",
							UID:        "test-uid",
						},
					},
				},
			},
			want:    10,
			wantErr: false,
		},
		{
			name: "from pub",
			controllerFinder: &fakeControllerFinder{
				replicas: 0,
			},
			pub: &kruisepolicyv1alpha1.PodUnavailableBudget{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pub-1",
					Annotations: map[string]string{
						kruisepolicyv1alpha1.PubProtectTotalReplicas: "10",
					},
				},
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						PodRelatedPubAnnotation: "pub-1",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "",
							Kind:       "",
							Name:       "",
							UID:        "test-uid",
						},
					},
				},
			},
			want:    10,
			wantErr: false,
		},
		{
			name: "from actual pod num",
			controllerFinder: &fakeControllerFinder{
				replicas: 0,
			},
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						PodRelatedPubAnnotation: "pub-1",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							UID:        "test-uid",
							Controller: pointer.Bool(true),
						},
					},
				},
			},
			want:    10,
			wantErr: false,
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
			if tt.pod != nil {
				assert.NoError(t, runtimeClient.Create(context.TODO(), tt.pod))
				pods = append(pods, tt.pod)
				for i := 1; i < int(tt.want); i++ {
					pod := &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name: fmt.Sprintf("test-pod-%d", i),
							Annotations: map[string]string{
								PodRelatedPubAnnotation: tt.pod.Annotations[PodRelatedPubAnnotation],
							},
							OwnerReferences: []metav1.OwnerReference{
								{
									UID:        tt.pod.OwnerReferences[0].UID,
									Controller: pointer.Bool(true),
								},
							},
						},
					}
					pods = append(pods, pod)
					assert.NoError(t, runtimeClient.Create(context.TODO(), pod))
				}
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
			got, err := s.GetExpectedScaleForPod(tt.pod)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetExpectedScaleForPod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetExpectedScaleForPod() got = %v, want %v", got, tt.want)
			}
		})
	}
}

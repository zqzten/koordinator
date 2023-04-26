package transformer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	schedulingv1alpha1 "github.com/koordinator-sh/koordinator/apis/scheduling/v1alpha1"
	koordfake "github.com/koordinator-sh/koordinator/pkg/client/clientset/versioned/fake"
	koordinatorinformers "github.com/koordinator-sh/koordinator/pkg/client/informers/externalversions"
)

func TestInstallReservationTransformer(t *testing.T) {
	enableENIResourceTransform = true
	resources := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("4000m"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
	}
	updatedResources := corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(0, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(0, resource.BinarySI),
	}
	batchResources := corev1.ResourceList{
		extension.BatchCPU:    *resource.NewQuantity(4000, resource.DecimalSI),
		extension.BatchMemory: resource.MustParse("4Gi"),
	}
	sigmaENIResources := corev1.ResourceList{
		unified.ResourceSigmaENI: resource.MustParse("1"),
	}
	memberENIResources := corev1.ResourceList{
		unified.ResourceAliyunMemberENI: resource.MustParse("1"),
	}

	tests := []struct {
		name                string
		reservation         *schedulingv1alpha1.Reservation
		expectedReservation *schedulingv1alpha1.Reservation
	}{
		{
			name: "normal prod reservation",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "pod with SIGMA_IGNORE_RESOURCE",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{

							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
								{
									Env: []corev1.EnvVar{
										{
											Name:  unified.EnvSigmaIgnoreResource,
											Value: "true",
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
								{
									Env: []corev1.EnvVar{
										{
											Name:  unified.EnvSigmaIgnoreResource,
											Value: "true",
										},
									},
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: updatedResources.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "batch reservation",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Priority: pointer.Int32(extension.PriorityBatchValueMax),
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
								{
									Resources: corev1.ResourceRequirements{
										Limits:   resources.DeepCopy(),
										Requests: resources.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Priority: pointer.Int32(extension.PriorityBatchValueMax),
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   batchResources.DeepCopy(),
										Requests: batchResources.DeepCopy(),
									},
								},
								{
									Resources: corev1.ResourceRequirements{
										Limits:   batchResources.DeepCopy(),
										Requests: batchResources.DeepCopy(),
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "sigma eni reservation",
			reservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   sigmaENIResources,
										Requests: sigmaENIResources,
									},
								},
								{
									Resources: corev1.ResourceRequirements{
										Limits:   sigmaENIResources,
										Requests: sigmaENIResources,
									},
								},
							},
						},
					},
				},
			},
			expectedReservation: &schedulingv1alpha1.Reservation{
				ObjectMeta: metav1.ObjectMeta{Name: "test-reservation"},
				Spec: schedulingv1alpha1.ReservationSpec{
					Template: &corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Resources: corev1.ResourceRequirements{
										Limits:   memberENIResources,
										Requests: memberENIResources,
									},
								},
								{
									Resources: corev1.ResourceRequirements{
										Limits:   memberENIResources,
										Requests: memberENIResources,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			koordClient := koordfake.NewSimpleClientset()
			koordInformerFactory := koordinatorinformers.NewSharedInformerFactoryWithOptions(koordClient, 0)
			InstallReservationTransformer(koordInformerFactory.Scheduling().V1alpha1().Reservations().Informer())
			_, err := koordClient.SchedulingV1alpha1().Reservations().Create(ctx, tt.reservation, metav1.CreateOptions{})
			assert.NoError(t, err)
			koordInformerFactory.Start(ctx.Done())
			koordInformerFactory.WaitForCacheSync(ctx.Done())
			reservation, err := koordInformerFactory.Scheduling().V1alpha1().Reservations().Lister().Get("test-reservation")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedReservation.Spec.Template.Spec, reservation.Spec.Template.Spec)
		})
	}
}

package inplaceupdate

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	uniext "gitlab.alibaba-inc.com/unischeduler/api/apis/extension"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes/fake"

	extunified "github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func Test_reject(t *testing.T) {
	type args struct {
	}
	tests := []struct {
		name                      string
		resourceUpdateSpec        *uniext.ResourceUpdateSpec
		expectResourceUpdateState *uniext.ResourceUpdateSchedulerState
		targetPodName             string
		err                       error
		wantErr                   bool
	}{
		{
			name: "normal flow",
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
			},
			targetPodName: "test",
			err:           fmt.Errorf("some error"),
			expectResourceUpdateState: &uniext.ResourceUpdateSchedulerState{
				Version: "1",
				Status:  uniext.ResourceUpdateStateRejected,
				Reason:  "some error",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset()
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:   "default",
					Name:        tt.targetPodName,
					Annotations: map[string]string{},
				},
			}
			podClient := fakeClient.CoreV1().Pods(pod.Namespace)
			assert.NoError(t, uniext.SetResourceUpdateSpec(pod.Annotations, tt.resourceUpdateSpec))
			pod, err := podClient.Create(context.TODO(), pod, metav1.CreateOptions{})
			assert.NoError(t, err)
			if err := reject(podClient, pod, tt.resourceUpdateSpec.Version, tt.err); (err != nil) != tt.wantErr {
				t.Errorf("reject() error = %v, wantErr %v", err, tt.wantErr)
			}
			pod, err = podClient.Get(context.TODO(), tt.targetPodName, metav1.GetOptions{})
			assert.NoError(t, err)
			updateState, err := uniext.GetResourceUpdateSchedulerState(pod.Annotations)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectResourceUpdateState, updateState)
		})
	}
}

func Test_makeInplaceUpdatePod(t *testing.T) {
	originalUID := uuid.NewUUID()
	tests := []struct {
		name               string
		pod                *corev1.Pod
		resourceUpdateSpec *uniext.ResourceUpdateSpec
		want               *corev1.Pod
	}{
		{
			name: "normal flow",
			pod: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					UID:         originalUID,
					Annotations: map[string]string{},
					Name:        "original-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
							},
						},
						{
							Name: "main2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("3"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("4"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{},
			},
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
				Containers: []uniext.ContainerResource{
					{
						Name: "main1",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
					{
						Name: "main2",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
				},
			},
			want: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: string(originalUID + "-" + "1"),
					Annotations: map[string]string{
						extunified.AnnotationResourceUpdateTargetPodName: "original-pod",
						extunified.AnnotationResourceUpdateTargetPodUID:  string(originalUID),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
						{
							Name: "main2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{},
			},
		},
		{
			name: "only update one part of resource",
			pod: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					UID:         originalUID,
					Annotations: map[string]string{},
					Name:        "original-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
						{
							Name: "main2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("3"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{},
			},
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
				Containers: []uniext.ContainerResource{
					{
						Name: "main1",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
					{
						Name: "main2",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
				},
			},
			want: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: string(originalUID + "-" + "1"),
					Annotations: map[string]string{
						extunified.AnnotationResourceUpdateTargetPodName: "original-pod",
						extunified.AnnotationResourceUpdateTargetPodUID:  string(originalUID),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
						{
							Name: "main2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{},
			},
		},
		{
			name: "container not in resourceUpdateSpec",
			pod: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					UID:         originalUID,
					Annotations: map[string]string{},
					Name:        "original-pod",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
						{
							Name: "main1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
						{
							Name: "main2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("3"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("4"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{},
			},
			resourceUpdateSpec: &uniext.ResourceUpdateSpec{
				Version: "1",
				Containers: []uniext.ContainerResource{
					{
						Name: "main1",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
					{
						Name: "main2",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1"),
							},
						},
					},
				},
			},
			want: &corev1.Pod{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name: string(originalUID + "-" + "1"),
					Annotations: map[string]string{
						extunified.AnnotationResourceUpdateTargetPodName: "original-pod",
						extunified.AnnotationResourceUpdateTargetPodUID:  string(originalUID),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
						{
							Name: "main1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
						{
							Name: "main2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("100Gi"),
								},
							},
						},
					},
				},
				Status: corev1.PodStatus{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NoError(t, uniext.SetResourceUpdateSpec(tt.pod.Annotations, tt.resourceUpdateSpec))
			assert.NoError(t, uniext.SetResourceUpdateSpec(tt.want.Annotations, tt.resourceUpdateSpec))
			got := makeInplaceUpdatePod(tt.pod, tt.resourceUpdateSpec)
			assert.NotEqual(t, tt.pod.UID, got.UID)
			got.UID = ""
			assert.True(t, reflect.DeepEqual(got.ObjectMeta, tt.want.ObjectMeta))
			assert.True(t, reflect.DeepEqual(got.Spec, tt.want.Spec))
		})
	}
}

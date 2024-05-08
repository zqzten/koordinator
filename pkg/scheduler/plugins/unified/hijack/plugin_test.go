/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hijack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	kubeclientset "k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
)

type fakeExtendedHandle struct {
	frameworkext.ExtendedHandle
	clientset kubeclientset.Interface
	forgotPod *corev1.Pod
}

func (f *fakeExtendedHandle) ClientSet() kubeclientset.Interface {
	return f.clientset
}

func (f *fakeExtendedHandle) ForgetPod(logger klog.Logger, pod *corev1.Pod) error {
	f.forgotPod = pod
	return nil
}

func TestApplyPatch(t *testing.T) {
	hijackedPodUID := "123456"
	modifiedHijackedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hijacked-pod",
			Namespace: "default",
			UID:       types.UID(hijackedPodUID),
			Annotations: map[string]string{
				"test-a":                       "1",
				AnnotationContainerNameMapping: `{"main":"fake-main"}`,
			},
			Labels: map[string]string{
				"test-b": "2",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "fake-main",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("2"),
						},
					},
				},
			},
		},
	}

	tests := []struct {
		name                 string
		targetPod            *corev1.Pod
		hijackedPod          *corev1.Pod
		modifiedHijackedPod  *corev1.Pod
		expectedPod          *corev1.Pod
		expectedHijackedPods map[types.UID]*corev1.Pod
		wantStatus           *framework.Status
	}{
		{
			name: "apply patches from hijacked to target",
			targetPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-pod",
					Namespace: "default",
					Labels: map[string]string{
						apiext.LabelPodOperatingMode: string(apiext.ReservationPodOperatingMode),
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
						},
					},
				},
			},
			hijackedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hijacked-pod",
					Namespace: "default",
					UID:       types.UID(hijackedPodUID),
					Annotations: map[string]string{
						AnnotationContainerNameMapping: `{"main":"fake-main"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "fake-main",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("4"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			modifiedHijackedPod: modifiedHijackedPod,
			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-pod",
					Namespace: "default",
					Labels: map[string]string{
						"test-b":                     "2",
						apiext.LabelPodOperatingMode: string(apiext.ReservationPodOperatingMode),
					},
					Annotations: map[string]string{
						"test-a":                                 "1",
						AnnotationHijackedPod:                    `{"namespace":"default","name":"hijacked-pod","uid":"123456"}`,
						apiext.AnnotationReservationCurrentOwner: `{"namespace":"default","name":"hijacked-pod","uid":"123456"}`,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("4"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
							},
						},
					},
				},
			},
			expectedHijackedPods: map[types.UID]*corev1.Pod{
				types.UID(hijackedPodUID): modifiedHijackedPod,
			},
		},
		{
			name: "target pod has no changes",
			targetPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-pod",
					Namespace: "default",
					Labels: map[string]string{
						"alreadyExists": "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
						},
					},
				},
			},
			hijackedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hijacked-pod",
					Namespace: "default",
					UID:       types.UID(hijackedPodUID),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
						},
					},
				},
			},
			modifiedHijackedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hijacked-pod",
					Namespace: "default",
					UID:       types.UID(hijackedPodUID),
					Labels: map[string]string{
						"alreadyExists": "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
						},
					},
				},
			},
			expectedPod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "target-pod",
					Namespace: "default",
					Labels: map[string]string{
						"alreadyExists": "true",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "main",
						},
					},
				},
			},
			expectedHijackedPods: map[types.UID]*corev1.Pod{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := kubefake.NewSimpleClientset()
			pl := &Plugin{
				extendedHandle: &fakeExtendedHandle{
					clientset: clientset,
				},
				hijackedPods: map[types.UID]*corev1.Pod{},
			}

			_, err := clientset.CoreV1().Pods(tt.targetPod.Namespace).Create(context.TODO(), tt.targetPod, metav1.CreateOptions{})
			assert.NoError(t, err)

			cycleState := framework.NewCycleState()
			cycleState.Write(Name, &stateData{
				targetPod: tt.targetPod,
			})
			SetTargetPod(cycleState, tt.targetPod)

			status := pl.ApplyPatch(context.TODO(), cycleState, tt.hijackedPod, tt.modifiedHijackedPod)
			assert.Equal(t, tt.wantStatus, status)

			pod, err := clientset.CoreV1().Pods(tt.targetPod.Namespace).Get(context.TODO(), tt.targetPod.Name, metav1.GetOptions{})
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedPod, pod)
			assert.Equal(t, tt.expectedHijackedPods, pl.hijackedPods)
		})
	}
}

func TestBind(t *testing.T) {
	pl := &Plugin{}
	status := pl.Bind(context.TODO(), framework.NewCycleState(), &corev1.Pod{}, "xx")
	assert.Equal(t, framework.NewStatus(framework.Skip), status)

	cycleState := framework.NewCycleState()
	cycleState.Write(Name, &stateData{})
	SetTargetPod(cycleState, &corev1.Pod{})
	status = pl.Bind(context.TODO(), cycleState, &corev1.Pod{}, "yy")
	assert.Equal(t, framework.NewStatus(framework.Success), status)
}

func TestForgetPod(t *testing.T) {
	fh := &fakeExtendedHandle{}
	pl := &Plugin{
		extendedHandle: fh,
		hijackedPods:   map[types.UID]*corev1.Pod{},
	}
	hijackedPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hijacked-pod",
			Namespace: "default",
			UID:       uuid.NewUUID(),
		},
	}
	targetPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "target-pod",
			Namespace: "default",
			UID:       uuid.NewUUID(),
		},
	}
	assert.NoError(t, pl.assumeHijackedPod(targetPod, hijackedPod))
	assert.Equal(t, map[types.UID]*corev1.Pod{hijackedPod.UID: hijackedPod}, pl.hijackedPods)
	pl.onPodUpdate(targetPod, targetPod)
	assert.Equal(t, map[types.UID]*corev1.Pod{}, pl.hijackedPods)
	assert.Equal(t, hijackedPod, fh.forgotPod)

	assert.NoError(t, pl.assumeHijackedPod(targetPod, hijackedPod))
	assert.Equal(t, map[types.UID]*corev1.Pod{hijackedPod.UID: hijackedPod}, pl.hijackedPods)
	pl.onPodDelete(targetPod)
	assert.Equal(t, map[types.UID]*corev1.Pod{}, pl.hijackedPods)
	assert.Equal(t, hijackedPod, fh.forgotPod)
}

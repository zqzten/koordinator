package deadlineevict

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/pointer"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
	"github.com/koordinator-sh/koordinator/apis/slo/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/features"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/qosmanager/framework"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	mock_statesinformer "github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer/mockstatesinformer"
)

func TestParseNodeStrategy(t *testing.T) {
	type testCase struct {
		name           string
		nodeSLO        *v1alpha1.NodeSLO
		expectedResult *unified.DeadlineEvictStrategy
		expectedError  bool
	}

	cases := []testCase{
		{
			name: "nil NodeSLO",
			nodeSLO: func() *v1alpha1.NodeSLO {
				return nil
			}(),
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "empty NodeSLO.Extensions",
			nodeSLO: func() *v1alpha1.NodeSLO {
				return &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{},
					},
				}
			}(),
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "invalid DeadlineEvictStrategy type",
			nodeSLO: func() *v1alpha1.NodeSLO {
				return &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								"deadline-evict": "invalid-strategy-type",
							},
						},
					},
				}
			}(),
			expectedResult: nil,
			expectedError:  true,
		},
		{
			name: "valid DeadlineEvictStrategy",
			nodeSLO: func() *v1alpha1.NodeSLO {
				return &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: map[string]interface{}{
									"enable":           true,
									"deadlineDuration": "1m",
								},
							},
						},
					},
				}
			}(),
			expectedResult: &unified.DeadlineEvictStrategy{
				Enable: pointer.Bool(true),
				DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{
					Duration: time.Minute,
				},
				},
			},
			expectedError: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result, err := parseNodeStrategy(c.nodeSLO)
			assert.Equal(t, c.expectedResult, result)
			assert.Equal(t, c.expectedError, err != nil)
		})
	}
}

func TestGetPodDeadline(t *testing.T) {
	type testCase struct {
		name             string
		pod              *corev1.Pod
		deadlineDuration metav1.Duration
		expectedResult   *time.Time
	}

	now := time.Now()
	timeNow = func() time.Time {
		return now
	}
	pointerTime := func(t time.Time) *time.Time {
		return &t
	}
	cases := []testCase{
		{
			name:             "nil pod",
			pod:              nil,
			deadlineDuration: metav1.Duration{Duration: time.Minute},
			expectedResult:   nil,
		},
		{
			name: "pod with nil startTime",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			},
			deadlineDuration: metav1.Duration{Duration: time.Minute},
			expectedResult:   nil,
		},
		{
			name: "pod without durationBeforeEviction",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
				Status: corev1.PodStatus{
					StartTime: &metav1.Time{Time: timeNow().Add(-time.Second)},
				},
			},
			deadlineDuration: metav1.Duration{Duration: time.Minute * 30},
			expectedResult:   pointerTime(timeNow().Add(-time.Second).Add(time.Minute * 30).Add(-unified.DefaultPodDurationBeforeEviction)),
		},
		{
			name: "pod with specified durationBeforeEviction",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						unified.AnnotationPodDurationBeforeEvictionKey: "5m",
					},
				},
				Status: corev1.PodStatus{
					StartTime: &metav1.Time{Time: timeNow().Add(-time.Second)},
				},
			},
			deadlineDuration: metav1.Duration{Duration: time.Minute * 30},
			expectedResult:   pointerTime(timeNow().Add(-time.Second).Add(time.Minute * 30).Add(-time.Minute * 5)),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := getPodDeadline(c.pod, c.deadlineDuration)
			if c.expectedResult == nil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Equal(t, c.expectedResult.Format(time.RFC3339), result.Format(time.RFC3339))
			}
		})
	}
}

func TestEvictPod(t *testing.T) {
	tests := []struct {
		name           string
		pod            *corev1.Pod
		fakeClientset  *fakeclientset.Clientset
		expectedError  bool
		expecedUpdated bool
		expectedResult *corev1.Pod
	}{
		{
			name:           "nil pod",
			pod:            nil,
			fakeClientset:  fakeclientset.NewSimpleClientset(),
			expectedError:  false,
			expecedUpdated: false,
			expectedResult: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
		},
		{
			name: "patch success",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			fakeClientset:  fakeclientset.NewSimpleClientset(),
			expectedError:  false,
			expecedUpdated: true,
			expectedResult: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						unified.LabelEvictionKey: unified.LabelEvictionValue,
					},
					Annotations: map[string]string{
						unified.AnnotationEvictionTypeKey:            `["involuntary"]`,
						unified.AnnotationSkipNotReadyFlowControlKey: unified.AnnotationSkipNotReadyFlowControlValue,
						unified.AnnotationEvictionMessageKey:         "test-msg",
					},
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
		},
		{
			name: "skip patch with already evicted",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						unified.LabelEvictionKey: unified.LabelEvictionValue,
					},
					Annotations: map[string]string{
						unified.AnnotationEvictionTypeKey:            `["involuntary"]`,
						unified.AnnotationSkipNotReadyFlowControlKey: unified.AnnotationSkipNotReadyFlowControlValue,
						unified.AnnotationEvictionMessageKey:         "test-msg",
					},
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			fakeClientset:  fakeclientset.NewSimpleClientset(),
			expectedError:  false,
			expecedUpdated: false,
			expectedResult: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels: map[string]string{
						unified.LabelEvictionKey: unified.LabelEvictionValue,
					},
					Annotations: map[string]string{
						unified.AnnotationEvictionTypeKey:            `["involuntary"]`,
						unified.AnnotationSkipNotReadyFlowControlKey: unified.AnnotationSkipNotReadyFlowControlValue,
						unified.AnnotationEvictionMessageKey:         "test-msg",
					},
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
		},
		{
			name: "patch failure",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			fakeClientset:  fakeclientset.NewSimpleClientset(),
			expectedError:  true,
			expecedUpdated: false,
			expectedResult: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pod != nil {
				_, err := tt.fakeClientset.CoreV1().Pods("default").Create(context.TODO(), tt.pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}

			if tt.expectedError {
				tt.fakeClientset.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("patch", "pods", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &corev1.Pod{}, errors.New("Error patch pod")
				})
			}

			acsEvictor := newACSEvictor(tt.fakeClientset)
			updated, err := acsEvictor.evictPod(tt.pod, "test-msg")
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expecedUpdated, updated)
			if tt.pod != nil {
				patchedPod, err := tt.fakeClientset.CoreV1().Pods(tt.pod.Namespace).Get(context.TODO(), tt.pod.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, tt.expectedResult, patchedPod)
			}
		})
	}
}

func TestGetMergedDeadlineEvictStrategy(t *testing.T) {
	cases := []struct {
		name             string
		pod              *corev1.Pod
		nodeStrategy     *unified.DeadlineEvictStrategy
		expectedStrategy *unified.DeadlineEvictStrategy
	}{
		{
			name:             "nil node and pod strategies",
			pod:              &corev1.Pod{},
			nodeStrategy:     nil,
			expectedStrategy: nil,
		},
		{
			name:             "node strategy without deadline config",
			pod:              &corev1.Pod{},
			nodeStrategy:     &unified.DeadlineEvictStrategy{Enable: pointer.Bool(true)},
			expectedStrategy: &unified.DeadlineEvictStrategy{Enable: pointer.Bool(true)},
		},
		{
			name: "pod strategy without deadline config",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						unified.AnnotationPodDeadlineEvictKey: `{"enable":true}`,
					},
				},
			},
			nodeStrategy:     &unified.DeadlineEvictStrategy{Enable: pointer.Bool(false)},
			expectedStrategy: &unified.DeadlineEvictStrategy{Enable: pointer.Bool(true)},
		},
		{
			name: "both node and pod strategies have deadline config",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						unified.AnnotationPodDeadlineEvictKey: `{"enable":true,"deadlineDuration":"30m"}`,
					},
				},
			},
			nodeStrategy:     &unified.DeadlineEvictStrategy{Enable: pointer.Bool(false), DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Minute}}},
			expectedStrategy: &unified.DeadlineEvictStrategy{Enable: pointer.Bool(true), DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 30 * time.Minute}}},
		},
		{
			name: "valid pod strategy with deadline config",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						unified.AnnotationPodDeadlineEvictKey: `{"enable":true,"deadlineDuration":"30h"}`,
					},
				},
			},
			nodeStrategy:     nil,
			expectedStrategy: &unified.DeadlineEvictStrategy{Enable: pointer.Bool(true), DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 30 * time.Hour}}},
		},
		{
			name: "node strategy contains deadlineEvictConfig and enable, pod does not contain deadlineEvictConfig but has different enable value",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						unified.AnnotationPodDeadlineEvictKey: `{"enable":true}`,
					},
				},
			},
			nodeStrategy:     &unified.DeadlineEvictStrategy{Enable: pointer.Bool(false), DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Hour}}},
			expectedStrategy: &unified.DeadlineEvictStrategy{Enable: pointer.Bool(true), DeadlineEvictConfig: unified.DeadlineEvictConfig{DeadlineDuration: &metav1.Duration{Duration: 60 * time.Hour}}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			strategy := getMergedDeadlineEvictStrategy(c.pod, c.nodeStrategy)
			assert.Equal(t, c.expectedStrategy, strategy)
		})
	}
}

func Test_deadlineEvict_deadlineEvict(t *testing.T) {
	now := time.Now()
	timeNow = func() time.Time {
		return now
	}
	type fields struct {
		pods    []*statesinformer.PodMeta
		nodeSLO *v1alpha1.NodeSLO
	}
	type wants struct {
		patchedPods []*corev1.Pod
	}
	tests := []struct {
		name   string
		fields fields
		wants  wants
	}{
		{
			name: "node slo is nil",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Status: corev1.PodStatus{
								StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
							},
						},
					},
				},
				nodeSLO: nil,
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS: string(extension.QoSBE),
							},
						},
						Status: corev1.PodStatus{
							StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
						},
					},
				},
			},
		},
		{
			name: "node slo with bad format",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Status: corev1.PodStatus{
								StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
							},
						},
					},
				},
				nodeSLO: &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: "bad format",
							},
						},
					},
				},
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS: string(extension.QoSBE),
							},
						},
						Status: corev1.PodStatus{
							StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
						},
					},
				},
			},
		},
		{
			name: "skip ls pod",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSLS),
								},
							},
							Status: corev1.PodStatus{
								StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
							},
						},
					},
				},
				nodeSLO: &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: &unified.DeadlineEvictStrategy{
									Enable: pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{
										DeadlineDuration: &metav1.Duration{Duration: 35 * time.Hour},
									},
								},
							},
						},
					},
				},
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS: string(extension.QoSLS),
							},
						},
						Status: corev1.PodStatus{
							StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
						},
					},
				},
			},
		},
		{
			name: "strategy is not enabled",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Status: corev1.PodStatus{
								StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
							},
						},
					},
				},
				nodeSLO: &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: &unified.DeadlineEvictStrategy{
									Enable: pointer.Bool(false),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{
										DeadlineDuration: &metav1.Duration{Duration: 35 * time.Hour},
									},
								},
							},
						},
					},
				},
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS: string(extension.QoSBE),
							},
						},
						Status: corev1.PodStatus{
							StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
						},
					},
				},
			},
		},
		{
			name: "pod has no start time",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Status: corev1.PodStatus{},
						},
					},
				},
				nodeSLO: &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: &unified.DeadlineEvictStrategy{
									Enable: pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{
										DeadlineDuration: &metav1.Duration{Duration: 35 * time.Hour},
									},
								},
							},
						},
					},
				},
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS: string(extension.QoSBE),
							},
						},
						Status: corev1.PodStatus{},
					},
				},
			},
		},
		{
			name: "pod has not reach deadline",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Status: corev1.PodStatus{
								StartTime: &metav1.Time{Time: timeNow().Add(-34 * time.Hour)},
							},
						},
					},
				},
				nodeSLO: &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: &unified.DeadlineEvictStrategy{
									Enable: pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{
										DeadlineDuration: &metav1.Duration{Duration: 35 * time.Hour},
									},
								},
							},
						},
					},
				},
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS: string(extension.QoSBE),
							},
						},
						Status: corev1.PodStatus{
							StartTime: &metav1.Time{Time: timeNow().Add(-34 * time.Hour)},
						},
					},
				},
			},
		},
		{
			name: "evict pod reach deadline",
			fields: fields{
				pods: []*statesinformer.PodMeta{
					{
						Pod: &corev1.Pod{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-be-pod",
								Namespace: "default",
								Labels: map[string]string{
									extension.LabelPodQoS: string(extension.QoSBE),
								},
							},
							Status: corev1.PodStatus{
								StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
							},
						},
					},
				},
				nodeSLO: &v1alpha1.NodeSLO{
					Spec: v1alpha1.NodeSLOSpec{
						Extensions: &v1alpha1.ExtensionsMap{
							Object: map[string]interface{}{
								unified.DeadlineEvictExtKey: &unified.DeadlineEvictStrategy{
									Enable: pointer.Bool(true),
									DeadlineEvictConfig: unified.DeadlineEvictConfig{
										DeadlineDuration: &metav1.Duration{Duration: 35 * time.Hour},
									},
								},
							},
						},
					},
				},
			},
			wants: wants{
				patchedPods: []*corev1.Pod{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-be-pod",
							Namespace: "default",
							Labels: map[string]string{
								extension.LabelPodQoS:    string(extension.QoSBE),
								unified.LabelEvictionKey: unified.LabelEvictionValue,
							},
							Annotations: map[string]string{
								unified.AnnotationEvictionTypeKey:            `["involuntary"]`,
								unified.AnnotationSkipNotReadyFlowControlKey: unified.AnnotationSkipNotReadyFlowControlValue,
								unified.AnnotationEvictionMessageKey:         "pod is expired because best-effort instance can only run maximum 35h0m0s duration",
							},
						},
						Status: corev1.PodStatus{
							StartTime: &metav1.Time{Time: timeNow().Add(-36 * time.Hour)},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctl := gomock.NewController(t)
			defer ctl.Finish()

			mockStatesInformer := mock_statesinformer.NewMockStatesInformer(ctl)
			mockStatesInformer.EXPECT().GetAllPods().Return(tt.fields.pods).AnyTimes()
			mockStatesInformer.EXPECT().GetNodeSLO().Return(tt.fields.nodeSLO).AnyTimes()

			fakeClient := fakeclientset.NewSimpleClientset()
			for _, podMeta := range tt.fields.pods {
				_, err := fakeClient.CoreV1().Pods(podMeta.Pod.Namespace).Create(context.TODO(), podMeta.Pod, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}

			opt := &framework.Options{
				StatesInformer: mockStatesInformer,
				KubeClient:     fakeClient,
			}

			d := New(opt).(*deadlineEvict)
			d.Setup(&framework.Context{})
			d.deadlineEvict()

			for _, wantPod := range tt.wants.patchedPods {
				gotPod, err := fakeClient.CoreV1().Pods(wantPod.Namespace).Get(context.TODO(), wantPod.Name, metav1.GetOptions{})
				if err != nil {
					t.Fatal(err)
				}
				assert.Equal(t, wantPod.Labels, gotPod.Labels)
				assert.Equal(t, wantPod.Annotations, gotPod.Annotations)
			}
		})
	}
}

func Test_deadlineEvict_Enabled(t *testing.T) {
	d := &deadlineEvict{
		evictInterval: time.Minute,
	}
	assert.Equalf(t, false, d.Enabled(), "Enabled()")
	err := features.DefaultMutableKoordletFeatureGate.SetFromMap(map[string]bool{string(features.DeadlineEvict): true})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equalf(t, true, d.Enabled(), "Enabled()")
}

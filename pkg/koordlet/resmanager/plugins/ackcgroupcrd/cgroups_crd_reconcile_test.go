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

package ackcgroupcrd

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/executor"
	cgroupscrdutil "github.com/koordinator-sh/koordinator/pkg/koordlet/resmanager/plugins/ackcgroupcrd/util"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/statesinformer"
	"github.com/koordinator-sh/koordinator/pkg/util"
	sysutil "github.com/koordinator-sh/koordinator/pkg/util/system"

	uniapiext "gitlab.alibaba-inc.com/cos/unified-resource-api/apis/extension"
)

func createPod(kubeQosClass corev1.PodQOSClass, qosClass apiext.QoSClass) *statesinformer.PodMeta {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "test_pod",
			UID:  "test_pod",
			Labels: map[string]string{
				apiext.LabelPodQoS: string(qosClass),
			},
			Annotations: map[string]string{},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test",
				},
				{
					Name: "main",
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:        "test",
					ContainerID: fmt.Sprintf("docker://%s", "test"),
				},
				{
					Name:        "main",
					ContainerID: fmt.Sprintf("docker://%s", "main"),
				},
			},
			QOSClass: kubeQosClass,
			Phase:    corev1.PodRunning,
		},
	}

	if qosClass == apiext.QoSBE {
		pod.Spec.Containers[1].Resources = corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				apiext.BatchCPU:    resource.MustParse("1024"),
				apiext.BatchMemory: resource.MustParse("1Gi"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				apiext.BatchCPU:    resource.MustParse("1024"),
				apiext.BatchMemory: resource.MustParse("1Gi"),
			},
		}
	} else {
		pod.Spec.Containers[1].Resources = corev1.ResourceRequirements{
			Requests: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		}
	}

	return &statesinformer.PodMeta{
		CgroupDir: util.GetPodKubeRelativePath(pod),
		Pod:       pod,
	}
}

func TestCgroupsCrdReconcile_updateConfig(t *testing.T) {
	type args struct {
		newConfigMap *corev1.ConfigMap
	}
	tests := []struct {
		name string
		args args
		want *cgroupscrdutil.CgroupsControllerConfig
	}{
		{
			name: "update-config",
			args: args{
				newConfigMap: &corev1.ConfigMap{
					Data: map[string]string{
						"default.cpuacct": "{\"cpu.cfs_burst_us\": \"300000\"}",
						"default.cpuset":  "{\"cpuset.sched_load_balance\": \"0\"}",
						"default.cpushare.qos.high.force-reserved": "true",
						"default.cpushare.qos.low.cpu-watermark":   "30",
						"default.cpushare.qos.low.l3-percent":      "10",
						"default.cpushare.qos.low.mb-percent":      "20",
						"default.cpushare.qos.low.namespaces":      "besteffort-ns1, besteffort-ns2",
						"default.qosClass":                         "LS",
					},
				},
			},
			want: &cgroupscrdutil.CgroupsControllerConfig{
				CPUSet: map[string]string{
					"cpuset.sched_load_balance": "0",
				},
				CPUAcct: map[string]string{
					"cpu.cfs_burst_us": "300000",
				},
				QOSClass:          "LS",
				LowNamespaces:     "besteffort-ns1, besteffort-ns2",
				LowL3Percent:      "10",
				LowMbPercent:      "20",
				LowCPUWatermark:   "30",
				HighForceReserved: "true",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &plugin{}
			c.updateConfig(tt.args.newConfigMap)
			if !reflect.DeepEqual(tt.want, c.controllerConf) {
				t.Errorf("update config not equal, want:\n%#v\ngot:\n%#v", tt.want, c.controllerConf)
				return
			}
		})
	}
}

func Test_calculateCatMbSchemata(t *testing.T) {
	got := calculateCatMbSchemata(-1)
	assert.Equal(t, "1", got)
	got = calculateCatMbSchemata(0)
	assert.Equal(t, "1", got)
	got = calculateCatMbSchemata(2)
	assert.Equal(t, "2", got)
	got = calculateCatMbSchemata(100)
	assert.Equal(t, "100", got)
	got = calculateCatMbSchemata(200)
	assert.Equal(t, "100", got)
}

func Test_executePodPlan(t *testing.T) {
	t.Run("test not panic", func(t *testing.T) {
		helper := sysutil.NewFileTestUtil(t)
		defer helper.Cleanup()

		testPodMeta := createPod(corev1.PodQOSGuaranteed, apiext.QoSNone)
		testPodMeta.Pod.Annotations[uniapiext.AnnotationPodCPUSetScheduler] = string(uniapiext.CPUSetSchedulerTrue)
		testPodMeta.Pod.Annotations[uniapiext.AnnotationPodCPUSet] = `
{"test":{"0":{"elems":{"0":{}}}},"main":{"1":{"elems":{"26":{}}}}}
`
		assert.Equal(t, true, cgroupscrdutil.IsPodCfsQuotaNeedUnset(testPodMeta.Pod))

		testPodPlan := cgroupscrdutil.GeneratePlanByPod(testPodMeta, &cgroupscrdutil.CgroupsControllerConfig{}, 32)
		c := plugin{
			config:   NewDefaultConfig(),
			executor: executor.NewResourceUpdateExecutor("CgroupsCrdExecutorTest", 60),
		}
		c.executePodPlan(testPodPlan)
	})
}

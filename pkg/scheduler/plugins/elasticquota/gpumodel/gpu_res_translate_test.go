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

package gpumodel

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/extension/unified"
)

func Test_normalizeGpuResourcesToCardRatioForNode(t *testing.T) {
	{
		result := corev1.ResourceList{
			"test": resource.MustParse("200"),
		}
		resultExpect := corev1.ResourceList{
			"test": resource.MustParse("200"),
		}
		result = NormalizeGPUResourcesToCardRatioForNode(result, "v100")
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceGPUMemoryRatio: resource.MustParse("200"),
		}
		resultExpect := corev1.ResourceList{
			extension.ResourceGPUMemoryRatio: resource.MustParse("200"),
			unified.GPUCardRatio:             resource.MustParse("200"),
			unified.GPUCardRatio + "-v100":   resource.MustParse("200"),
		}
		result = NormalizeGPUResourcesToCardRatioForNode(result, "v100")
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceNvidiaGPU: resource.MustParse("2"),
		}
		resultExpect := corev1.ResourceList{
			extension.ResourceNvidiaGPU:    resource.MustParse("2"),
			unified.GPUCardRatio:           resource.MustParse("200"),
			unified.GPUCardRatio + "-v100": resource.MustParse("200"),
		}
		result = NormalizeGPUResourcesToCardRatioForNode(result, "v100")
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
}

func Test_parseGPUResourceByModel(t *testing.T) {
	testCases := []struct {
		name        string
		nodeLabel   map[string]string
		allocatable corev1.ResourceList
		output      corev1.ResourceList
	}{
		{
			name: "node without gpu resources",
			allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			output: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
		{
			name: "node with gpu allocatable but without gpu mode label",
			allocatable: corev1.ResourceList{
				corev1.ResourceCPU:          resource.MustParse("1"),
				corev1.ResourceMemory:       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU: resource.MustParse("1"),
			},
			output: corev1.ResourceList{
				corev1.ResourceCPU:          resource.MustParse("1"),
				corev1.ResourceMemory:       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU: resource.MustParse("1"),
			},
		},
		{
			name: "node with gpu allocatable and with gpu mode label detail Tesla-P100-16Gb",
			nodeLabel: map[string]string{
				extension.LabelGPUModel: "Tesla-P100-16Gb",
			},
			allocatable: corev1.ResourceList{
				corev1.ResourceCPU:          resource.MustParse("1"),
				corev1.ResourceMemory:       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU: resource.MustParse("1"),
			},
			output: corev1.ResourceList{
				corev1.ResourceCPU:                        resource.MustParse("1"),
				corev1.ResourceMemory:                     resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:               resource.MustParse("1"),
				unified.GPUCardRatio:                      resource.MustParse("100"),
				unified.GPUCardRatio + "-tesla-p100-16gb": resource.MustParse("100"),
			},
		},
		{
			name: "node with gpu&mem-ratio allocatable and with gpu mode label detail Tesla-P100-16Gb",
			nodeLabel: map[string]string{
				extension.LabelGPUModel: "Tesla-P100-16Gb",
			},
			allocatable: corev1.ResourceList{
				corev1.ResourceCPU:               resource.MustParse("1"),
				corev1.ResourceMemory:            resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:      resource.MustParse("1"),
				extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
			},
			output: corev1.ResourceList{
				corev1.ResourceCPU:                        resource.MustParse("1"),
				corev1.ResourceMemory:                     resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:               resource.MustParse("1"),
				extension.ResourceGPUMemoryRatio:          resource.MustParse("100"),
				unified.GPUCardRatio:                      resource.MustParse("100"),
				unified.GPUCardRatio + "-tesla-p100-16gb": resource.MustParse("100"),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			out := ParseGPUResourcesByModel("node", testCase.allocatable, testCase.nodeLabel)
			if !quotav1.Equals(out, testCase.output) {
				t.Errorf("unexpected output, expected: %+v, got: %+v", testCase.output, out)
			}
		})
	}
}

func TestNormalizeQuotaResourcesToCardRatio(t *testing.T) {
	testCases := []struct {
		name   string
		model  string
		input  corev1.ResourceList
		output corev1.ResourceList
		erase  bool
	}{
		{
			name: "no gpu resources",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
		},
		{
			name: "gpu resources with nvidia request and without model",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:          resource.MustParse("1"),
				corev1.ResourceMemory:       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU: resource.MustParse("1"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:          resource.MustParse("1"),
				corev1.ResourceMemory:       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU: resource.MustParse("1"),
				unified.GPUCardRatio:        resource.MustParse("100"),
			},
		},
		{
			name: "gpu resources with alibabacloud request and without model",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				extension.ResourceGPU: resource.MustParse("100"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				extension.ResourceGPU: resource.MustParse("100"),
				unified.GPUCardRatio:  resource.MustParse("100"),
			},
		},
		{
			name: "gpu resources with gpumem ratio request and without model",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:               resource.MustParse("1"),
				corev1.ResourceMemory:            resource.MustParse("1Gi"),
				extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:               resource.MustParse("1"),
				corev1.ResourceMemory:            resource.MustParse("1Gi"),
				extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
				unified.GPUCardRatio:             resource.MustParse("100"),
			},
		},
		{
			name: "gpu resources with both gpumem-ratio & core request and without model",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:               resource.MustParse("1"),
				corev1.ResourceMemory:            resource.MustParse("1Gi"),
				extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
				extension.ResourceGPUCore:        resource.MustParse("150"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:               resource.MustParse("1"),
				corev1.ResourceMemory:            resource.MustParse("1Gi"),
				extension.ResourceGPUMemoryRatio: resource.MustParse("100"),
				extension.ResourceGPUCore:        resource.MustParse("150"),
				unified.GPUCardRatio:             resource.MustParse("150"),
			},
		},
		{
			name:  "gpu resources with nvidia request and with model Tesla-V100",
			model: "Tesla-V100",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:          resource.MustParse("1"),
				corev1.ResourceMemory:       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU: resource.MustParse("1"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:                          resource.MustParse("1"),
				corev1.ResourceMemory:                       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:                 resource.MustParse("1"),
				extension.ResourceNvidiaGPU + "-tesla-v100": resource.MustParse("1"),
				unified.GPUCardRatio:                        resource.MustParse("100"),
				unified.GPUCardRatio + "-tesla-v100":        resource.MustParse("100"),
			},
		},
		{
			name:  "gpu resources with alibabacloud request and with model Tesla-V100",
			model: "Tesla-V100",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
				extension.ResourceGPU: resource.MustParse("100"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:                    resource.MustParse("1"),
				corev1.ResourceMemory:                 resource.MustParse("1Gi"),
				extension.ResourceGPU:                 resource.MustParse("100"),
				extension.ResourceGPU + "-tesla-v100": resource.MustParse("100"),
				unified.GPUCardRatio:                  resource.MustParse("100"),
				unified.GPUCardRatio + "-tesla-v100":  resource.MustParse("100"),
			},
		},
		{
			name:  "gpu resources with nvidia-card-model request and with model Tesla-V100",
			model: "Tesla-V100",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:                          resource.MustParse("1"),
				corev1.ResourceMemory:                       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:                 resource.MustParse("2"),
				extension.ResourceNvidiaGPU + "-tesla-v100": resource.MustParse("1"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:                          resource.MustParse("1"),
				corev1.ResourceMemory:                       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:                 resource.MustParse("2"),
				extension.ResourceNvidiaGPU + "-tesla-v100": resource.MustParse("2"),
				unified.GPUCardRatio:                        resource.MustParse("200"),
				unified.GPUCardRatio + "-tesla-v100":        resource.MustParse("200"),
			},
		},
		{
			name: "gpu resources with nvidia-card-model request and without model",
			input: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:                          resource.MustParse("1"),
				corev1.ResourceMemory:                       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:                 resource.MustParse("2"),
				extension.ResourceNvidiaGPU + "-tesla-v100": resource.MustParse("1"),
			},
			output: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:                          resource.MustParse("1"),
				corev1.ResourceMemory:                       resource.MustParse("1Gi"),
				extension.ResourceNvidiaGPU:                 resource.MustParse("2"),
				extension.ResourceNvidiaGPU + "-tesla-v100": resource.MustParse("1"),
				unified.GPUCardRatio:                        resource.MustParse("200"),
				//unified.GPUCardRatio + "-tesla-v100": resource.MustParse("100"),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := NormalizeGPUResourcesToCardRatioForPod(testCase.input, testCase.model)
			if !quotav1.Equals(actual, testCase.output) {
				t.Errorf("unexpected output, expected: %+v\n actual: %+v", testCase.output, actual)
			}
		})
	}
}

func TestNormalizeGpuResourcesToCardRatioForQuota2(t *testing.T) {
	{
		result := corev1.ResourceList{
			"test": resource.MustParse("200"),
		}
		resultExpect := corev1.ResourceList{
			"test": resource.MustParse("200"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceNvidiaGPU:           resource.MustParse("5"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("4"),
		}
		resultExpect := corev1.ResourceList{
			extension.ResourceNvidiaGPU:           resource.MustParse("5"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("4"),
			unified.GPUCardRatio:                  resource.MustParse("500"),
			unified.GPUCardRatio + "-v100":        resource.MustParse("400"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceGPU:           resource.MustParse("500"),
			extension.ResourceGPU + "-v100": resource.MustParse("400"),
		}
		resultExpect := corev1.ResourceList{
			extension.ResourceGPU:           resource.MustParse("500"),
			extension.ResourceGPU + "-v100": resource.MustParse("400"),
			unified.GPUCardRatio:            resource.MustParse("500"),
			unified.GPUCardRatio + "-v100":  resource.MustParse("400"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			unified.GPUCardRatio:           resource.MustParse("500"),
			unified.GPUCardRatio + "-v100": resource.MustParse("400"),
		}
		resultExpect := corev1.ResourceList{
			unified.GPUCardRatio:           resource.MustParse("500"),
			unified.GPUCardRatio + "-v100": resource.MustParse("400"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
}

func TestNormalizeGpuResourcesToCardRatioForQuota(t *testing.T) {
	{
		result := corev1.ResourceList{
			"test": resource.MustParse("200"),
		}
		resultExpect := corev1.ResourceList{
			"test": resource.MustParse("200"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceNvidiaGPU:           resource.MustParse("4"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("4"),
		}
		resultExpect := corev1.ResourceList{
			extension.ResourceNvidiaGPU:           resource.MustParse("4"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("4"),
			unified.GPUCardRatio:                  resource.MustParse("400"),
			unified.GPUCardRatio + "-v100":        resource.MustParse("400"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceGPU:           resource.MustParse("400"),
			extension.ResourceGPU + "-v100": resource.MustParse("400"),
		}
		resultExpect := corev1.ResourceList{
			extension.ResourceGPU:           resource.MustParse("400"),
			extension.ResourceGPU + "-v100": resource.MustParse("400"),
			unified.GPUCardRatio:            resource.MustParse("400"),
			unified.GPUCardRatio + "-v100":  resource.MustParse("400"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			unified.GPUCardRatio:           resource.MustParse("600"),
			unified.GPUCardRatio + "-v100": resource.MustParse("400"),
		}
		resultExpect := corev1.ResourceList{
			unified.GPUCardRatio:           resource.MustParse("600"),
			unified.GPUCardRatio + "-v100": resource.MustParse("400"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("6"),
			extension.ResourceNvidiaGPU + "-p100": resource.MustParse("4"),
		}
		resultExpect := corev1.ResourceList{
			unified.GPUCardRatio:                  resource.MustParse("1000"),
			unified.GPUCardRatio + "-v100":        resource.MustParse("600"),
			unified.GPUCardRatio + "-p100":        resource.MustParse("400"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("6"),
			extension.ResourceNvidiaGPU + "-p100": resource.MustParse("4"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
	{
		result := corev1.ResourceList{
			extension.ResourceNvidiaGPU:           resource.MustParse("12"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("6"),
			extension.ResourceNvidiaGPU + "-p100": resource.MustParse("4"),
		}
		resultExpect := corev1.ResourceList{
			unified.GPUCardRatio:                  resource.MustParse("1200"),
			unified.GPUCardRatio + "-v100":        resource.MustParse("600"),
			unified.GPUCardRatio + "-p100":        resource.MustParse("400"),
			extension.ResourceNvidiaGPU:           resource.MustParse("12"),
			extension.ResourceNvidiaGPU + "-v100": resource.MustParse("6"),
			extension.ResourceNvidiaGPU + "-p100": resource.MustParse("4"),
		}
		result = NormalizeGpuResourcesToCardRatioForQuota(result)
		if !quotav1.Equals(result, resultExpect) {
			t.Errorf("expected: %v, actual: %v", resultExpect, result)
		}
	}
}

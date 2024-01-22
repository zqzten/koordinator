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

package system

import (
	"testing"
)

func TestParseCPUStatRawV2Anolis(t *testing.T) {
	tests := []struct {
		input   string
		want    *CPUStatAnolisRaw
		wantErr bool
	}{
		{
			input: "sibidle_usec 10\nthrottled_usec 5",
			want: &CPUStatAnolisRaw{
				SibidleUSec:   10,
				ThrottledUSec: 5,
			},
			wantErr: false,
		},
		{
			input:   "sibidle_usec not_a_number\nthrottled_usec 5",
			want:    nil,
			wantErr: true,
		},
		{
			input:   "invalid_field 10\nthrottled_usec 5",
			want:    nil,
			wantErr: true,
		},
	}

	for _, test := range tests {
		got, err := ParseCPUStatRawV2Anolis(test.input)

		if test.wantErr {
			if err == nil {
				t.Errorf("Expected an error for input: %s", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input: %s, err: %v", test.input, err)
			}
			if !compareCPUStatV2AnolisRaw(got, test.want) {
				t.Errorf("For input: %s, got: %v, want: %v", test.input, got, test.want)
			}
		}
	}
}

func compareCPUStatV2AnolisRaw(a, b *CPUStatAnolisRaw) bool {
	return a.SibidleUSec == b.SibidleUSec &&
		a.ThrottledUSec == b.ThrottledUSec
}

func TestParseCPUSchedCfsStatisticsV2AnolisRaw(t *testing.T) {
	tests := []struct {
		input   string
		want    *CPUSchedCfsStatisticsAnolisRaw
		wantErr bool
	}{
		{
			input: "100 70 10 10 10 10\n",
			want: &CPUSchedCfsStatisticsAnolisRaw{
				Serve:        100,
				OnCpu:        70,
				QueueOther:   10,
				QueueSibling: 10,
				QueueMax:     10,
				ForceIdle:    10,
			},
			wantErr: false,
		},
		{
			input:   "100\n", // len(fileds) < 6
			want:    nil,
			wantErr: true,
		},
		{
			input:   "100 not_a_number 10 10 10 10\n",
			want:    nil,
			wantErr: true,
		},
	}

	for _, test := range tests {
		got, err := ParseCPUSchedCfsStatisticsV2AnolisRaw(test.input)

		if test.wantErr {
			if err == nil {
				t.Errorf("Expected an error for input: %s", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input: %s, err: %v", test.input, err)
			}
			if !compareParseCPUSchedCfsStatisticsV2AnolisRaw(got, test.want) {
				t.Errorf("For input: %s, got: %v, want: %v", test.input, got, test.want)
			}
		}
	}
}

func compareParseCPUSchedCfsStatisticsV2AnolisRaw(a, b *CPUSchedCfsStatisticsAnolisRaw) bool {
	return a.Serve == b.Serve &&
		a.OnCpu == b.OnCpu &&
		a.QueueOther == b.QueueOther &&
		a.QueueSibling == b.QueueSibling &&
		a.QueueMax == b.QueueMax &&
		a.ForceIdle == b.ForceIdle
}

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

package utils

import (
	"testing"
)

func TestIsAbort(t *testing.T) {
	type args struct {
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil label",
			args: args{
				labels: nil,
			},
			want: false,
		}, {
			name: "no key",
			args: args{
				labels: map[string]string{
					"a": "b",
				},
			},
			want: false,
		}, {
			name: "has abort key, value is not true",
			args: args{
				labels: map[string]string{
					AbortKey: "b",
				},
			},
			want: false,
		}, {
			name: "has abort key",
			args: args{
				labels: map[string]string{
					AbortKey: "true",
				},
			},
			want: true,
		}, {
			name: "has clean key",
			args: args{
				labels: map[string]string{
					CleanKey: "true",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAbort(tt.args.labels); got != tt.want {
				t.Errorf("IsAbort() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedCleanTaint(t *testing.T) {
	type args struct {
		labels map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "nil label",
			args: args{
				labels: nil,
			},
			want: false,
		}, {
			name: "no key",
			args: args{
				labels: map[string]string{
					"a": "b",
				},
			},
			want: false,
		}, {
			name: "has clean key",
			args: args{
				labels: map[string]string{
					CleanKey: "true",
				},
			},
			want: true,
		}, {
			name: "has clean key, value is not true",
			args: args{
				labels: map[string]string{
					CleanKey: "b",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedCleanTaint(tt.args.labels); got != tt.want {
				t.Errorf("NeedCleanTaint() = %v, want %v", got, tt.want)
			}
		})
	}
}

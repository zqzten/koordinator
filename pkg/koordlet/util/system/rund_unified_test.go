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
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/ttrpc"
	"github.com/stretchr/testify/assert"

	"gitlab.alibaba-inc.com/virtcontainers/agent-protocols/protos/extends"
	"gitlab.alibaba-inc.com/virtcontainers/agent-protocols/protos/grpc"
)

func TestGetRundStats(t *testing.T) {
	helper := NewFileTestUtil(t)
	defer helper.Cleanup()
	oldHostRunRootDir := Conf.RunRootDir
	Conf.RunRootDir = helper.TempDir
	defer func() {
		Conf.RunRootDir = oldHostRunRootDir
	}()

	sandboxID := "8c000081"
	socketPath := GenRundShimSocketPath(sandboxID)
	err := os.MkdirAll(filepath.Dir(socketPath), 0766)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	defer cancel()
	expected := &grpc.ExtendedStatsResponse{
		PodStats: &grpc.PodStats{
			CgroupMemStats: &grpc.ContainerCgroupMem{
				Total: 2097152000,
			},
		},
	}

	s, err := ttrpc.NewServer()
	assert.NoError(t, err)
	svc := &FakeExtendedStatusService{}
	svc.FakeExtendedStats = func(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error) {
		return expected, nil
	}
	extends.RegisterExtendedStatusService(s, svc)
	l, err := net.Listen("unix", socketPath)
	assert.NoError(t, err, "listen error")

	go func() {
		err = s.Serve(ctx, l)
		assert.NoError(t, err, "serve error")
	}()

	got, err := GetRundStats(sandboxID)
	assert.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestGetRundPodStats(t *testing.T) {
	tests := []struct {
		name    string
		field   extends.ExtendedStatusService
		arg     string
		want    *grpc.PodStats
		wantErr bool
	}{
		{
			name: "success",
			field: &FakeExtendedStatusService{
				FakeExtendedStats: func(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error) {
					return &grpc.ExtendedStatsResponse{
						PodStats: &grpc.PodStats{
							CgroupMemStats: &grpc.ContainerCgroupMem{
								Total: 2097152000,
							},
							MemxStats: &grpc.PodMemxStats{
								Ifile: 1,
							},
							MemReclaimStats: &grpc.PodMemReclaimStats{
								Reported:      0,
								Reporting:     1,
								TotalReported: 2,
								TotalRefault:  3,
							},
						},
					}, nil
				},
			},
			arg: "8c000081",
			want: &grpc.PodStats{
				CgroupMemStats: &grpc.ContainerCgroupMem{
					Total: 2097152000,
				},
				MemxStats: &grpc.PodMemxStats{
					Ifile: 1,
				},
				MemReclaimStats: &grpc.PodMemReclaimStats{
					Reported:      0,
					Reporting:     1,
					TotalReported: 2,
					TotalRefault:  3,
				},
			},
			wantErr: false,
		},
		{
			name: "failed to get stats",
			field: &FakeExtendedStatusService{
				FakeExtendedStats: func(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error) {
					return nil, fmt.Errorf("get rund stats failed")
				},
			},
			arg:     "8c000082",
			want:    nil,
			wantErr: true,
		},
		{
			name: "get nil stat",
			field: &FakeExtendedStatusService{
				FakeExtendedStats: func(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error) {
					return &grpc.ExtendedStatsResponse{}, nil
				},
			},
			arg:     "8c000083",
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helper := NewFileTestUtil(t)
			defer helper.Cleanup()
			oldHostRunRootDir := Conf.RunRootDir
			Conf.RunRootDir = helper.TempDir
			defer func() {
				Conf.RunRootDir = oldHostRunRootDir
			}()

			socketPath := GenRundShimSocketPath(tt.arg)
			err := os.MkdirAll(filepath.Dir(socketPath), 0766)
			assert.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
			defer cancel()
			s, err := ttrpc.NewServer()
			defer func() {
				_ = s.Close()
			}()
			assert.NoError(t, err)
			svc := tt.field
			extends.RegisterExtendedStatusService(s, svc)
			l, err := net.Listen("unix", socketPath)
			assert.NoError(t, err, "listen error")
			go s.Serve(ctx, l)

			got, err := GetRundPodStats(tt.arg)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestGetRundContainerCPUUsageTick(t *testing.T) {
	tests := []struct {
		name    string
		args    *grpc.ContainerBaseStats
		want    uint64
		wantErr bool
	}{
		{
			name:    "got invalid cpu stats",
			args:    &grpc.ContainerBaseStats{},
			want:    0,
			wantErr: true,
		},
		{
			name: "got valid cpu stats",
			args: &grpc.ContainerBaseStats{
				CgroupCpu: &grpc.ContainerCgroupCpu{
					User: 9000,
					Sys:  1000,
					Nice: 0,
					Sirq: 0,
					Hirq: 0,
				},
			},
			want:    10000,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := GetRundContainerCPUUsageTick(tt.args)
			assert.Equal(t, tt.wantErr, gotErr != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetRundContainerMemoryStat(t *testing.T) {
	tests := []struct {
		name    string
		args    *grpc.ContainerBaseStats
		want    *MemoryStatRaw
		wantErr bool
	}{
		{
			name:    "got invalid memory stats",
			args:    &grpc.ContainerBaseStats{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "got invalid memory stats 1",
			args: &grpc.ContainerBaseStats{
				CgroupMem:  &grpc.ContainerCgroupMem{},
				CgroupMemx: nil,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "got valid memory stats",
			args: &grpc.ContainerBaseStats{
				CgroupMem: &grpc.ContainerCgroupMem{
					Rss:   1048576,
					Cache: 524288,
					Total: 2097152,
				},
				CgroupMemx: &grpc.ContainerCgroupMemx{
					Aanon: 524288,
					Ianon: 524288,
					Afile: 262144,
					Ifile: 262144,
				},
			},
			want: &MemoryStatRaw{
				Cache:        524288,
				RSS:          1048576,
				InactiveFile: 262144,
				ActiveFile:   262144,
				InactiveAnon: 524288,
				ActiveAnon:   524288,
				Unevictable:  0,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := GetRundContainerMemoryStat(tt.args)
			assert.Equal(t, tt.wantErr, gotErr != nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

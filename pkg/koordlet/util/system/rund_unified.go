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
	"path/filepath"
	"time"

	"github.com/containerd/ttrpc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"k8s.io/klog/v2"

	"gitlab.alibaba-inc.com/virtcontainers/agent-protocols/protos/extends"
	"gitlab.alibaba-inc.com/virtcontainers/agent-protocols/protos/grpc"
)

const (
	defaultConnectTimeout = 1 * time.Second
	defaultRequestTimeout = 2 * time.Second

	RundExtendedStatsSocketSubDir   = "/vc/sbs/"
	RundExtendedStatsSocketFilename = "extendedstats.sock"

	RundMemoryCgroupSubdir = "/kata"
)

func GetRundMemoryCgroupParentDir(sandboxID string) string {
	// https://aliyuque.antfin.com/sigmahost/bdrtdk/mq5buk#baiVb
	// Instead, rund pod on cgroups-v2 should use the standard cgroup path.
	return filepath.Join(RundMemoryCgroupSubdir, sandboxID)
}

func GetRundPodStats(sandboxID string) (*grpc.PodStats, error) {
	extendedStats, err := GetRundStats(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("cannot get rund extendedStats, sandbox id %s, err: %s", sandboxID, err)
	}

	if extendedStats == nil || extendedStats.PodStats == nil {
		return nil, fmt.Errorf("invalid rund extendedStats, extendedStats %v, sandbox id %s",
			extendedStats, sandboxID)
	}

	return extendedStats.PodStats, nil
}

func GetRundContainerStats(sandboxID string) (map[string]*grpc.ContainerStats, error) {
	extendedStats, err := GetRundStats(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("cannot get rund extendedStats, sandbox id %s, err: %s", sandboxID, err)
	}

	if extendedStats == nil {
		return nil, fmt.Errorf("invalid rund extendedStats for container, extendedStats %v, sandbox id %s",
			extendedStats, sandboxID)
	}

	containerStatsMap := map[string]*grpc.ContainerStats{}
	for i := range extendedStats.ConStats {
		c := extendedStats.ConStats[i]
		if c.BaseStats == nil || len(c.BaseStats.ContainerId) <= 0 {
			klog.V(6).Infof("failed to parse rund extendedStats for container, invalid BaseStats %v, sandbox id %s",
				c.BaseStats, sandboxID)
			continue
		}
		containerStatsMap[c.BaseStats.ContainerId] = c
	}

	return containerStatsMap, nil
}

// GetRundStats gets extended stats for a rund sandbox.
// extended stats: http://gitlab.alibaba-inc.com/virtcontainers/agent-protocols/blob/master/protos/extendedstats.proto
func GetRundStats(sandboxID string) (*grpc.ExtendedStatsResponse, error) {
	conn, err := connectExtendedStats(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to rund ExtendedStats socket, err: %s", err)
	}
	defer func() {
		err = conn.Close()
		if err != nil {
			klog.Warningf("failed to close the connection to rund ExtendedStats socket, sandbox id %s, err: %s", sandboxID, err)
		}
	}()

	client := ttrpc.NewClient(conn)
	defer func() {
		err = client.Close()
		if err != nil {
			klog.Warningf("failed to close ttrpc client to rund ExtendedStats, sandbox id %s, err: %s", sandboxID, err)
		}
	}()

	// TBD: add containerd namespace for context
	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout*100)
	defer cancel()

	extendsClient := extends.NewExtendedStatusClient(client)
	resp, err := extendsClient.ExtendedStats(ctx, &grpc.ExtendedStatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("cannot get rund ExtendedStats, err: %s", err)
	}

	return resp, nil
}

// GenRundShimSocketPath generates the path of rund shim socket for given sandbox
// e.g. /run/vc/sbs/d57d43ebccef237b31c64ba851c7e3569afe56d6b7364b3385aad605559ae964/extendedstats.sock
func GenRundShimSocketPath(sandboxID string) string {
	return filepath.Join(Conf.RunRootDir, RundExtendedStatsSocketSubDir, sandboxID, RundExtendedStatsSocketFilename)
}

func connectExtendedStats(sandboxID string) (net.Conn, error) {
	addr := GenRundShimSocketPath(sandboxID)
	conn, err := net.DialTimeout("unix", addr, defaultConnectTimeout)
	if err == nil {
		return conn, nil
	}

	return nil, fmt.Errorf("cannot connect to ttrpc server, err: %s", err)
}

var _ extends.ExtendedStatusService = &FakeExtendedStatusService{}

type FakeExtendedStatusService struct {
	extends.ExtendedStatusService
	FakeExtendedStats             func(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error)
	FakeReclaimMemory             func(ctx context.Context, req *grpc.ReclaimMemoryRequest) (*emptypb.Empty, error)
	FakeConfigureMemoryCompaction func(ctx context.Context, req *grpc.ConfigureMemoryCompactionRequest) (*emptypb.Empty, error)
}

func (c *FakeExtendedStatusService) ExtendedStats(ctx context.Context, req *grpc.ExtendedStatsRequest) (*grpc.ExtendedStatsResponse, error) {
	return c.FakeExtendedStats(ctx, req)
}

func (c *FakeExtendedStatusService) ReclaimMemory(ctx context.Context, req *grpc.ReclaimMemoryRequest) (*emptypb.Empty, error) {
	return c.FakeReclaimMemory(ctx, req)
}

func (c *FakeExtendedStatusService) ConfigureMemoryCompaction(ctx context.Context, req *grpc.ConfigureMemoryCompactionRequest) (*emptypb.Empty, error) {
	return c.FakeConfigureMemoryCompaction(ctx, req)
}

func GetRundContainerCPUUsageTick(containerBaseStats *grpc.ContainerBaseStats) (uint64, error) {
	cgroupCPU := containerBaseStats.CgroupCpu
	if cgroupCPU == nil {
		return 0, fmt.Errorf("invalid CgroupCpu %v", containerBaseStats.CgroupCpu)
	}
	return cgroupCPU.User + cgroupCPU.Sys + cgroupCPU.Nice + cgroupCPU.Hirq + cgroupCPU.Sirq, nil
}

func GetRundContainerMemoryStat(containerBaseStats *grpc.ContainerBaseStats) (*MemoryStatRaw, error) {
	cgroupMem, cgroupMemx := containerBaseStats.CgroupMem, containerBaseStats.CgroupMemx
	if cgroupMem == nil {
		return nil, fmt.Errorf("invalid CgroupMem %v", containerBaseStats.CgroupMem)
	}
	if cgroupMemx == nil {
		return nil, fmt.Errorf("invalid CgroupMemx %v", containerBaseStats.CgroupMemx)
	}

	return &MemoryStatRaw{
		RSS:          int64(cgroupMem.Rss),
		Cache:        int64(cgroupMem.Cache),
		InactiveFile: int64(cgroupMemx.Ifile),
		ActiveFile:   int64(cgroupMemx.Afile),
		InactiveAnon: int64(cgroupMemx.Ianon),
		ActiveAnon:   int64(cgroupMemx.Aanon),
		// FIXME: currently rund extendedstats does not export Unevictable. So we have to calculate the memory usage
		//  according to the rss and cache.
		Unevictable: 0,
	}, nil
}

func (m *MemoryStatRaw) UsageForRund() int64 {
	// memory.stat usage for rund: RSS
	return m.RSS
}

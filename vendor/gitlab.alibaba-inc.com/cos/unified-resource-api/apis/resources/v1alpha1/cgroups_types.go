/*

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

package v1alpha1

/*

Compatible purpose for old Cgroups CRD(cgroups.resources.alibabacloud.com). slo-controller watch Cgroups CRD,
and update corresponding pod cgroup info on NodeSLO.Spec.Cgroups.
See https://yuque.antfin-inc.com/docs/share/55fd2f61-36ce-4bba-85f9-023e32388ca3 for more design details.

Origin Cgroup CRD repo: https://code.aone.alibaba-inc.com/cos/cgroups-controller/blob/master/api/v1alpha1/cgroups_types.go

*/

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CgroupsSpec defines the desired state of Cgroups
type CgroupsSpec struct {
	PodInfo        PodInfoSpec        `json:"pod,omitempty"`
	DeploymentInfo DeploymentInfoSpec `json:"deployment,omitempty"`
	JobInfo        JobInfoSpec        `json:"job,omitempty"`
}

// CgroupsStatus defines the observed state of Cgroups
type CgroupsStatus struct {
	Generation int64       `json:"generation,omitempty"`
	UpdateTime metav1.Time `json:"update-time,omitempty"`
	Status     string      `json:"status,omitempty"`
	Message    string      `json:"message,omitempty"`
}

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=cgroups
// +kubebuilder:subresource:status

// Cgroups is the Schema for the cgroups API
type Cgroups struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CgroupsSpec   `json:"spec,omitempty"`
	Status CgroupsStatus `json:"status,omitempty"`
}

type ContainerInfoSpec struct {
	Name        string            `json:"name"`
	NodeName    string            `json:"nodename,omitempty"`
	Cpu         resource.Quantity `json:"cpu,omitempty"`
	Memory      resource.Quantity `json:"memory,omitempty"`
	BlkioSpec   Blkio             `json:"blkio,omitempty"`
	CPUSet      string            `json:"cpuset-cpus,omitempty"`
	LLCSpec     LLCinfo           `json:"llc-spec,omitempty"`
	CpuSetSpec  string            `json:"cpu-set,omitempty"`
	CpuAcctSpec string            `json:"cpuacct,omitempty"`
}

// Value resource.Quantity  `json:"value"`
type DeviceValue struct {
	Device string `json:"device"`
	Value  string `json:"value"`
}

type Blkio struct {
	Weight          uint16        `json:"weight,omitempty"`
	WeightDevice    []DeviceValue `json:"weight_device,omitempty"`
	DeviceReadBps   []DeviceValue `json:"device_read_bps,omitempty"`
	DeviceWriteBps  []DeviceValue `json:"device_write_bps,omitempty"`
	DeviceReadIOps  []DeviceValue `json:"device_read_iops,omitempty"`
	DeviceWriteIOps []DeviceValue `json:"device_write_iops,omitempty"`
}

func (b *Blkio) IsEmpty() bool {
	if b.Weight == 0 && len(b.WeightDevice) == 0 && len(b.DeviceReadBps) == 0 &&
		len(b.DeviceWriteBps) == 0 && len(b.DeviceReadIOps) == 0 && len(b.DeviceWriteIOps) == 0 {
		return true
	}
	return false
}

type PodInfoSpec struct {
	Name       string              `json:"name"`
	Namespace  string              `json:"namespace"`
	Containers []ContainerInfoSpec `json:"containers"`
}

type CgroupsCondition struct {
	//Type means Progressing or Available
	Type CgroupsConditionType `json:"type,omitempty"`
	// The last time this condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

type CgroupsConditionType string

const (
	CgroupsAvailable   CgroupsConditionType = "Available"
	CgroupsProgressing CgroupsConditionType = "Progressing"
)

// +kubebuilder:object:root=true

// CgroupsList contains a list of Cgroups
type CgroupsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cgroups `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Cgroups{}, &CgroupsList{})
}

type DeploymentInfoSpec struct {
	Name       string              `json:"name"`
	Namespace  string              `json:"namespace"`
	Containers []ContainerInfoSpec `json:"containers"`
}

type JobInfoSpec struct {
	Name       string              `json:"name"`
	Namespace  string              `json:"namespace"`
	Containers []ContainerInfoSpec `json:"containers"`
}

type LLCinfo struct {
	Socket      string `json:"socket,omitempty"`
	LLCPriority string `json:"llcpriority,omitempty"`
	MBPercent   string `json:"mbpercent,omitempty"`
	L3Percent   string `json:"l3percent,omitempty"`
}

/*
/sys/fs/cgroup/cpuset/cpuset.cpu_exclusive:0
/sys/fs/cgroup/cpuset/cpuset.cpus:0-1
/sys/fs/cgroup/cpuset/cpuset.effective_cpus:0-1
/sys/fs/cgroup/cpuset/cpuset.effective_mems:0-1
/sys/fs/cgroup/cpuset/cpuset.mem_exclusive:0
/sys/fs/cgroup/cpuset/cpuset.mem_hardwall:0
/sys/fs/cgroup/cpuset/cpuset.memory_migrate:0
/sys/fs/cgroup/cpuset/cpuset.memory_pressure:0
/sys/fs/cgroup/cpuset/cpuset.memory_spread_page:0
/sys/fs/cgroup/cpuset/cpuset.memory_spread_slab:0
/sys/fs/cgroup/cpuset/cpuset.mems:0-1
/sys/fs/cgroup/cpuset/cpuset.sched_load_balance:1
/sys/fs/cgroup/cpuset/cpuset.sched_relax_domain_level:-1
*/
type Cpuset struct {
	CpuExclusive          string `json:"cpu_exclusive,omitempty"`
	Cpus                  string `json:"cpus,omitempty"`
	EffectiveCpus         string `json:"effective_cpus,omitempty"`
	EffectiveMems         string `json:"effective_mems,omitempty"`
	MemExclusive          string `json:"mem_exclusive,omitempty"`
	MemHardwall           string `json:"mem_hardwall,omitempty"`
	MemoryMigrate         string `json:"memory_migrate,omitempty"`
	MemoryPressure        string `json:"memory_pressure,omitempty"`
	MemorySpreadPage      string `json:"memory_spread_page,omitempty"`
	MemorySpreadSlab      string `json:"memory_spread_slab,omitempty"`
	Mems                  string `json:"mems,omitempty"`
	SchedLoadBalance      string `json:"sched_load_balance,omitempty"`
	SchedRelaxDomainLevel string `json:"sched_relax_domain_level,omitempty"`
}

type Cpuacct struct {
	CfsQuotaUs string `json:"cfs_quota_us,omitempty"`
	Identity   string `json:"identity,omitempty"`
}

func (c *Cpuset) IsEmpty() bool {
	if c.CpuExclusive == "" && c.Cpus == "" && c.EffectiveCpus == "" && c.EffectiveMems == "" &&
		c.MemExclusive == "" && c.MemHardwall == "" && c.MemoryMigrate == "" && c.MemoryPressure == "" &&
		c.MemorySpreadPage == "" && c.MemorySpreadSlab == "" {
		return true
	}
	return false
}

func (l *LLCinfo) IsEmpty() bool {
	if l.LLCPriority == "" && l.Socket == "" {
		return true
	}
	return false
}

func (c *Cpuacct) IsEmpty() bool {
	if c.CfsQuotaUs == "" && c.Identity == "" {
		return true
	}
	return false
}

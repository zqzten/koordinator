package memorylocality

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/koordlet/util/system"
)

const (
	// AnnotationMemoryMigrateResult represents memory locality result.
	// The annotation updates after every migration.
	AnnotationMemoryLocalityStatus = extension.DomainPrefix + "memory-locality-status"
)

// MemoryMigrateResult describes memory locality result.
type MemoryMigrateResult struct {
	// CompletedTime represents the completed time of memory locality
	CompletedTime metav1.Time `json:"completedTime,omitempty"`
	// Result represents the result of last memory locality
	Result MigratePagesStatus `json:"result,omitempty"`
	// CompletedTimeLocalityRatio represents the local memory perecent of pod at last completed time
	CompletedTimeLocalityRatio int64 `json:"completedTimeLocalityRatio,omitempty"`
	// CompletedTimeRemotePages represents the rest remote memory pages of pod at last completed time
	CompletedTimeRemotePages int64 `json:"completedTimeRemotePages,omitempty"`
}

type MigratePagesResult struct {
	Status  MigratePagesStatus
	Message string
}

type MigratePagesStatus string

const (
	MigratedCompleted MigratePagesStatus = "MemoryLocalityCompleted"
	MigratedSkipped   MigratePagesStatus = "MemoryLocalitySkipped"
	MigratedFailed    MigratePagesStatus = "MemoryLocalityFailed"
)

type MemoryLocalityPhase string

const (
	MemoryLocalityStatusCompleted  MemoryLocalityPhase = "Completed"
	MemoryLocalityStatusInProgress MemoryLocalityPhase = "InProgress"
	MemoryLocalityStatusClosed     MemoryLocalityPhase = "Closed"
)

func (phase MemoryLocalityPhase) Pointer() *MemoryLocalityPhase {
	new := phase
	return &new
}

// MemoryLocalityStatus describe memory locality status of the pod
type MemoryLocalityStatus struct {
	Phase      *MemoryLocalityPhase `json:"phase,omitempty"`
	LastResult *MemoryMigrateResult `json:"lastResult,omitempty"`
}

func (in *MemoryLocalityStatus) DeepCopy() *MemoryLocalityStatus {
	if in == nil {
		return nil
	}
	out := new(MemoryLocalityStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *MemoryLocalityStatus) DeepCopyInto(out *MemoryLocalityStatus) {
	*out = *in
	if in.Phase != nil {
		in, out := &in.Phase, &out.Phase
		*out = new(MemoryLocalityPhase)
		**out = **in
	}
	if in.LastResult != nil {
		in, out := &in.LastResult, &out.LastResult
		*out = new(MemoryMigrateResult)
		(*in).DeepCopyInto(*out)
	}
}

func (in *MemoryMigrateResult) DeepCopy() *MemoryMigrateResult {
	if in == nil {
		return nil
	}
	out := new(MemoryMigrateResult)
	in.DeepCopyInto(out)
	return out
}

func (in *MemoryMigrateResult) DeepCopyInto(out *MemoryMigrateResult) {
	*out = *in
}

type NumaInfo struct {
	totalNumaInfo     []string
	nonAepNumaInfo    []string
	aepNumaInfo       []string
	podLocalNumaInfo  []string
	podRemoteNumaInfo []string
}

func (in *NumaInfo) DeepCopyInto(out *NumaInfo) {
	*out = *in
	if in.totalNumaInfo != nil {
		in, out := &in.totalNumaInfo, &out.totalNumaInfo
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

func (in *NumaInfo) DeepCopy() *NumaInfo {
	if in == nil {
		return nil
	}
	out := new(NumaInfo)
	in.DeepCopyInto(out)
	return out
}

type ContainerInfo struct {
	TaskIds  []int32
	Cpulist  []int
	Numalist map[int]struct{}
	NumaStat []system.NumaMemoryPages
	Result   MigratePagesResult
	Invalid  bool
}

type ContainerInfoMap map[string]*ContainerInfo

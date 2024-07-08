package cpuset

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	log "k8s.io/klog"
)

// Builder is a mutable builder for CPUSet. Functions that mutate instances
// of this type are not thread-safe.
type Builder struct {
	result CPUSet
	done   bool
}

// NewBuilder returns a mutable CPUSet builder.
func NewBuilder() Builder {
	return Builder{
		result: CPUSet{
			Elems: map[int]CPUInfo{},
		},
	}
}

// Add adds the supplied elements to the result. Calling Add after calling
// Result has no effect.
func (b Builder) Add(elems ...int) {
	if b.done {
		return
	}
	for _, elem := range elems {
		b.result.Elems[elem] = CPUInfo{}
	}
}

// Result returns the result CPUSet containing all elements that were
// previously added to this builder. Subsequent calls to Add have no effect.
func (b Builder) Result() CPUSet {
	b.done = true
	return b.result
}

type NodeCPUTopology map[int]SocketCPUTopology     //socketId
type SocketCPUTopology map[int]NUMANodeCPUTopology //numaId
type NUMANodeCPUTopology map[int]L3CPUTopology     //L3Id
type L3CPUTopology struct {
	Topology  CPUSet //cpuId
	Allocated map[int]int
}

// CPUSet is a thread-safe, immutable set-like data structure for CPU IDs.
type CPUSet struct {
	Elems map[int]CPUInfo `json:"elems,omitempty"`
	LLC   string          `json:"llc,omitempty"`
}

// cpu detil info
type CPUInfo struct {
	Id       int    `json:"id"`
	Node     int    `json:"node"`
	Socket   int    `json:"socket"`
	Core     int    `json:"core"`
	L1dl1il2 string `json:"l1dl1il2"`
	L3       int    `json:"l3"`
	Online   string `json:"online"`
	Size     int    `json:"size"`
}

type CPUAllocateResult map[string]map[int]SimpleCPUSet //containername -> numaId

func (ca CPUAllocateResult) ToCPUSet() CPUSet {
	cpusets := NewCPUSet()
	for _, containerCPUs := range ca {
		for _, simpleCPUSet := range containerCPUs {
			cpusets = cpusets.Union(simpleCPUSet.ToCPUSet())
		}
	}
	return cpusets
}

func (ca CPUAllocateResult) ToContainerCPUSets() map[string]CPUSet {
	result := make(map[string]CPUSet)
	for containerName, containerCPUs := range ca {
		containerCPUSets := NewCPUSet()
		for _, simpleCPUSet := range containerCPUs {
			containerCPUSets = containerCPUSets.Union(simpleCPUSet.ToCPUSet())
		}
		result[containerName] = containerCPUSets
	}
	return result
}

type SimpleCPUSet struct {
	Elems map[int]struct{} `json:"elems,omitempty"` // cpuId
	LLC   string           `json:"llc,omitempty"`
}

func (sc SimpleCPUSet) ToCPUSet() CPUSet {
	builder := NewBuilder()
	for cpuId := range sc.Elems {
		builder.Add(cpuId)
	}
	return builder.result
}

// NewCPUSet returns a new CPUSet containing the supplied elements.
func NewCPUSet(cpus ...int) CPUSet {
	b := NewBuilder()
	for _, c := range cpus {
		b.Add(c)
	}
	return b.Result()
}

// Size returns the number of elements in this set.
func (s CPUSet) Size() int {
	return len(s.Elems)
}

// IsEmpty returns true if there are zero elements in this set.
func (s CPUSet) IsEmpty() bool {
	return s.Size() == 0
}

// Contains returns true if the supplied element is present in this set.
func (s CPUSet) Contains(cpu int) bool {
	_, found := s.Elems[cpu]
	return found
}

// Equals returns true if the supplied set contains exactly the same elements
// as this set (s IsSubsetOf s2 and s2 IsSubsetOf s).
func (s CPUSet) Equals(s2 CPUSet) bool {
	return reflect.DeepEqual(s.Elems, s2.Elems)
}

// Filter returns a new CPU set that contains all of the elements from this
// set that match the supplied predicate, without mutating the source set.
func (s CPUSet) Filter(predicate func(int) bool) CPUSet {
	b := NewBuilder()
	for cpu := range s.Elems {
		if predicate(cpu) {
			b.Add(cpu)
		}
	}
	return b.Result()
}

// FilterNot returns a new CPU set that contains all of the elements from this
// set that do not match the supplied predicate, without mutating the source
// set.
func (s CPUSet) FilterNot(predicate func(int) bool) CPUSet {
	b := NewBuilder()
	for cpu := range s.Elems {
		if !predicate(cpu) {
			b.Add(cpu)
		}
	}
	return b.Result()
}

// IsSubsetOf returns true if the supplied set contains all the elements
func (s CPUSet) IsSubsetOf(s2 CPUSet) bool {
	result := true
	for cpu := range s.Elems {
		if !s2.Contains(cpu) {
			result = false
			break
		}
	}
	return result
}

// Union returns a new CPU set that contains all of the elements from this
// set and all of the elements from the supplied set, without mutating
// either source set.
func (s CPUSet) Union(s2 CPUSet) CPUSet {
	b := NewBuilder()
	for cpu := range s.Elems {
		b.Add(cpu)
	}
	for cpu := range s2.Elems {
		b.Add(cpu)
	}
	return b.Result()
}

// UnionAll returns a new CPU set that contains all of the elements from this
// set and all of the elements from the supplied sets, without mutating
// either source set.
func (s CPUSet) UnionAll(s2 []CPUSet) CPUSet {
	b := NewBuilder()
	for cpu := range s.Elems {
		b.Add(cpu)
	}
	for _, cs := range s2 {
		for cpu := range cs.Elems {
			b.Add(cpu)
		}
	}
	return b.Result()
}

// Intersection returns a new CPU set that contains all of the elements
// that are present in both this set and the supplied set, without mutating
// either source set.
func (s CPUSet) Intersection(s2 CPUSet) CPUSet {
	return s.Filter(func(cpu int) bool { return s2.Contains(cpu) })
}

// Difference returns a new CPU set that contains all of the elements that
// are present in this set and not the supplied set, without mutating either
// source set.
func (s CPUSet) Difference(s2 CPUSet) CPUSet {
	return s.FilterNot(func(cpu int) bool { return s2.Contains(cpu) })
}

// ToSlice returns a slice of integers that contains all elements from
// this set.
func (s CPUSet) ToSlice() []int {
	result := []int{}
	for cpu := range s.Elems {
		result = append(result, cpu)
	}
	sort.Ints(result)
	return result
}

// ToSliceNoSort returns a slice of integers that contains all elements from
// this set.
func (s CPUSet) ToSliceNoSort() []int {
	result := []int{}
	for cpu := range s.Elems {
		result = append(result, cpu)
	}
	return result
}

// String returns a new string representation of the elements in this CPU set
// in canonical linux CPU list format.
//
// See: http://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS
func (s CPUSet) String() string {
	if s.IsEmpty() {
		return ""
	}

	elems := s.ToSlice()

	type rng struct {
		start int
		end   int
	}

	ranges := []rng{{elems[0], elems[0]}}

	for i := 1; i < len(elems); i++ {
		lastRange := &ranges[len(ranges)-1]
		// if this element is adjacent to the high end of the last range
		if elems[i] == lastRange.end+1 {
			// then extend the last range to include this element
			lastRange.end = elems[i]
			continue
		}
		// otherwise, start a new range beginning with this element
		ranges = append(ranges, rng{elems[i], elems[i]})
	}

	// construct string from ranges
	var result bytes.Buffer
	for _, r := range ranges {
		if r.start == r.end {
			result.WriteString(strconv.Itoa(r.start))
		} else {
			result.WriteString(fmt.Sprintf("%d-%d", r.start, r.end))
		}
		result.WriteString(",")
	}
	return strings.TrimRight(result.String(), ",")
}

func (s CPUSet) LabelString() string {
	if s.IsEmpty() {
		return ""
	}

	elems := s.ToSlice()

	type rng struct {
		start int
		end   int
	}

	ranges := []rng{{elems[0], elems[0]}}

	for i := 1; i < len(elems); i++ {
		lastRange := &ranges[len(ranges)-1]
		// if this element is adjacent to the high end of the last range
		if elems[i] == lastRange.end+1 {
			// then extend the last range to include this element
			lastRange.end = elems[i]
			continue
		}
		// otherwise, start a new range beginning with this element
		ranges = append(ranges, rng{elems[i], elems[i]})
	}

	// construct string from ranges
	var result bytes.Buffer
	for _, r := range ranges {
		if r.start == r.end {
			result.WriteString(strconv.Itoa(r.start))
		} else {
			result.WriteString(fmt.Sprintf("%d-%d", r.start, r.end))
		}
		result.WriteString("_")
	}
	return strings.TrimRight(result.String(), "_")
}

// MustParse CPUSet constructs a new CPU set from a Linux CPU list formatted
// string. Unlike Parse, it does not return an error but rather panics if the
// input cannot be used to construct a CPU set.
func MustParse(s string) CPUSet {
	res, err := Parse(s)
	if err != nil {
		log.Fatalf("unable to parse [%s] as CPUSet: %v", s, err)
	}
	return res
}

// Parse CPUSet constructs a new CPU set from a Linux CPU list formatted string.
//
// See: http://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS
func Parse(s string) (CPUSet, error) {
	b := NewBuilder()

	// Handle empty string.
	if s == "" {
		return b.Result(), nil
	}

	// Split CPU list string:
	// "0-5,34,46-48 => ["0-5", "34", "46-48"]
	ranges := strings.Split(s, ",")

	for _, r := range ranges {
		boundaries := strings.Split(r, "-")
		if len(boundaries) == 1 {
			// Handle ranges that consist of only one element like "34".
			elem, err := strconv.Atoi(boundaries[0])
			if err != nil {
				return NewCPUSet(), err
			}
			b.Add(elem)
		} else if len(boundaries) == 2 {
			// Handle multi-element ranges like "0-5".
			start, err := strconv.Atoi(boundaries[0])
			if err != nil {
				return NewCPUSet(), err
			}
			end, err := strconv.Atoi(boundaries[1])
			if err != nil {
				return NewCPUSet(), err
			}
			// Add all elements to the result.
			// e.g. "0-5", "46-48" => [0, 1, 2, 3, 4, 5, 46, 47, 48].
			for e := start; e <= end; e++ {
				b.Add(e)
			}
		}
	}
	return b.Result(), nil
}

// Clone returns a copy of this CPU set.
func (s CPUSet) Clone() CPUSet {
	b := NewBuilder()
	for elem := range s.Elems {
		b.Add(elem)
	}
	return b.Result()
}

package noderesource

import (
	"reflect"
	"strings"
)

// Builder is a mutable builder for ElemSet. Functions that mutate instances
// of this type are not thread-safe.
type Builder struct {
	result ElemSet
	done   bool
}

// NewBuilder returns a mutable ElemSet builder.
func NewBuilder() Builder {
	return Builder{
		result: ElemSet{
			Elems: map[string]struct{}{},
		},
	}
}

// Add adds the supplied elements to the result. Calling Add after calling
// Result has no effect.
func (b Builder) Add(elems ...string) {
	if b.done {
		return
	}
	for _, elem := range elems {
		b.result.Elems[elem] = struct{}{}
	}
}

// Result returns the result ElemSet containing all elements that were
// previously added to this builder. Subsequent calls to Add have no effect.
func (b *Builder) Result() ElemSet {
	b.done = true
	return b.result
}

// ElemSet is a thread-safe, immutable set-like data structure for Core IDs.
type ElemSet struct {
	Elems map[string]struct{} `json:"elems,omitempty"`
}

// NewElemSet returns a new ElemSet containing the supplied elements.
func NewElemSet(cores ...string) ElemSet {
	b := NewBuilder()
	for _, c := range cores {
		b.Add(c)
	}
	return b.Result()
}

// Size returns the number of elements in this set.
func (s ElemSet) Size() int {
	return len(s.Elems)
}

// IsEmpty returns true if there are zero elements in this set.
func (s ElemSet) IsEmpty() bool {
	return s.Size() == 0
}

// Contains returns true if the supplied element is present in this set.
func (s ElemSet) Contains(core string) bool {
	_, found := s.Elems[core]
	return found
}

// Equals returns true if the supplied set contains exactly the same elements
// as this set (s IsSubsetOf s2 and s2 IsSubsetOf s).
func (s ElemSet) Equals(s2 ElemSet) bool {
	return reflect.DeepEqual(s.Elems, s2.Elems)
}

// Filter returns a new Core set that contains all of the elements from this
// set that match the supplied predicate, without mutating the source set.
func (s ElemSet) Filter(predicate func(string) bool) ElemSet {
	b := NewBuilder()
	for core := range s.Elems {
		if predicate(core) {
			b.Add(core)
		}
	}
	return b.Result()
}

// FilterNot returns a new Core set that contains all of the elements from this
// set that do not match the supplied predicate, without mutating the source
// set.
func (s ElemSet) FilterNot(predicate func(string) bool) ElemSet {
	b := NewBuilder()
	for core := range s.Elems {
		if !predicate(core) {
			b.Add(core)
		}
	}
	return b.Result()
}

// IsSubsetOf returns true if the supplied set contains all the elements
func (s ElemSet) IsSubsetOf(s2 ElemSet) bool {
	result := true
	for cpu := range s.Elems {
		if !s2.Contains(cpu) {
			result = false
			break
		}
	}
	return result
}

// Union returns a new Core set that contains all of the elements from this
// set and all of the elements from the supplied set, without mutating
// either source set.
func (s ElemSet) Union(s2 ElemSet) ElemSet {
	b := NewBuilder()
	for cpu := range s.Elems {
		b.Add(cpu)
	}
	for cpu := range s2.Elems {
		b.Add(cpu)
	}
	return b.Result()
}

// UnionAll returns a new Core set that contains all of the elements from this
// set and all of the elements from the supplied sets, without mutating
// either source set.
func (s ElemSet) UnionAll(s2 []ElemSet) ElemSet {
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

// Intersection returns a new Core set that contains all of the elements
// that are present in both this set and the supplied set, without mutating
// either source set.
func (s ElemSet) Intersection(s2 ElemSet) ElemSet {
	return s.Filter(func(core string) bool { return s2.Contains(core) })
}

// Difference returns a new Core set that contains all of the elements that
// are present in this set and not the supplied set, without mutating either
// source set.
func (s ElemSet) Difference(s2 ElemSet) ElemSet {
	return s.FilterNot(func(core string) bool { return s2.Contains(core) })
}

// ToSlice returns a slice of integers that contains all elements from
// this set.
func (s ElemSet) ToSlice() []string {
	result := []string{}
	for core := range s.Elems {
		result = append(result, core)
	}
	return result
}

// ToSliceNoSort returns a slice of integers that contains all elements from
// this set.
func (s ElemSet) ToSliceNoSort() []string {
	result := []string{}
	for core := range s.Elems {
		result = append(result, core)
	}
	return result
}

func (s ElemSet) String() string {
	if s.IsEmpty() {
		return ""
	}

	elems := s.ToSlice()
	return strings.Join(elems, ",")
}

// Clone returns a copy of this Core set.
func (s ElemSet) Clone() ElemSet {
	b := NewBuilder()
	for elem := range s.Elems {
		b.Add(elem)
	}
	return b.Result()
}

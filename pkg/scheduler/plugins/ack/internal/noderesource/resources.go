package noderesource

import (
	"fmt"
	"strconv"
	"strings"
)

// a group of resources whose resource type are the same
type Resources interface {
	// get the size of group resources
	Size() int
	// add resource group to me
	Add(resources Resources) Resources
	// remove the resource group from me
	Remove(resources Resources) Resources
	// return the resource type
	Type() ResourceType
	// clone the resource group
	Clone() Resources
	// string the resource group
	String() string
}

// CountableResources is a group of resources who have resource id
// for example: npu core has core id like: core1,core2...
type CountableResources ElemSet

func NewCountableResources(resourceIds []string) Resources {
	resources := NewElemSet(resourceIds...)
	return CountableResources(resources)
}

func (w CountableResources) Type() ResourceType {
	return CountableResource
}

func (w CountableResources) Size() int {
	e := ElemSet(w)
	return e.Size()
}

func (w CountableResources) Clone() Resources {
	e := ElemSet(w)
	return CountableResources(e.Clone())
}

func (w CountableResources) String() string {
	e := ElemSet(w)
	return e.String()
}

func (w CountableResources) Add(resources Resources) Resources {
	if resources == nil || resources.Size() == 0 {
		return w.Clone()
	}
	idString := resources.String()
	if idString == "" {
		return w.Clone()
	}
	ids := strings.Split(idString, ",")
	re := NewElemSet(ids...)
	e := ElemSet(w)
	return CountableResources(e.Union(re).Clone())

}

func (w CountableResources) Remove(resources Resources) Resources {
	if resources == nil || resources.Size() == 0 {
		return w.Clone()
	}
	idString := resources.String()
	if idString == "" {
		return w.Clone()
	}
	ids := strings.Split(idString, ",")
	re := NewElemSet(ids...)
	e := ElemSet(w)
	return CountableResources(e.Difference(re).Clone())
}

// NoneIDResources is a group of resources who have no resource id
// for example: gpu memory has no ids,we only care that how much gpu memory of the device
type NoneIDResources int

func NewNoneIDResources(size int) Resources {
	return NoneIDResources(size)
}

func (w NoneIDResources) Size() int {
	return int(w)
}

func (w NoneIDResources) Type() ResourceType {
	return NoneIDResource
}

func (w NoneIDResources) Clone() Resources {
	return w
}

func (w NoneIDResources) String() string {
	return fmt.Sprintf("%v", int(w))
}

func (w NoneIDResources) Add(resources Resources) Resources {
	if resources == nil || resources.Size() == 0 {
		return w.Clone()
	}
	r := resources.String()
	d, _ := strconv.Atoi(r)
	n := int(w) + d
	return NoneIDResources(n)
}

func (w NoneIDResources) Remove(resources Resources) Resources {
	if resources == nil || resources.Size() == 0 {
		return w.Clone()
	}
	r := resources.String()
	d, _ := strconv.Atoi(r)
	n := int(w) - d
	if n < 0 {
		n = 0
	}
	return NoneIDResources(n)
}

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

package nodesorting

import "sync/atomic"

// NodeIterator iterates over a list of nodes based on the defined policy
type NodeIterator interface {
	HasNext() bool
	Next() *Node
	Size() int32
	Reset()
}

// All iterators extend the base iterator
type baseIterator struct {
	NodeIterator
	countIdx int32
	size     int32
	nodes    []*Node
}

// Reset the iterator to start from the beginning
func (bi *baseIterator) Reset() {
	atomic.StoreInt32(&bi.countIdx, 0)
}

// HasNext returns true if there is a next element in the array.
// Returns false if there are no more elements or list is empty.
func (bi *baseIterator) HasNext() bool {
	return !(atomic.LoadInt32(&bi.countIdx)+1 > bi.size)
}

// Next returns the next element and advances to next element in array.
// Returns nil at the end of iteration.
func (bi *baseIterator) Next() *Node {
	if !bi.HasNext() {
		return nil
	}

	index := atomic.AddInt32(&bi.countIdx, 1)
	value := bi.nodes[index-1]
	return value
}

func (bi *baseIterator) Size() int32 {
	return bi.size
}

// Default iterator, wraps the base iterator.
// Iterates over the list from the start, position zero, to end.
type defaultNodeIterator struct {
	baseIterator
}

func NewDefaultNodeIterator(schedulerNodes []*Node) NodeIterator {
	it := &defaultNodeIterator{}
	it.nodes = schedulerNodes
	it.size = int32(len(schedulerNodes))
	return it
}

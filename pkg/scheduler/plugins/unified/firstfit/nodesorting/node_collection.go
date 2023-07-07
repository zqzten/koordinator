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

import (
	"fmt"
	"sync"

	"github.com/google/btree"
	"k8s.io/client-go/informers"
)

type NodeCollection interface {
	GetNodeCount() int
	GetNodes() []*Node
	GetNode(nodeName string) *Node
	GetNodeIterator() NodeIterator
	NodeUpdated(node *Node)
}

type nodeRef struct {
	node      *Node   // node reference
	nodeScore float64 // node score
}

func (nr nodeRef) Less(than btree.Item) bool {
	other, ok := than.(nodeRef)
	if !ok {
		return false
	}
	if nr.nodeScore < other.nodeScore {
		return true
	}
	if other.nodeScore < nr.nodeScore {
		return false
	}
	return nr.node.Name < other.node.Name
}

type baseNodeCollection struct {
	nsp         NodeSortingPolicy   // node sorting policy
	nodes       map[string]*nodeRef // nodes assigned to this collection
	sortedNodes *btree.BTree        // nodes sorted by score

	lock sync.RWMutex
}

func (nc *baseNodeCollection) scoreNode(node *Node) float64 {
	if nc.nsp == nil {
		return 0
	}
	return nc.nsp.ScoreNode(node)
}

func (nc *baseNodeCollection) AddNode(node *Node) error {
	nc.lock.Lock()
	defer nc.lock.Unlock()

	if node == nil {
		return fmt.Errorf("node cannot be nil")
	}
	if nc.nodes[node.Name] != nil {
		return fmt.Errorf("has an existing node %s, node name must be unique", node.Name)
	}
	nref := nodeRef{
		node:      node,
		nodeScore: nc.scoreNode(node),
	}
	nc.nodes[node.Name] = &nref
	nc.sortedNodes.ReplaceOrInsert(nref)
	return nil
}

func (nc *baseNodeCollection) RemoveNode(nodeName string) *Node {
	nc.lock.Lock()
	defer nc.lock.Unlock()
	nref := nc.nodes[nodeName]
	if nref == nil {
		return nil
	}

	// Remove node from list of tracked nodes
	nc.sortedNodes.Delete(*nref)
	delete(nc.nodes, nodeName)

	return nref.node
}

func (nc *baseNodeCollection) NodeUpdated(node *Node) {
	nc.lock.Lock()
	defer nc.lock.Unlock()

	nref := nc.nodes[node.Name]
	if nref == nil {
		return
	}

	updatedScore := nc.scoreNode(node)
	if nref.nodeScore != updatedScore {
		nc.sortedNodes.Delete(*nref)
		nref.nodeScore = nc.scoreNode(node)
		nc.sortedNodes.ReplaceOrInsert(*nref)
	}
}

func (nc *baseNodeCollection) GetNode(nodeName string) *Node {
	nc.lock.RLock()
	defer nc.lock.RUnlock()
	nref := nc.nodes[nodeName]
	if nref == nil {
		return nil
	}
	return nref.node
}

func (nc *baseNodeCollection) GetNodeCount() int {
	nc.lock.RLock()
	defer nc.lock.RUnlock()
	return len(nc.nodes)
}

func (nc *baseNodeCollection) GetNodes() []*Node {
	nc.lock.RLock()
	defer nc.lock.RUnlock()
	nodes := make([]*Node, 0, len(nc.nodes))
	for _, nref := range nc.nodes {
		nodes = append(nodes, nref.node)
	}
	return nodes
}

func (nc *baseNodeCollection) GetNodeIterator() NodeIterator {
	tree := nc.cloneSortedNodes()

	length := tree.Len()
	if length == 0 {
		return nil
	}

	nodes := make([]*Node, 0, length)
	tree.Ascend(func(item btree.Item) bool {
		node := item.(nodeRef).node
		nodes = append(nodes, node)
		return true
	})

	if len(nodes) == 0 {
		return nil
	}

	return NewDefaultNodeIterator(nodes)
}

func (nc *baseNodeCollection) cloneSortedNodes() *btree.BTree {
	nc.lock.Lock()
	defer nc.lock.Unlock()

	return nc.sortedNodes.Clone()
}

func NewNodeCollection(factory informers.SharedInformerFactory) NodeCollection {
	collection := &baseNodeCollection{
		nsp:         &fairnessNodeSortingPolicy{resourceWeights: defaultResourceWeights()},
		nodes:       make(map[string]*nodeRef),
		sortedNodes: btree.New(7), // Degree=7 here is experimentally the most efficient for up to around 5k nodes
	}
	registerNodeEventHandler(collection, factory)
	registerPodEventHandler(collection, factory)
	return collection
}

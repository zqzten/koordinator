package noderesource

import (
	"sync"

	v1 "k8s.io/api/core/v1"
)

type NodeResourceManager struct {
	caches map[v1.ResourceName]*NodeResourceCache
	lock   *sync.RWMutex
}

func NewNodeResourceManager() *NodeResourceManager {
	return &NodeResourceManager{
		caches: map[v1.ResourceName]*NodeResourceCache{},
		lock:   new(sync.RWMutex),
	}
}

func (n *NodeResourceManager) ResourceCacheIsExisted(resourceName v1.ResourceName) bool {
	n.lock.RLock()
	defer n.lock.RUnlock()
	_, ok := n.caches[resourceName]
	return ok
}

func (n *NodeResourceManager) GetNodeResourceCache(resourceName v1.ResourceName) *NodeResourceCache {
	n.lock.Lock()
	defer n.lock.Unlock()
	cache, ok := n.caches[resourceName]
	if !ok {
		cache = NewNodeResourceCache(resourceName)
		n.caches[resourceName] = cache
	}
	return cache
}

func (n *NodeResourceManager) Clone() *NodeResourceManager {
	n.lock.Lock()
	defer n.lock.Unlock()
	c := &NodeResourceManager{
		caches: map[v1.ResourceName]*NodeResourceCache{},
		lock:   new(sync.RWMutex),
	}
	for name, cache := range n.caches {
		c.caches[name] = cache.Clone()
	}
	return c
}

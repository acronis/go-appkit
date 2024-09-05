/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"container/list"
	"fmt"
	"sync"
)

type cacheEntry[K comparable, V any] struct {
	key   K
	value V
}

// LRUCache represents an LRU cache with eviction mechanism and Prometheus metrics.
type LRUCache[K comparable, V any] struct {
	maxEntries int

	mu      sync.RWMutex
	lruList *list.List
	cache   map[K]*list.Element // map of cache entries, value is a lruList element

	metricsCollector MetricsCollector
}

// New creates a new LRUCache with the provided maximum number of entries.
func New[K comparable, V any](maxEntries int, metricsCollector MetricsCollector) (*LRUCache[K, V], error) {
	if maxEntries <= 0 {
		return nil, fmt.Errorf("maxEntries must be greater than 0")
	}
	if metricsCollector == nil {
		metricsCollector = disabledMetrics{}
	}
	return &LRUCache[K, V]{
		maxEntries:       maxEntries,
		lruList:          list.New(),
		cache:            make(map[K]*list.Element),
		metricsCollector: metricsCollector,
	}, nil
}

// Get returns a value from the cache by the provided key and type.
func (c *LRUCache[K, V]) Get(key K) (value V, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.get(key)
}

// Add adds a value to the cache with the provided key and type.
// If the cache is full, the oldest entry will be removed.
func (c *LRUCache[K, V]) Add(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lruList.MoveToFront(elem)
		elem.Value = &cacheEntry[K, V]{key: key, value: value}
		return
	}
	c.addNew(key, value)
}

func (c *LRUCache[K, V]) GetOrAdd(key K, valueProvider func() V) (value V, exists bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if value, exists = c.get(key); exists {
		return value, exists
	}
	value = valueProvider()
	c.addNew(key, value)
	return value, false
}

// Remove removes a value from the cache by the provided key and type.
func (c *LRUCache[K, V]) Remove(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[key]
	if !ok {
		return false
	}

	c.lruList.Remove(elem)
	delete(c.cache, key)
	c.metricsCollector.SetAmount(len(c.cache))
	return true
}

// Purge clears the cache.
// Keep in mind that this method does not reset the cache size
// and does not reset Prometheus metrics except for the total number of entries.
// All removed entries will not be counted as evictions.
func (c *LRUCache[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metricsCollector.SetAmount(0)
	c.cache = make(map[K]*list.Element)
	c.lruList.Init()
}

// Resize changes the cache size and returns the number of evicted entries.
func (c *LRUCache[K, V]) Resize(size int) (evicted int) {
	if size <= 0 {
		return 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxEntries = size
	evicted = len(c.cache) - size
	if evicted <= 0 {
		return
	}
	for i := 0; i < evicted; i++ {
		_ = c.removeOldest()
	}
	c.metricsCollector.SetAmount(len(c.cache))
	c.metricsCollector.AddEvictions(evicted)
	return evicted
}

// Len returns the number of items in the cache.
func (c *LRUCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

func (c *LRUCache[K, V]) get(key K) (value V, ok bool) {
	if elem, hit := c.cache[key]; hit {
		c.lruList.MoveToFront(elem)
		c.metricsCollector.IncHits()
		return elem.Value.(*cacheEntry[K, V]).value, true
	}
	c.metricsCollector.IncMisses()
	return value, false
}

func (c *LRUCache[K, V]) addNew(key K, value V) {
	c.cache[key] = c.lruList.PushFront(&cacheEntry[K, V]{key: key, value: value})
	if len(c.cache) <= c.maxEntries {
		c.metricsCollector.SetAmount(len(c.cache))
		return
	}
	if evictedEntry := c.removeOldest(); evictedEntry != nil {
		c.metricsCollector.AddEvictions(1)
	}
}

func (c *LRUCache[K, V]) removeOldest() *cacheEntry[K, V] {
	elem := c.lruList.Back()
	if elem == nil {
		return nil
	}
	c.lruList.Remove(elem)
	entry := elem.Value.(*cacheEntry[K, V])
	delete(c.cache, entry.key)
	return entry
}

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

// EntryType is a type of storing in cache entries.
type EntryType int

// EntryTypeDefault is a default entry type.
const EntryTypeDefault EntryType = 0

type cacheKey[K comparable] struct {
	key       K
	entryType EntryType
}

type cacheEntry[K comparable] struct {
	key   cacheKey[K]
	value interface{}
}

// LRUCache represents an LRU cache with eviction mechanism and Prometheus metrics.
type LRUCache[K comparable] struct {
	maxEntries int

	mu      sync.RWMutex
	lruList *list.List
	cache   map[cacheKey[K]]*list.Element // map of cache entries, value is a lruList element

	MetricsCollector *MetricsCollector
}

// New creates a new LRUCache with the provided maximum number of entries.
func New[K comparable](maxEntries int, metricsCollector *MetricsCollector) (*LRUCache[K], error) {
	if maxEntries <= 0 {
		return nil, fmt.Errorf("maxEntries must be greater than 0")
	}
	return &LRUCache[K]{
		maxEntries:       maxEntries,
		lruList:          list.New(),
		cache:            make(map[cacheKey[K]]*list.Element),
		MetricsCollector: metricsCollector,
	}, nil
}

// Get returns a value from the cache by the provided key and type.
func (c *LRUCache[K]) Get(key K, entryType EntryType) (value interface{}, ok bool) {
	metrics := c.MetricsCollector.getEntryTypeMetrics(entryType)

	defer func() {
		if ok {
			metrics.HitsTotal.Inc()
		} else {
			metrics.MissesTotal.Inc()
		}
	}()

	cKey := cacheKey[K]{key, entryType}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, hit := c.cache[cKey]; hit {
		c.lruList.MoveToFront(elem)
		return elem.Value.(*cacheEntry[K]).value, true
	}
	return nil, false
}

// Add adds a value to the cache with the provided key and type.
// If the cache is full, the oldest entry will be removed.
func (c *LRUCache[K]) Add(key K, value interface{}, entryType EntryType) {
	var evictedEntry *cacheEntry[K]

	defer func() {
		if evictedEntry != nil {
			c.MetricsCollector.getEntryTypeMetrics(evictedEntry.key.entryType).EvictionsTotal.Inc()
		}
	}()

	cKey := cacheKey[K]{key, entryType}
	entry := &cacheEntry[K]{key: cKey, value: value}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[cKey]; ok {
		c.lruList.MoveToFront(elem)
		elem.Value = entry
		return
	}

	c.cache[cKey] = c.lruList.PushFront(entry)
	c.MetricsCollector.getEntryTypeMetrics(cKey.entryType).Amount.Inc()
	if len(c.cache) <= c.maxEntries {
		return
	}
	if evictedEntry = c.removeOldest(); evictedEntry != nil {
		c.MetricsCollector.getEntryTypeMetrics(evictedEntry.key.entryType).Amount.Dec()
	}
}

// Remove removes a value from the cache by the provided key and type.
func (c *LRUCache[K]) Remove(key K, entryType EntryType) bool {
	cKey := cacheKey[K]{key, entryType}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[cKey]
	if !ok {
		return false
	}

	c.lruList.Remove(elem)
	delete(c.cache, cKey)
	c.MetricsCollector.getEntryTypeMetrics(entryType).Amount.Dec()
	return true
}

// Purge clears the cache.
func (c *LRUCache[K]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, etMetrics := range c.MetricsCollector.entryTypeMetrics {
		etMetrics.Amount.Set(0)
	}
	c.cache = make(map[cacheKey[K]]*list.Element)
	c.lruList.Init()
}

// Resize changes the cache size.
// Note that resizing the cache may cause some entries to be evicted.
func (c *LRUCache[K]) Resize(size int) {
	if size <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxEntries = size
	diff := len(c.cache) - size
	if diff <= 0 {
		return
	}

	rmCounts := make([]int, len(c.MetricsCollector.entryTypeMetrics))
	for i := 0; i < diff; i++ {
		if rmEntry := c.removeOldest(); rmEntry != nil {
			rmCounts[rmEntry.key.entryType]++
		}
	}
	for et, cnt := range rmCounts {
		typeMetrics := c.MetricsCollector.getEntryTypeMetrics(EntryType(et))
		typeMetrics.Amount.Sub(float64(cnt))
		typeMetrics.EvictionsTotal.Add(float64(cnt))
	}
}

// Len returns the number of items in the cache.
func (c *LRUCache[K]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

func (c *LRUCache[K]) removeOldest() *cacheEntry[K] {
	elem := c.lruList.Back()
	if elem == nil {
		return nil
	}
	c.lruList.Remove(elem)
	entry := elem.Value.(*cacheEntry[K])
	delete(c.cache, entry.key)
	return entry
}

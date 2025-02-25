/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRUCache(t *testing.T) {
	bob := User{"375ea40b-c49f-43a9-ba49-df720d21d274", "Bob"}
	john := User{"06f84bd8-df55-4de4-b062-ce4a12fad14c", "John"}
	piter := User{"d69fa1a4-84ad-48ac-a8a9-1cbd4092ac9f", "Piter"}
	firstPost := Post{"5ce98a1e-0090-4681-80b4-4c7fdb849d78", "My first post."}

	tests := []struct {
		name                    string
		maxEntries              int
		fn                      func(t *testing.T, adminCache *LRUCache[string, User], customerCache *LRUCache[string, User], postCache *LRUCache[string, Post])
		adminExpectedMetrics    expectedMetrics
		customerExpectedMetrics expectedMetrics
		postExpectedMetrics     expectedMetrics
	}{
		{
			name:       "attempt to get not existing keys",
			maxEntries: 100,
			fn: func(t *testing.T, adminCache *LRUCache[string, User], customerCache *LRUCache[string, User], postCache *LRUCache[string, Post]) {
				var found bool
				_, found = adminCache.Get("not_existing_key_1")
				require.False(t, found)
				_, found = adminCache.Get("not_existing_key_2")
				require.False(t, found)
				_, found = customerCache.Get("not_existing_key_3")
				require.False(t, found)
				_, found = postCache.Get("not_existing_key_4")
				require.False(t, found)
			},
			adminExpectedMetrics:    expectedMetrics{MissesTotal: 2},
			customerExpectedMetrics: expectedMetrics{MissesTotal: 1},
			postExpectedMetrics:     expectedMetrics{MissesTotal: 1},
		},
		{
			name:       "add entries and get them",
			maxEntries: 100,
			fn: func(t *testing.T, adminCache *LRUCache[string, User], customerCache *LRUCache[string, User], postCache *LRUCache[string, Post]) {
				adminCache.Add(bob.ID, bob)
				customerCache.Add(john.ID, john)
				customerCache.Add(piter.ID, piter)
				postCache.Add(firstPost.ID, firstPost)

				user, found := adminCache.Get(bob.ID)
				require.True(t, found)
				require.Equal(t, bob, user)

				user, found = customerCache.Get(john.ID)
				require.True(t, found)
				require.Equal(t, john, user)

				user, found = customerCache.Get(piter.ID)
				require.True(t, found)
				require.Equal(t, piter, user)

				for i := 0; i < 10; i++ {
					post, found := postCache.Get(firstPost.ID)
					require.True(t, found)
					require.Equal(t, firstPost, post)
				}
			},
			adminExpectedMetrics:    expectedMetrics{EntriesAmount: 1, HitsTotal: 1},
			customerExpectedMetrics: expectedMetrics{EntriesAmount: 2, HitsTotal: 2},
			postExpectedMetrics:     expectedMetrics{EntriesAmount: 1, HitsTotal: 10},
		},
		{
			name:       "add entries with evictions",
			maxEntries: 2,
			fn: func(t *testing.T, _ *LRUCache[string, User], customerCache *LRUCache[string, User], _ *LRUCache[string, Post]) {
				alice := User{ID: "a9fb0f2b-2675-4287-bedd-4ae3ba0b1b36", Name: "Alice"}
				kate := User{ID: "96c8b9a0-0a70-4b49-85e6-b514009b62a1", Name: "Kate"}

				// Fill cache with entries.
				customerCache.Add(john.ID, john)
				customerCache.Add(piter.ID, piter)

				// Add a new entry, which should evict the oldest one (John).
				customerCache.Add(alice.ID, alice)
				_, found := customerCache.Get(john.ID) // John should be evicted.
				require.False(t, found)
				user, found := customerCache.Get(alice.ID)
				require.True(t, found)
				require.Equal(t, user, alice)
				user, found = customerCache.Get(piter.ID)
				require.True(t, found)
				require.Equal(t, user, piter)

				// Add a new entry, which should evict the oldest one (Alice).
				customerCache.Add(kate.ID, kate)
				_, found = customerCache.Get(alice.ID) // Alice should be evicted.
				require.False(t, found)
				user, found = customerCache.Get(piter.ID)
				require.True(t, found)
				require.Equal(t, user, piter)
				user, found = customerCache.Get(kate.ID)
				require.True(t, found)
				require.Equal(t, user, kate)
			},
			customerExpectedMetrics: expectedMetrics{EntriesAmount: 2, HitsTotal: 4, MissesTotal: 2, EvictionsTotal: 2},
		},
		{
			name:       "get or add",
			maxEntries: 100,
			fn: func(t *testing.T, _ *LRUCache[string, User], customerCache *LRUCache[string, User], _ *LRUCache[string, Post]) {
				_, found := customerCache.GetOrAdd(john.ID, func() User {
					return john
				})
				require.False(t, found)
				_, found = customerCache.GetOrAdd(john.ID, func() User {
					return john
				})
				require.True(t, found)
			},
			customerExpectedMetrics: expectedMetrics{EntriesAmount: 1, HitsTotal: 1, MissesTotal: 1},
		},
		{
			name:       "remove entries",
			maxEntries: 100,
			fn: func(t *testing.T, _ *LRUCache[string, User], customerCache *LRUCache[string, User], _ *LRUCache[string, Post]) {
				customerCache.Add(john.ID, john)
				customerCache.Add(piter.ID, piter)
				require.True(t, customerCache.Remove(john.ID))
				require.False(t, customerCache.Remove(john.ID))
				require.False(t, customerCache.Remove("not_existing_key"))
			},
			customerExpectedMetrics: expectedMetrics{EntriesAmount: 1},
		},
		{
			name:       "resize",
			maxEntries: 100,
			fn: func(t *testing.T, _ *LRUCache[string, User], customerCache *LRUCache[string, User], _ *LRUCache[string, Post]) {
				for _, user := range []User{bob, john, piter} {
					customerCache.Add(user.ID, user)
				}

				// Resize without evictions.
				customerCache.Resize(3)
				for _, user := range []User{bob, john, piter} {
					_, found := customerCache.Get(user.ID)
					require.True(t, found)
				}

				// Resize with evictions.
				customerCache.Resize(2)
				_, found := customerCache.Get(bob.ID)
				require.False(t, found)
				_, found = customerCache.Get(john.ID)
				require.True(t, found)
				_, found = customerCache.Get(piter.ID)
				require.True(t, found)
			},
			customerExpectedMetrics: expectedMetrics{
				EntriesAmount:  2,
				HitsTotal:      5,
				MissesTotal:    1,
				EvictionsTotal: 1,
			},
		},
		{
			name:       "purge",
			maxEntries: 100,
			fn: func(t *testing.T, _ *LRUCache[string, User], customerCache *LRUCache[string, User], _ *LRUCache[string, Post]) {
				customerCache.Add(john.ID, john)
				customerCache.Add(piter.ID, piter)
				customerCache.Purge()
				_, found := customerCache.Get(john.ID)
				require.False(t, found)
				_, found = customerCache.Get(piter.ID)
				require.False(t, found)
			},
			customerExpectedMetrics: expectedMetrics{EntriesAmount: 0, MissesTotal: 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userMetrics := NewPrometheusMetricsWithOpts(PrometheusMetricsOpts{Namespace: "user", CurriedLabelNames: []string{"type"}})
			userMetrics.MustRegister()
			defer userMetrics.Unregister()

			postMetrics := NewPrometheusMetricsWithOpts(PrometheusMetricsOpts{Namespace: "post"})
			postMetrics.MustRegister()
			defer postMetrics.Unregister()

			adminMetrics := userMetrics.MustCurryWith(map[string]string{"type": "admin"})
			adminCache, err := New[string, User](tt.maxEntries, adminMetrics)
			require.NoError(t, err)

			customerMetrics := userMetrics.MustCurryWith(map[string]string{"type": "customer"})
			customerCache, err := New[string, User](tt.maxEntries, customerMetrics)
			require.NoError(t, err)

			postCache, err := New[string, Post](tt.maxEntries, postMetrics)
			require.NoError(t, err)

			tt.fn(t, adminCache, customerCache, postCache)

			assertPrometheusMetrics(t, tt.adminExpectedMetrics, adminMetrics)
			assertPrometheusMetrics(t, tt.customerExpectedMetrics, customerMetrics)
			assertPrometheusMetrics(t, tt.postExpectedMetrics, postMetrics)
		})
	}
}

func TestLRUCache_TTL(t *testing.T) {
	const ttl = 100 * time.Millisecond

	tests := []struct {
		name           string
		defaultTTL     time.Duration
		keySpecificTTL time.Duration
		expectExpired  bool
		sleepDuration  time.Duration
	}{
		{
			name:          "defaultTTL small, expires",
			defaultTTL:    ttl,
			expectExpired: true,
			sleepDuration: ttl * 2,
		},
		{
			name:          "defaultTTL small, not expired if short sleep",
			defaultTTL:    ttl,
			expectExpired: false,
			sleepDuration: ttl / 2,
		},
		{
			name:           "no defaultTTL, customTTL small, expires",
			keySpecificTTL: ttl,
			expectExpired:  true,
			sleepDuration:  ttl * 2,
		},
		{
			name:           "no defaultTTL, customTTL small, not expired if short sleep",
			keySpecificTTL: ttl,
			expectExpired:  false,
			sleepDuration:  ttl / 2,
		},
		{
			name:           "both defaultTTL and customTTL are used",
			defaultTTL:     ttl,
			keySpecificTTL: ttl / 4,
			sleepDuration:  ttl / 2,
			expectExpired:  true,
		},
		{
			name:          "no TTL, never expires",
			expectExpired: false,
			sleepDuration: ttl,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a cache with the given default TTL
			cache, err := NewWithOpts[string, string](10, nil, Options{DefaultTTL: tt.defaultTTL})
			require.NoError(t, err)

			key, value := "some-key", "some-value"

			if tt.keySpecificTTL != 0 {
				cache.AddWithTTL(key, value, tt.keySpecificTTL)
			} else {
				cache.Add(key, value)
			}

			// Immediately after adding, we should be able to get the item
			v, found := cache.Get(key)
			require.True(t, found, "expected to find the item right after add")
			require.Equal(t, value, v)

			time.Sleep(tt.sleepDuration)

			require.Equal(t, 1, cache.Len(),
				"expected the item to still be in the cache, because it hasn't been accessed yet")

			// Re-check item
			v, found = cache.Get(key)
			if tt.expectExpired {
				require.False(t, found, "expected the item to be expired")
				require.Equal(t, 0, cache.Len(), "expected the item to be removed from the cache")
			} else {
				require.True(t, found, "expected the item to still be in the cache")
				require.Equal(t, value, v)
			}
		})
	}
}

func TestLRUCache_PeriodicCleanup(t *testing.T) {
	const ttl = 100 * time.Millisecond

	// We'll create a short-lived item but never manually Get it.
	// We'll rely on periodic cleanup to remove it from the cache.
	cache, err := New[string, string](10, nil)
	require.NoError(t, err)

	// Start periodic cleanup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const cleanupInterval = ttl / 2
	go cache.RunPeriodicCleanup(ctx, cleanupInterval)

	const key1, value1 = "key1", "value1"
	const key2, value2 = "key2", "value2"
	cache.AddWithTTL(key1, value1, ttl)
	cache.Add(key2, value2) // no TTL, should not be removed

	// Immediately found
	v, found := cache.Get(key1)
	require.True(t, found)
	require.Equal(t, value1, v)
	v, found = cache.Get(key2)
	require.True(t, found)
	require.Equal(t, value2, v)
	require.Equal(t, 2, cache.Len())

	// Wait enough time for TTL to expire and cleanup to run
	time.Sleep(ttl * 2)

	// The item should be removed by periodic cleanup
	require.Equal(t, 1, cache.Len())
	_, found = cache.Get(key1)
	require.False(t, found)
	_, found = cache.Get(key2)
	require.True(t, found)
}

func TestLRUCache_GetOrLoad(t *testing.T) {
	t.Run("key exists", func(t *testing.T) {
		metrics := NewPrometheusMetrics()
		cache, err := New[string, int](10, metrics)
		require.NoError(t, err)

		// Pre-populate the cache.
		cache.Add("existing", 42)

		// Call GetOrLoad with a load function that should not be invoked.
		callCount := 0
		val, exists, err := cache.GetOrLoad("existing", func(key string) (int, error) {
			callCount++
			return 73, nil
		})
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, 42, val)
		require.Equal(t, 0, callCount)

		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 1, HitsTotal: 1}, metrics)
	})

	t.Run("load value, success", func(t *testing.T) {
		metrics := NewPrometheusMetrics()
		cache, err := New[string, int](10, metrics)
		require.NoError(t, err)

		callCount := 0

		// First call: key does not exist, so load function is called.
		val, exists, err := cache.GetOrLoad("key", func(key string) (int, error) {
			callCount++
			return 456, nil
		})
		require.NoError(t, err)
		require.False(t, exists) // fresh load returns exists == false
		require.Equal(t, 456, val)
		require.Equal(t, 1, callCount)
		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 1, MissesTotal: 1}, metrics)

		// Second call: the value should be cached.
		val, exists, err = cache.GetOrLoad("key", func(key string) (int, error) {
			callCount++
			return 789, nil
		})
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, 456, val)
		require.Equal(t, 1, callCount) // load function is not invoked again
		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 1, MissesTotal: 1, HitsTotal: 1}, metrics)
	})

	t.Run("load value, error", func(t *testing.T) {
		loadErr := errors.New("load error")

		metrics := NewPrometheusMetrics()
		cache, err := New[string, int](10, metrics)
		require.NoError(t, err)

		callCount := 0

		// The first call returns an error.
		_, _, err = cache.GetOrLoad("errorKey", func(key string) (int, error) {
			callCount++
			return 0, loadErr
		})
		require.ErrorIs(t, err, loadErr)
		require.Equal(t, 1, callCount)
		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 0, MissesTotal: 1}, metrics)

		// The next call should try to load again.
		_, _, err = cache.GetOrLoad("errorKey", func(key string) (int, error) {
			callCount++
			return 0, loadErr
		})
		require.ErrorIs(t, err, loadErr)
		require.Equal(t, 2, callCount)
		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 0, MissesTotal: 2}, metrics)
	})

	t.Run("load value, single-flight", func(t *testing.T) {
		metrics := NewPrometheusMetrics()
		cache, err := NewWithOpts[string, int](10, metrics, Options{DefaultTTL: time.Minute})
		require.NoError(t, err)

		var callCount atomic.Int64
		loadFunc := func(key string) (int, error) {
			time.Sleep(100 * time.Millisecond) // simulate some delay to force overlapping calls
			callCount.Add(1)
			return 999, nil
		}

		const numGoroutines = 20
		var wg sync.WaitGroup
		results := make([]int, numGoroutines)
		existsFlags := make([]bool, numGoroutines)
		errs := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				v, exists, err := cache.GetOrLoad("sf-key", loadFunc)
				results[idx] = v
				existsFlags[idx] = exists
				errs[idx] = err
			}(i)
		}
		wg.Wait()

		// Ensure each goroutine received the expected result.
		for i := 0; i < numGoroutines; i++ {
			require.NoError(t, errs[i])
			require.Equal(t, 999, results[i])
			require.False(t, existsFlags[i])
		}
		require.EqualValues(t, 1, callCount.Load())
		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 1, MissesTotal: numGoroutines}, metrics)

		// A later call should find the key in the cache.
		v, exists, err := cache.GetOrLoad("sf-key", loadFunc)
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, 999, v)
		require.EqualValues(t, 1, callCount.Load())
		assertPrometheusMetrics(t, expectedMetrics{EntriesAmount: 1, MissesTotal: numGoroutines, HitsTotal: 1}, metrics)
	})
}

func TestLRUCache_GetOrLoadWithTTL(t *testing.T) {
	// Define a custom TTL shorter than the default TTL.
	const customTTL = 100 * time.Millisecond

	// Set a default TTL that is longer than the custom TTL to ensure the custom TTL is used.
	cache, err := NewWithOpts[string, string](10, nil, Options{DefaultTTL: time.Second})
	require.NoError(t, err)

	v, exists, err := cache.GetOrLoadWithTTL("ttl-key", func(key string) (string, time.Duration, error) {
		return "ttl-value", customTTL, nil
	})
	require.NoError(t, err)
	require.False(t, exists)
	require.Equal(t, "ttl-value", v)

	// Immediately after loading, the value is in the cache.
	v2, ok := cache.Get("ttl-key")
	require.True(t, ok)
	require.Equal(t, "ttl-value", v2)

	// Wait longer than the custom TTL.
	time.Sleep(2 * customTTL)

	// The value should now be expired and thus not returned.
	_, ok = cache.Get("ttl-key")
	require.False(t, ok, "expected the value to be expired after the custom TTL")
}

type User struct {
	ID   string
	Name string
}
type Post struct {
	ID    string
	Title string
}

type expectedMetrics struct {
	EntriesAmount  int
	HitsTotal      int
	MissesTotal    int
	EvictionsTotal int
}

func assertPrometheusMetrics(t *testing.T, expected expectedMetrics, mc *PrometheusMetrics) {
	t.Helper()
	assert.Equal(t, expected.EntriesAmount, int(testutil.ToFloat64(mc.EntriesAmount.With(nil))))
	assert.Equal(t, expected.HitsTotal, int(testutil.ToFloat64(mc.HitsTotal.With(nil))))
	assert.Equal(t, expected.MissesTotal, int(testutil.ToFloat64(mc.MissesTotal.With(nil))))
	assert.Equal(t, expected.EvictionsTotal, int(testutil.ToFloat64(mc.EvictionsTotal.With(nil))))
}

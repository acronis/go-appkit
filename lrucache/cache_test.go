/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"crypto/sha256"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRUCache(t *testing.T) {
	users := map[[sha256.Size]byte]User{
		sha256.Sum256([]byte("user:1")):   {"Bob"},
		sha256.Sum256([]byte("user:42")):  {"John"},
		sha256.Sum256([]byte("user:777")): {"Ivan"},
	}
	posts := map[[sha256.Size]byte]Post{
		sha256.Sum256([]byte("post:101")): {"My first post."},
		sha256.Sum256([]byte("post:777")): {"My second post."},
	}

	fillCache := func(cache *LRUCache[[sha256.Size]byte]) {
		keys := [][sha256.Size]byte{
			sha256.Sum256([]byte("user:1")),
			sha256.Sum256([]byte("user:42")),
			sha256.Sum256([]byte("user:777"))}

		for _, key := range keys {
			cache.Add(key, users[key], entryTypeUser)
		}
		for _, key := range [][sha256.Size]byte{sha256.Sum256([]byte("post:101")), sha256.Sum256([]byte("post:777"))} {
			cache.Add(key, posts[key], entryTypePost)
		}
	}

	tests := []struct {
		name        string
		maxEntries  int
		fn          func(t *testing.T, cache *LRUCache[[sha256.Size]byte])
		wantMetrics testMetrics
	}{
		{
			name:       "attempt to get not existing keys",
			maxEntries: 100,
			fn: func(t *testing.T, cache *LRUCache[[sha256.Size]byte]) {
				for key := range users {
					_, found := cache.Get(key, entryTypeUser)
					require.False(t, found)
				}
				for key := range posts {
					_, found := cache.Get(key, entryTypePost)
					require.False(t, found)
				}
			},
			wantMetrics: testMetrics{
				Misses: testMetricsPair{len(users), len(posts)},
			},
		},
		{
			name:       "add entries and get them",
			maxEntries: 100,
			fn: func(t *testing.T, cache *LRUCache[[sha256.Size]byte]) {
				fillCache(cache)

				for key, wantUser := range users {
					val, found := cache.Get(key, entryTypeUser)
					require.True(t, found)
					require.Equal(t, wantUser, val.(User))
				}
				for key, wantPost := range posts {
					val, found := cache.Get(key, entryTypePost)
					require.True(t, found)
					require.Equal(t, wantPost, val.(Post))
				}
			},
			wantMetrics: testMetrics{
				Amount: testMetricsPair{len(users), len(posts)},
				Hits:   testMetricsPair{len(users), len(posts)},
			},
		},
		{
			name:       "add entries with evictions",
			maxEntries: len(users) + len(posts) - 1,
			fn: func(t *testing.T, cache *LRUCache[[sha256.Size]byte]) {
				fillCache(cache) // "user:1" key will be evicted.

				for key, wantUser := range users {
					if key == sha256.Sum256([]byte("user:1")) {
						_, found := cache.Get(key, entryTypeUser)
						require.False(t, found)
						continue
					}
					val, found := cache.Get(key, entryTypeUser)
					require.True(t, found)
					require.Equal(t, wantUser, val.(User))
				}
				for key, wantPost := range posts {
					val, found := cache.Get(key, entryTypePost)
					require.True(t, found)
					require.Equal(t, wantPost, val.(Post))
				}
			},
			wantMetrics: testMetrics{
				Amount:    testMetricsPair{len(users) - 1, len(posts)},
				Hits:      testMetricsPair{len(users) - 1, len(posts)},
				Misses:    testMetricsPair{1, 0},
				Evictions: testMetricsPair{1, 0},
			},
		},
		{
			name:       "remove entries",
			maxEntries: 100,
			fn: func(t *testing.T, cache *LRUCache[[sha256.Size]byte]) {
				fillCache(cache)

				require.False(t, cache.Remove(sha256.Sum256([]byte("user:100500")), entryTypeUser))
				require.False(t, cache.Remove(sha256.Sum256([]byte("user:42")), entryTypePost))
				require.True(t, cache.Remove(sha256.Sum256([]byte("user:42")), entryTypeUser))
				require.True(t, cache.Remove(sha256.Sum256([]byte("post:101")), entryTypePost))
			},
			wantMetrics: testMetrics{
				Amount: testMetricsPair{User: len(users) - 1, Post: len(posts) - 1},
			},
		},
		{
			name:       "resize, no evictions",
			maxEntries: 100,
			fn: func(t *testing.T, cache *LRUCache[[sha256.Size]byte]) {
				fillCache(cache)
				cache.Resize(50)
				for key := range users {
					_, found := cache.Get(key, entryTypeUser)
					require.True(t, found)
				}
				for key := range posts {
					_, found := cache.Get(key, entryTypePost)
					require.True(t, found)
				}
			},
			wantMetrics: testMetrics{
				Amount: testMetricsPair{len(users), len(posts)},
				Hits:   testMetricsPair{len(users), len(posts)},
			},
		},
		{
			name:       "resize with evictions",
			maxEntries: 100,
			fn: func(t *testing.T, cache *LRUCache[[sha256.Size]byte]) {
				fillCache(cache)
				_, found := cache.Get(sha256.Sum256([]byte("user:42")), entryTypeUser)
				require.True(t, found)
				_, found = cache.Get(sha256.Sum256([]byte("user:777")), entryTypeUser)
				require.True(t, found)
				_, found = cache.Get(sha256.Sum256([]byte("post:777")), entryTypePost)
				require.True(t, found)

				cache.Resize(2)

				_, found = cache.Get(sha256.Sum256([]byte("user:42")), entryTypeUser)
				require.False(t, found)
				_, found = cache.Get(sha256.Sum256([]byte("user:777")), entryTypeUser)
				require.True(t, found)
				_, found = cache.Get(sha256.Sum256([]byte("post:777")), entryTypePost)
				require.True(t, found)
			},
			wantMetrics: testMetrics{
				Amount:    testMetricsPair{1, 1},
				Hits:      testMetricsPair{3, 2},
				Misses:    testMetricsPair{1, 0},
				Evictions: testMetricsPair{2, 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, metricsCollector := makeCache(t, tt.maxEntries)
			tt.fn(t, cache)
			assertMetrics(t, tt.wantMetrics, metricsCollector)
		})
	}
}

const (
	entryTypeUser EntryType = iota
	entryTypePost
)
const (
	metricsLabelUser = "user"
	metricsLabelPost = "post"
)

type User struct {
	Name string
}
type Post struct {
	Title string
}

type testMetricsPair struct {
	User int
	Post int
}

type testMetrics struct {
	Amount     testMetricsPair
	Hits       testMetricsPair
	Misses     testMetricsPair
	Expiration testMetricsPair
	Evictions  testMetricsPair
}

func assertMetrics(t *testing.T, want testMetrics, mc *MetricsCollector) {
	t.Helper()
	assert.Equal(t, want.Amount.User, int(testutil.ToFloat64(mc.EntriesAmount.WithLabelValues(metricsLabelUser))))
	assert.Equal(t, want.Amount.Post, int(testutil.ToFloat64(mc.EntriesAmount.WithLabelValues(metricsLabelPost))))
	assert.Equal(t, want.Hits.User, int(testutil.ToFloat64(mc.HitsTotal.WithLabelValues(metricsLabelUser))))
	assert.Equal(t, want.Hits.Post, int(testutil.ToFloat64(mc.HitsTotal.WithLabelValues(metricsLabelPost))))
	assert.Equal(t, want.Misses.User, int(testutil.ToFloat64(mc.MissesTotal.WithLabelValues(metricsLabelUser))))
	assert.Equal(t, want.Misses.Post, int(testutil.ToFloat64(mc.MissesTotal.WithLabelValues(metricsLabelPost))))
	assert.Equal(t, want.Evictions.User, int(testutil.ToFloat64(mc.EvictionsTotal.WithLabelValues(metricsLabelUser))))
	assert.Equal(t, want.Evictions.Post, int(testutil.ToFloat64(mc.EvictionsTotal.WithLabelValues(metricsLabelPost))))
}

func makeCache(t *testing.T, maxEntries int) (*LRUCache[[sha256.Size]byte], *MetricsCollector) {
	t.Helper()
	mc := NewMetricsCollector("")
	mc.SetupEntryTypeLabels(map[EntryType]string{
		entryTypeUser: metricsLabelUser,
		entryTypePost: metricsLabelPost,
	})
	cache, err := New[[sha256.Size]byte](maxEntries, mc)
	require.NoError(t, err)
	return cache, mc
}

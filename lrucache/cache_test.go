/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"testing"

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

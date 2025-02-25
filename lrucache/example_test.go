/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/acronis/go-appkit/lrucache"
)

type User struct {
	UUID string
	Name string
}

type Post struct {
	UUID string
	Text string
}

func Example() {
	// Make and register Prometheus metrics collector.
	promMetrics := lrucache.NewPrometheusMetricsWithOpts(lrucache.PrometheusMetricsOpts{
		Namespace:         "my_app",                                  // Will be prepended to all metric names.
		ConstLabels:       prometheus.Labels{"app_version": "1.2.3"}, // Will be applied to all metrics.
		CurriedLabelNames: []string{"entry_type"},                    // For distinguishing between cached entities.
	})
	promMetrics.MustRegister()

	// LRU cache for users.
	const aliceUUID = "966971df-a592-4e7e-a309-52501016fa44"
	const bobUUID = "848adf28-84c1-4259-97a2-acba7cf5c0b6"
	usersCache, err := lrucache.New[string, User](100_000,
		promMetrics.MustCurryWith(prometheus.Labels{"entry_type": "user"}))
	if err != nil {
		log.Fatal(err)
	}
	usersCache.Add(aliceUUID, User{aliceUUID, "Alice"})
	usersCache.Add(bobUUID, User{bobUUID, "Bob"})
	if user, found := usersCache.Get(aliceUUID); found {
		fmt.Printf("User: %s, %s\n", user.UUID, user.Name)
	}
	if user, found := usersCache.Get(bobUUID); found {
		fmt.Printf("User: %s, %s\n", user.UUID, user.Name)
	}

	// LRU cache for posts. Posts are loaded from DB if not found in cache.
	const post1UUID = "823e50c7-984d-4de3-8a09-92fa21d3cc3b"
	const post2UUID = "24707009-ddf6-4e88-bd51-84ae236b7fda"
	postsCache, err := lrucache.NewWithOpts[string, Post](1_000,
		promMetrics.MustCurryWith(prometheus.Labels{"entry_type": "note"}), lrucache.Options{
			DefaultTTL: 5 * time.Minute, // Expired entries are removed during cleanup (see RunPeriodicCleanup method) or when accessed.
		})
	if err != nil {
		log.Fatal(err)
	}

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go postsCache.RunPeriodicCleanup(cleanupCtx, 10*time.Minute) // Run cleanup every 10 minutes.

	loadPostFromDatabase := func(id string) (value Post, err error) {
		// Emulate loading post from DB.
		if id == post1UUID {
			return Post{id, "Lorem ipsum dolor sit amet..."}, nil
		}
		return Post{}, fmt.Errorf("not found")
	}

	for _, postID := range []string{post1UUID, post1UUID, post2UUID} {
		// Get post from cache or load it from DB. If two goroutines try to load the same post concurrently,
		// only one of them will actually load the post, while the other will wait for the first one to finish.
		if post, exists, loadErr := postsCache.GetOrLoad(postID, loadPostFromDatabase); loadErr != nil {
			fmt.Printf("Failed to load post %s: %v\n", postID, loadErr)
		} else {
			if exists {
				fmt.Printf("Post: %s, %s\n", post.UUID, post.Text)
			} else {
				fmt.Printf("Post (loaded from db): %s, %s\n", post.UUID, post.Text)
			}
		}
	}

	// The following Prometheus metrics will be exposed:
	// my_app_cache_entries_amount{app_version="1.2.3",entry_type="note"} 1
	// my_app_cache_entries_amount{app_version="1.2.3",entry_type="user"} 2
	// my_app_cache_hits_total{app_version="1.2.3",entry_type="note"} 1
	// my_app_cache_hits_total{app_version="1.2.3",entry_type="user"} 2
	// my_app_cache_misses_total{app_version="1.2.3",entry_type="note"} 2

	fmt.Printf("Users: %d\n", usersCache.Len())
	fmt.Printf("Posts: %d\n", postsCache.Len())

	// Output:
	// User: 966971df-a592-4e7e-a309-52501016fa44, Alice
	// User: 848adf28-84c1-4259-97a2-acba7cf5c0b6, Bob
	// Post (loaded from db): 823e50c7-984d-4de3-8a09-92fa21d3cc3b, Lorem ipsum dolor sit amet...
	// Post: 823e50c7-984d-4de3-8a09-92fa21d3cc3b, Lorem ipsum dolor sit amet...
	// Failed to load post 24707009-ddf6-4e88-bd51-84ae236b7fda: not found
	// Users: 2
	// Posts: 1
}

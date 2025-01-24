# LRUCache

[![GoDoc Widget]][GoDoc]

The `lrucache` package provides an in-memory cache with an LRU (Least Recently Used) eviction policy and Prometheus metrics integration.

## Features

- **LRU Eviction Policy**: Automatically removes the least recently used items when the cache reaches its maximum size.
- **Prometheus Metrics**: Collects and exposes metrics to monitor cache usage and performance.
- **Expiration**: Supports setting TTL (Time To Live) for entries. Expired entries are removed during cleanup or when accessed.

## Usage

### Basic Example

```go
package lrucache_test

import (
	"fmt"
	"log"

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

	// LRU cache for posts.
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

	postsCache.Add(post1UUID, Post{post1UUID, "Lorem ipsum dolor sit amet..."})
	if post, found := postsCache.Get(post1UUID); found {
		fmt.Printf("Post: %s, %s\n", post.UUID, post.Text)
	}
	if _, found := postsCache.Get(post2UUID); !found {
		fmt.Printf("Post: %s is missing\n", post2UUID)
	}

	// The following Prometheus metrics will be exposed:
	// my_app_cache_entries_amount{app_version="1.2.3",entry_type="note"} 1
	// my_app_cache_entries_amount{app_version="1.2.3",entry_type="user"} 2
	// my_app_cache_hits_total{app_version="1.2.3",entry_type="note"} 1
	// my_app_cache_hits_total{app_version="1.2.3",entry_type="user"} 2
	// my_app_cache_misses_total{app_version="1.2.3",entry_type="note"} 1

	fmt.Printf("Users: %d\n", usersCache.Len())
	fmt.Printf("Posts: %d\n", postsCache.Len())

	// Output:
	// User: 966971df-a592-4e7e-a309-52501016fa44, Alice
	// User: 848adf28-84c1-4259-97a2-acba7cf5c0b6, Bob
	// Post: 823e50c7-984d-4de3-8a09-92fa21d3cc3b, Lorem ipsum dolor sit amet...
	// Post: 24707009-ddf6-4e88-bd51-84ae236b7fda is missing
	// Users: 2
	// Posts: 1
}
```

### Prometheus Metrics

Here is the full list of Prometheus metrics exposed by the `lrucache` package:

- `cache_entries_amount`: Total number of entries in the cache.
- `cache_hits_total`: Number of successfully found keys in the cache.
- `cache_misses_total`: Number of not found keys in the cache.
- `cache_evictions_total`: Number of evicted entries.

These metrics can be further customized with namespaces, constant labels, and curried labels as shown in the examples.

## License

Copyright © 2024 Acronis International GmbH.

Licensed under [MIT License](./../LICENSE).

[GoDoc]: https://pkg.go.dev/github.com/acronis/go-appkit/lrucache
[GoDoc Widget]: https://godoc.org/github.com/acronis/go-appkit?status.svg
/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"fmt"
	"log"
)

func Example() {
	type User struct {
		ID   int
		Name string
	}

	// Make and register Prometheus metrics collector.
	promMetrics := NewPrometheusMetricsWithOpts(PrometheusMetricsOpts{Namespace: "my_service_user"})
	promMetrics.MustRegister()

	// Make LRU cache for storing maximum 1000 entries
	cache, err := New[string, User](1000, promMetrics)
	if err != nil {
		log.Fatal(err)
	}

	// Add entries to cache.
	cache.Add("user:1", User{1, "Alice"})
	cache.Add("user:2", User{2, "Bob"})

	// Get entries from cache.
	if user, found := cache.Get("user:1"); found {
		fmt.Printf("%d, %s\n", user.ID, user.Name)
	}
	if user, found := cache.Get("user:2"); found {
		fmt.Printf("%d, %s\n", user.ID, user.Name)
	}

	// Output:
	// 1, Alice
	// 2, Bob
}

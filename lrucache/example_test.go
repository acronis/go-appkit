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
	const metricsNamespace = "myservice"

	// There are 2 types of entries will be stored in cache: users and posts.
	const (
		cacheEntryTypeUser EntryType = iota
		cacheEntryTypePost
	)

	type User struct {
		ID   int
		Name string
	}

	type Post struct {
		ID    int
		Title string
	}

	// Make, configure and register Prometheus metrics collector.
	metricsCollector := NewMetricsCollector(metricsNamespace)
	metricsCollector.SetupEntryTypeLabels(map[EntryType]string{
		cacheEntryTypeUser: "user",
		cacheEntryTypePost: "post",
	})
	metricsCollector.MustRegister()

	// Make LRU cache for storing maximum 1000 entries
	cache, err := New[string](1000, metricsCollector)
	if err != nil {
		log.Fatal(err)
	}

	// Add entries to cache.
	cache.Add("user:1", User{1, "John"}, cacheEntryTypeUser)
	cache.Add("post:1", Post{1, "My first post."}, cacheEntryTypePost)

	// Get entries from cache.
	if val, found := cache.Get("user:1", cacheEntryTypeUser); found {
		user := val.(User)
		fmt.Printf("%d, %s\n", user.ID, user.Name)
	}
	if val, found := cache.Get("post:1", cacheEntryTypePost); found {
		post := val.(Post)
		fmt.Printf("%d, %s\n", post.ID, post.Title)
	}

	// Output:
	// 1, John
	// 1, My first post.
}

/*
Copyright © 2025-2026 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/acronis/go-appkit/lrucache"
)

// SlidingWindowLimiter implements sliding window rate limiting algorithm.
type SlidingWindowLimiter struct {
	getCounter func(key string) *slidingWindowCounter
	maxRate    Rate
	now        func() time.Time
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter.
func NewSlidingWindowLimiter(maxRate Rate, maxKeys int) (*SlidingWindowLimiter, error) {
	if maxRate.Duration <= 0 {
		return nil, fmt.Errorf("rate duration must be positive, got %s", maxRate.Duration)
	}

	if maxKeys == 0 {
		counter := newSlidingWindowCounter(maxRate.Duration, int64(maxRate.Count))
		return &SlidingWindowLimiter{
			maxRate:    maxRate,
			getCounter: func(_ string) *slidingWindowCounter { return counter },
			now:        time.Now,
		}, nil
	}

	store, err := lrucache.New[string, *slidingWindowCounter](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return &SlidingWindowLimiter{
		maxRate: maxRate,
		getCounter: func(key string) *slidingWindowCounter {
			counter, _ := store.GetOrAdd(key, func() *slidingWindowCounter {
				return newSlidingWindowCounter(maxRate.Duration, int64(maxRate.Count))
			})
			return counter
		},
		now: time.Now,
	}, nil
}

// Allow checks if the request should be allowed based on the rate limit.
// If the request is not allowed, retryAfter is the time until the start of the next fixed window, it's always positive.
func (l *SlidingWindowLimiter) Allow(_ context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	counter := l.getCounter(key)
	now := l.now() // After the LRU lookup that may block; the same value is used for retryAfter.
	if counter.allow(now) {
		return true, 0, nil
	}
	retryAfter = now.Truncate(l.maxRate.Duration).Add(l.maxRate.Duration).Sub(now)
	return false, retryAfter, nil
}

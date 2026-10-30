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
			getCounter: func(_ string) *slidingWindowCounter { return counter },
			now:        time.Now,
		}, nil
	}

	store, err := lrucache.New[string, *slidingWindowCounter](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return &SlidingWindowLimiter{
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
// On rejection, retryAfter is the earliest retry delay without intervening admissions or clock jumps.
// It is positive and capped at the maximum time.Duration, also used for non-positive limits.
func (l *SlidingWindowLimiter) Allow(_ context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	counter := l.getCounter(key)
	allow, retryAfter = counter.allow(l.now()) // Sample time after the potentially blocking LRU lookup.
	return allow, retryAfter, nil
}

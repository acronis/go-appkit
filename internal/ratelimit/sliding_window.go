/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/RussellLuo/slidingwindow"

	"github.com/acronis/go-appkit/lrucache"
)

// SlidingWindowLimiter implements sliding window rate limiting algorithm.
type SlidingWindowLimiter struct {
	getLimiter func(key string) *slidingwindow.Limiter
	maxRate    Rate
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter.
func NewSlidingWindowLimiter(maxRate Rate, maxKeys int) (*SlidingWindowLimiter, error) {
	if maxKeys == 0 {
		lim, _ := slidingwindow.NewLimiter(
			maxRate.Duration, int64(maxRate.Count), func() (slidingwindow.Window, slidingwindow.StopFunc) {
				return slidingwindow.NewLocalWindow()
			})
		return &SlidingWindowLimiter{
			maxRate:    maxRate,
			getLimiter: func(_ string) *slidingwindow.Limiter { return lim },
		}, nil
	}

	store, err := lrucache.New[string, *slidingwindow.Limiter](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return &SlidingWindowLimiter{
		maxRate: maxRate,
		getLimiter: func(key string) *slidingwindow.Limiter {
			lim, _ := store.GetOrAdd(key, func() *slidingwindow.Limiter {
				lim, _ := slidingwindow.NewLimiter(
					maxRate.Duration, int64(maxRate.Count), func() (slidingwindow.Window, slidingwindow.StopFunc) {
						return slidingwindow.NewLocalWindow()
					})
				return lim
			})
			return lim
		},
	}, nil
}

// Allow checks if the request should be allowed based on the rate limit.
func (l *SlidingWindowLimiter) Allow(_ context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	if l.getLimiter(key).Allow() {
		return true, 0, nil
	}
	now := time.Now()
	retryAfter = now.Truncate(l.maxRate.Duration).Add(l.maxRate.Duration).Sub(now)
	return false, retryAfter, nil
}

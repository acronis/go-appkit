/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/memstore"
)

// LeakyBucketLimiter implements GCRA (Generic Cell Rate Algorithm). It's a leaky bucket variant algorithm.
// More details and good explanation of this alg is provided here: https://brandur.org/rate-limiting#gcra.
type LeakyBucketLimiter struct {
	limiter *throttled.GCRARateLimiterCtx
}

// NewLeakyBucketLimiter creates a new leaky bucket rate limiter.
func NewLeakyBucketLimiter(maxRate Rate, maxBurst, maxKeys int) (*LeakyBucketLimiter, error) {
	gcraStore, err := memstore.NewCtx(maxKeys)
	if err != nil {
		return nil, fmt.Errorf("new in-memory store: %w", err)
	}
	reqQuota := throttled.RateQuota{
		MaxRate:  throttled.PerDuration(maxRate.Count, maxRate.Duration),
		MaxBurst: maxBurst,
	}
	gcraLimiter, err := throttled.NewGCRARateLimiterCtx(gcraStore, reqQuota)
	if err != nil {
		return nil, fmt.Errorf("new GCRA rate limiter: %w", err)
	}
	return &LeakyBucketLimiter{gcraLimiter}, nil
}

// Allow checks if the request should be allowed based on the rate limit.
func (l *LeakyBucketLimiter) Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	limited, res, err := l.limiter.RateLimitCtx(ctx, key, 1)
	if err != nil {
		return false, 0, err
	}
	return !limited, res.RetryAfter, nil
}

/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// SlidingWindowLimiterTestSuite contains tests for SlidingWindowLimiter
type SlidingWindowLimiterTestSuite struct {
	suite.Suite
}

func TestSlidingWindowLimiter(t *testing.T) {
	suite.Run(t, new(SlidingWindowLimiterTestSuite))
}

func (ts *SlidingWindowLimiterTestSuite) TestAllowSequential() {
	limiter, err := NewSlidingWindowLimiter(Rate{Count: 2, Duration: time.Second}, 100)
	ts.NoError(err)

	ctx := context.Background()
	key := "test-key"

	// First request should be allowed
	allow, retryAfter, err := limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.True(allow)
	ts.Equal(time.Duration(0), retryAfter)

	// Second request should be allowed
	allow, retryAfter, err = limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.True(allow)
	ts.Equal(time.Duration(0), retryAfter)

	// Third request should be rate limited
	allow, retryAfter, err = limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.False(allow)
	ts.Greater(retryAfter, time.Duration(0))
}

func (ts *SlidingWindowLimiterTestSuite) TestRetryAfterCalculation() {
	limiter, err := NewSlidingWindowLimiter(Rate{Count: 1, Duration: time.Second}, 100)
	ts.NoError(err)

	ctx := context.Background()
	key := "test-key"

	// First request allowed
	allow, _, err := limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.True(allow)

	// Second request rate limited
	allow, retryAfter, err := limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.False(allow)
	ts.Greater(retryAfter, time.Duration(0))
	ts.LessOrEqual(retryAfter, time.Second)
}

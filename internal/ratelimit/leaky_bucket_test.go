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

// LeakyBucketLimiterTestSuite contains tests for LeakyBucketLimiter
type LeakyBucketLimiterTestSuite struct {
	suite.Suite
}

func TestLeakyBucketLimiter(t *testing.T) {
	suite.Run(t, new(LeakyBucketLimiterTestSuite))
}

func (ts *LeakyBucketLimiterTestSuite) TestAllowSequential() {
	limiter, err := NewLeakyBucketLimiter(Rate{Count: 2, Duration: time.Second}, 1, 100)
	ts.NoError(err)

	ctx := context.Background()
	key := "test-key"

	// First request should be allowed (burst capacity)
	allow, retryAfter, err := limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.True(allow)
	ts.GreaterOrEqual(retryAfter, time.Duration(-1)) // Can be -1ns for allowed requests

	// Second request should be allowed (burst capacity)
	allow, retryAfter, err = limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.True(allow)
	ts.GreaterOrEqual(retryAfter, time.Duration(-1)) // Can be -1ns for allowed requests

	// Third request should be rate limited
	allow, retryAfter, err = limiter.Allow(ctx, key)
	ts.NoError(err)
	ts.False(allow)
	ts.Greater(retryAfter, time.Duration(0))
}

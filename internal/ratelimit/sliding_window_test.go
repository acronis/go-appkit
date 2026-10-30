/*
Copyright © 2025-2026 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
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
	now := windowStart.Add(400 * time.Millisecond)
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 2, Duration: time.Second}, 100, &now)

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
	ts.Equal(600*time.Millisecond+time.Nanosecond, retryAfter)
}

func (ts *SlidingWindowLimiterTestSuite) TestRetryAfterCalculation() {
	now := windowStart.Add(300 * time.Millisecond)
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 1, Duration: time.Second}, 100, &now)

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
	ts.Equal(700*time.Millisecond+time.Nanosecond, retryAfter)
}

func newTestSlidingWindowLimiter(ts *SlidingWindowLimiterTestSuite, maxRate Rate, maxKeys int, now *time.Time) *SlidingWindowLimiter {
	limiter, err := NewSlidingWindowLimiter(maxRate, maxKeys)
	ts.Require().NoError(err)
	limiter.now = func() time.Time { return *now }
	return limiter
}

func (ts *SlidingWindowLimiterTestSuite) assertAllow(limiter *SlidingWindowLimiter, key string, wantAllow bool) {
	ts.T().Helper()
	allow, _, err := limiter.Allow(context.Background(), key)
	ts.Require().NoError(err)
	ts.Require().Equal(wantAllow, allow, "key=%q", key)
}

func (ts *SlidingWindowLimiterTestSuite) TestNewInvalidParams() {
	for _, d := range []time.Duration{0, -1, -time.Second} {
		_, err := NewSlidingWindowLimiter(Rate{Count: 1, Duration: d}, 0)
		ts.Error(err, "duration=%s", d)
		_, err = NewSlidingWindowLimiter(Rate{Count: 1, Duration: d}, 10)
		ts.Error(err, "duration=%s", d)
	}
	_, err := NewSlidingWindowLimiter(Rate{Count: 1, Duration: time.Second}, -1)
	ts.Error(err)
}

func (ts *SlidingWindowLimiterTestSuite) TestGlobalLimitIgnoresKey() {
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 2, Duration: time.Second}, 0, &now)
	ts.assertAllow(limiter, "a", true)
	ts.assertAllow(limiter, "b", true)
	ts.assertAllow(limiter, "c", false)
}

func (ts *SlidingWindowLimiterTestSuite) TestPerKeyLimit() {
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 1, Duration: time.Second}, 10, &now)
	ts.assertAllow(limiter, "a", true)
	ts.assertAllow(limiter, "a", false)
	ts.assertAllow(limiter, "b", true)
	ts.assertAllow(limiter, "b", false)
}

// Eviction from LRU drops the key state, so the evicted key gets a fresh limit. It's an existing behavior.
func (ts *SlidingWindowLimiterTestSuite) TestLRUEvictionResetsKeyState() {
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 1, Duration: time.Second}, 2, &now)
	ts.assertAllow(limiter, "a", true)
	ts.assertAllow(limiter, "b", true)
	ts.assertAllow(limiter, "a", false) // "a" becomes the most recently used.
	ts.assertAllow(limiter, "c", true)  // Evicts "b".
	ts.assertAllow(limiter, "a", false)
	ts.assertAllow(limiter, "b", true) // Fresh state, evicts "c".
	ts.assertAllow(limiter, "c", true)
}

func (ts *SlidingWindowLimiterTestSuite) TestRetryAfterAtWindowBoundary() {
	const size = time.Second
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 1, Duration: size}, 0, &now)
	ts.assertAllow(limiter, "", true)

	tests := []struct {
		now            time.Time
		wantRetryAfter time.Duration
	}{
		{windowStart, size + time.Nanosecond},
		{windowStart.Add(300 * time.Millisecond), 700*time.Millisecond + time.Nanosecond},
		{windowStart.Add(size - 1), 2},
		// At the boundary the previous weight is 1; one nanosecond later it falls below 1.
		{windowStart.Add(size), time.Nanosecond},
	}
	for _, tt := range tests {
		now = tt.now
		allow, retryAfter, err := limiter.Allow(context.Background(), "")
		ts.Require().NoError(err)
		ts.Require().False(allow, "now=%s", tt.now)
		ts.Require().Equal(tt.wantRetryAfter, retryAfter, "now=%s", tt.now)
	}
}

// The decision and retryAfter must use the same time snapshot.
// A second clock read crossing a window boundary must not change the calculated delay.
func (ts *SlidingWindowLimiterTestSuite) TestRetryAfterUsesSingleTimeSnapshot() {
	limiter, err := NewSlidingWindowLimiter(Rate{Count: 1, Duration: time.Second}, 0)
	ts.Require().NoError(err)
	ts.Require().True(allowAt(limiter.getCounter(""), windowStart.Add(time.Second-1)))

	calls := 0
	limiter.now = func() time.Time {
		calls++
		if calls == 1 {
			return windowStart.Add(time.Second - 1)
		}
		return windowStart.Add(time.Second)
	}
	allow, retryAfter, err := limiter.Allow(context.Background(), "")
	ts.Require().NoError(err)
	ts.Require().False(allow)
	ts.Require().Equal(2*time.Nanosecond, retryAfter)
	ts.Require().Equal(1, calls)
}

// Context is ignored: a canceled context doesn't produce an error and doesn't affect the decision.
func (ts *SlidingWindowLimiterTestSuite) TestContextIsIgnored() {
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 1, Duration: time.Second}, 0, &now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	allow, retryAfter, err := limiter.Allow(ctx, "")
	ts.Require().NoError(err)
	ts.Require().True(allow)
	ts.Require().Zero(retryAfter)

	allow, retryAfter, err = limiter.Allow(ctx, "")
	ts.Require().NoError(err)
	ts.Require().False(allow)
	ts.Require().Equal(time.Second+time.Nanosecond, retryAfter)
}

func (ts *SlidingWindowLimiterTestSuite) TestConcurrentPerKey() {
	const limit, keys, goroutinesPerKey, callsPerGoroutine = 50, 8, 10, 20
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: limit, Duration: time.Second}, keys, &now)

	allowed := make([]atomic.Int64, keys)
	var wg sync.WaitGroup
	for k := 0; k < keys; k++ {
		for g := 0; g < goroutinesPerKey; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				key := string(rune('a' + k))
				for i := 0; i < callsPerGoroutine; i++ {
					if allow, _, _ := limiter.Allow(context.Background(), key); allow {
						allowed[k].Add(1)
					}
				}
			}()
		}
	}
	wg.Wait()
	for k := range allowed {
		ts.Equal(int64(limit), allowed[k].Load(), "key #%d", k)
	}
}

// A retry must succeed at the advertised time, and fail one nanosecond earlier,
// assuming no intervening admissions or clock jumps.
func (ts *SlidingWindowLimiterTestSuite) TestRetryAfterEarliestAdmission() {
	for _, rate := range []Rate{{Count: 1, Duration: time.Minute}, {Count: 100, Duration: time.Minute}} {
		ts.Run(fmt.Sprintf("%d_per_minute", rate.Count), func() {
			now := windowStart
			limiter := newTestSlidingWindowLimiter(ts, rate, 0, &now)
			for i := 0; i < rate.Count; i++ {
				ts.assertAllow(limiter, "", true)
			}
			allow, wait, err := limiter.Allow(context.Background(), "")
			ts.Require().NoError(err)
			ts.Require().False(allow)
			ts.Require().Equal(time.Minute+time.Nanosecond, wait)
			now = now.Add(wait - time.Nanosecond)
			ts.assertAllow(limiter, "", false)
			now = now.Add(time.Nanosecond)
			ts.assertAllow(limiter, "", true)
		})
	}
}

func (ts *SlidingWindowLimiterTestSuite) TestRetryAfterWithinWindow() {
	now := windowStart
	limiter := newTestSlidingWindowLimiter(ts, Rate{Count: 100, Duration: time.Minute}, 0, &now)
	for i := 0; i < 100; i++ {
		ts.assertAllow(limiter, "", true)
	}
	now = windowStart.Add(time.Minute + 100*time.Millisecond)
	ts.assertAllow(limiter, "", true)
	allow, wait, err := limiter.Allow(context.Background(), "")
	ts.Require().NoError(err)
	ts.Require().False(allow)
	ts.Require().Equal(500*time.Millisecond+time.Nanosecond, wait)
	now = now.Add(wait - time.Nanosecond)
	ts.assertAllow(limiter, "", false)
	now = now.Add(time.Nanosecond)
	ts.assertAllow(limiter, "", true)
}

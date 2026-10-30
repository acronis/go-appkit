/*
Copyright © 2026 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Exhaustively compare small windows against a nanosecond-by-nanosecond search.
// Include rollback, fractional weights, full windows and transitions to the next window.
func TestSlidingWindowCounter_RetryMatchesExhaustiveSearch(t *testing.T) {
	for _, size := range []time.Duration{1, 2, 7, 11} {
		for _, limit := range []int64{1, 3, 5} {
			for prev := int64(0); prev <= limit; prev++ {
				for curr := int64(0); curr <= limit; curr++ {
					for elapsed := -2 * size; elapsed < size; elapsed++ {
						var wantWait time.Duration
						for ; wantWait <= 4*size; wantWait++ {
							e, p, n := elapsed+wantWait, prev, curr
							if e >= 2*size {
								p, n = 0, 0
							} else if e >= size {
								p, n = n, 0
								e -= size
							}
							if n+p*int64(size-e)/int64(size) < limit {
								break
							}
						}
						start := windowStart.Truncate(size)
						c := &slidingWindowCounter{size: size, limit: limit, currStart: start.UnixNano(), prevCount: prev, currCount: curr}
						now := start.Add(elapsed)
						allowed, wait := c.allow(now)
						if allowed != (wantWait == 0) || wait != wantWait {
							t.Fatalf("size=%s limit=%d prev=%d curr=%d elapsed=%s: got (%v,%s), want wait=%s",
								size, limit, prev, curr, elapsed, allowed, wait, wantWait)
						}
						if !allowed {
							before, _ := c.allow(now.Add(wait - time.Nanosecond))
							after, afterWait := c.allow(now.Add(wait))
							require.False(t, before)
							require.True(t, after)
							require.Zero(t, afterWait)
						}
					}
				}
			}
		}
	}
}

func TestSlidingWindowCounter_RetryLargeCounts(t *testing.T) {
	c := &slidingWindowCounter{
		size: time.Second, limit: math.MaxInt64, currStart: windowStart.UnixNano(),
		prevCount: math.MaxInt64, currCount: math.MaxInt64 - 1,
	}
	now := windowStart.Add(500 * time.Millisecond)
	allowed, wait := c.allow(now)
	require.False(t, allowed)
	require.Equal(t, 500*time.Millisecond, wait)
	allowed, _ = c.allow(now.Add(wait - time.Nanosecond))
	require.False(t, allowed)
	allowed, wait = c.allow(now.Add(wait))
	require.True(t, allowed)
	require.Zero(t, wait)
}

func TestSlidingWindowCounter_RetryDelaySaturates(t *testing.T) {
	for _, curr := range []int64{0, 1} {
		c := &slidingWindowCounter{
			size: time.Second, limit: 1, currStart: windowStart.UnixNano(), prevCount: 1, currCount: curr,
		}
		allowed, wait := c.allow(windowStart.Add(time.Duration(math.MinInt64)))
		require.False(t, allowed)
		require.Equal(t, time.Duration(math.MaxInt64), wait)
	}
	for _, limit := range []int64{0, -1, math.MinInt64} {
		c := newSlidingWindowCounter(time.Second, limit)
		allowed, wait := c.allow(windowStart)
		require.False(t, allowed)
		require.Equal(t, time.Duration(math.MaxInt64), wait)
	}
}

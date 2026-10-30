/*
Copyright © 2026 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// windowStart is aligned to a whole second, so it's the start of a fixed window for sizes that divide a second.
var windowStart = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func allowAt(c *slidingWindowCounter, now time.Time) bool {
	allowed, _ := c.allow(now)
	return allowed
}

func allowN(c *slidingWindowCounter, now time.Time, n int) int {
	allowed := 0
	for i := 0; i < n; i++ {
		if allowAt(c, now) {
			allowed++
		}
	}
	return allowed
}

func TestSlidingWindowCounter_LimitWithinOneWindow(t *testing.T) {
	c := newSlidingWindowCounter(time.Second, 3)
	require.Equal(t, 3, allowN(c, windowStart, 10))
	require.False(t, allowAt(c, windowStart.Add(999*time.Millisecond)))
}

func TestSlidingWindowCounter_NonPositiveLimit(t *testing.T) {
	for _, limit := range []int64{0, -1, math.MinInt64} {
		c := newSlidingWindowCounter(time.Second, limit)
		require.Zero(t, allowN(c, windowStart, 5), "limit=%d", limit)
		require.Zero(t, allowN(c, windowStart.Add(5*time.Second), 5), "limit=%d", limit)
	}
}

func TestSlidingWindowCounter_WindowBoundary(t *testing.T) {
	c := newSlidingWindowCounter(time.Second, 2)

	// Events at the last nanosecond belong to the first window.
	require.Equal(t, 2, allowN(c, windowStart.Add(time.Second-1), 2))

	// At the very start of the next window the previous one has weight 1, so floor(1*2)+0 = 2 >= limit.
	require.False(t, allowAt(c, windowStart.Add(time.Second)))

	// Weight becomes 0.5 in the middle: floor(0.5*2)+0 = 1, so one more event is allowed.
	require.Equal(t, 1, allowN(c, windowStart.Add(1500*time.Millisecond), 2))
}

func TestSlidingWindowCounter_WeightedCountIsTruncated(t *testing.T) {
	c := newSlidingWindowCounter(time.Second, 3)
	require.Equal(t, 3, allowN(c, windowStart, 3))
	// floor(0.5*3) = 1, so 2 more events fit into the limit.
	require.Equal(t, 2, allowN(c, windowStart.Add(1500*time.Millisecond), 5))
}

// The mathematical weight is exact: 50 * 580ms / 1s = 29, leaving 21 slots.
func TestSlidingWindowCounter_ExactWeight(t *testing.T) {
	c := newSlidingWindowCounter(time.Second, 50)
	require.Equal(t, 50, allowN(c, windowStart, 50))
	require.Equal(t, 21, allowN(c, windowStart.Add(1420*time.Millisecond), 30))
}

func TestSlidingWindowCounter_Idle(t *testing.T) {
	t.Run("adjacent window inherits count", func(t *testing.T) {
		c := newSlidingWindowCounter(time.Second, 4)
		require.Equal(t, 4, allowN(c, windowStart, 4))
		// floor(0.75*4) = 3.
		require.Equal(t, 1, allowN(c, windowStart.Add(1250*time.Millisecond), 4))
		// At the last nanosecond of the next window the weight is 1e-9, so only the current count matters.
		c = newSlidingWindowCounter(time.Second, 4)
		require.Equal(t, 4, allowN(c, windowStart, 4))
		require.Equal(t, 4, allowN(c, windowStart.Add(2*time.Second-1), 10))
	})

	t.Run("gap of two or more windows resets count", func(t *testing.T) {
		for _, gap := range []time.Duration{2 * time.Second, 3 * time.Second, 24 * time.Hour} {
			c := newSlidingWindowCounter(time.Second, 4)
			require.Equal(t, 4, allowN(c, windowStart.Add(999*time.Millisecond), 4))
			require.Equal(t, 4, allowN(c, windowStart.Add(gap), 10), "gap=%s", gap)
		}
	})

	t.Run("windows are aligned by time.Truncate, not by the first event", func(t *testing.T) {
		c := newSlidingWindowCounter(7*time.Second, 2)
		first := windowStart.Add(3 * time.Second)
		boundary := first.Truncate(7 * time.Second).Add(7 * time.Second)
		require.Equal(t, 2, allowN(c, first, 2))
		require.False(t, allowAt(c, boundary.Add(-1)))
		require.False(t, allowAt(c, boundary)) // Weight is 1 at the boundary.
		require.True(t, allowAt(c, boundary.Add(4*time.Second)))
	})
}

func TestSlidingWindowCounter_ClockGoesBackwards(t *testing.T) {
	c := newSlidingWindowCounter(time.Second, 4)
	require.Equal(t, 2, allowN(c, windowStart, 2))
	require.Equal(t, 2, allowN(c, windowStart.Add(time.Second), 4))

	// Windows don't move back, the previous window weight becomes > 1 (floor(1.5*2)+2 = 5), it's stricter.
	require.False(t, allowAt(c, windowStart.Add(500*time.Millisecond)))

	// State is kept when the clock returns: floor(0.5*2)+2 = 3.
	require.Equal(t, 1, allowN(c, windowStart.Add(1500*time.Millisecond), 4))
}

func TestSlidingWindowCounter_NumericExtremes(t *testing.T) {
	t.Run("current count at max int64", func(t *testing.T) {
		c := newSlidingWindowCounter(time.Second, math.MaxInt64)
		require.True(t, allowAt(c, windowStart))
		c.currCount = math.MaxInt64 - 1
		require.True(t, allowAt(c, windowStart))
		require.False(t, allowAt(c, windowStart)) // currCount+1 would overflow.
		require.Equal(t, int64(math.MaxInt64), c.currCount)
	})

	t.Run("weighted previous count at max int64", func(t *testing.T) {
		c := newSlidingWindowCounter(time.Second, math.MaxInt64)
		require.True(t, allowAt(c, windowStart))
		c.currCount = math.MaxInt64
		// At the boundary the full previous count still consumes the entire limit.
		require.False(t, allowAt(c, windowStart.Add(time.Second)))
		require.Equal(t, int64(math.MaxInt64), c.prevCount)
		require.Zero(t, c.currCount)
	})

	t.Run("huge weight after clock goes backwards", func(t *testing.T) {
		c := newSlidingWindowCounter(time.Nanosecond, math.MaxInt64)
		require.True(t, allowAt(c, windowStart))
		require.True(t, allowAt(c, windowStart.Add(1))) // prevCount = 1.
		c.prevCount = 1 << 40
		// Weight is ~3.6e12, weighted previous count is ~4e24.
		require.False(t, allowAt(c, windowStart.Add(-time.Hour)))
	})
}

func TestSlidingWindowCounter_Concurrent(t *testing.T) {
	const limit, goroutines, callsPerGoroutine = 1000, 50, 100
	c := newSlidingWindowCounter(time.Second, limit)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed.Add(int64(allowN(c, windowStart, callsPerGoroutine)))
		}()
	}
	wg.Wait()
	require.Equal(t, int64(limit), allowed.Load())
}

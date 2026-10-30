/*
Copyright © 2026 Acronis International GmbH.

Released under MIT license.
*/

// The sliding window counter algorithm is derived from github.com/RussellLuo/slidingwindow
// that is distributed under the following license:
//
// Copyright (c) 2020 Luo Peng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package ratelimit

import (
	"math"
	"math/bits"
	"sync"
	"time"
)

// slidingWindowCounter estimates the count in a sliding window by two adjacent fixed windows:
// floor(prevCount*(size-elapsed)/size) + currCount. Windows are aligned by time.Time.Truncate (wall clock).
// This is an approximation, not a strict limit over every rolling interval of length size.
type slidingWindowCounter struct {
	size  time.Duration
	limit int64

	mu        sync.Mutex
	currStart int64 // Unix nanoseconds
	currCount int64
	prevCount int64
}

// newSlidingWindowCounter creates a new counter. size must be positive.
func newSlidingWindowCounter(size time.Duration, limit int64) *slidingWindowCounter {
	return &slidingWindowCounter{size: size, limit: limit}
}

// allow counts an event if permitted, otherwise returns the earliest retry delay assuming
// no intervening admissions or clock jumps. Delays exceeding time.Duration, and limits
// that never permit an event, return math.MaxInt64.
func (c *slidingWindowCounter) allow(now time.Time) (bool, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.advance(now)
	if c.limit <= 0 {
		return false, time.Duration(math.MaxInt64)
	}
	if c.currCount == c.limit {
		// At the next boundary the full current window becomes the previous window.
		// Its weight first drops below 1 one nanosecond after that boundary.
		return false, time.Unix(0, c.currStart).Add(c.size).Add(time.Nanosecond).Sub(now)
	}

	elapsed := now.Sub(time.Unix(0, c.currStart))
	// #nosec G115 -- size > 0, prevCount >= 0 and admissions keep 0 <= currCount < limit here.
	size, prev, headroom := uint64(c.size), uint64(c.prevCount), uint64(c.limit-c.currCount)
	// Unsigned subtraction also handles negative elapsed after a clock rollback:
	// size-elapsed can exceed MaxInt64, but always fits in uint64.
	// #nosec G115 -- converting negative elapsed uses intentional modulo-2^64 arithmetic.
	remaining := size - uint64(elapsed)
	// floor(prev*remaining/size) < headroom iff prev*remaining < headroom*size.
	// Compare 128-bit products to avoid both floating-point rounding and overflow.
	prevHi, prevLo := bits.Mul64(prev, remaining)
	limitHi, limitLo := bits.Mul64(headroom, size)
	if prevHi < limitHi || (prevHi == limitHi && prevLo < limitLo) {
		c.currCount++
		return true, 0
	}

	// Find the largest remaining duration for which prev*remaining < headroom*size:
	// ceil(headroom*size/prev)-1. Rejection implies prev > 0 and quotient <= remaining,
	// so Div64 cannot overflow. This also handles admission exactly at the next boundary.
	maxRemaining, remainder := bits.Div64(limitHi, limitLo, prev)
	if remainder == 0 {
		maxRemaining--
	}
	delay := remaining - maxRemaining
	if delay > math.MaxInt64 {
		return false, time.Duration(math.MaxInt64)
	}
	return false, time.Duration(delay)
}

func (c *slidingWindowCounter) advance(now time.Time) {
	newCurrStart := now.Truncate(c.size)
	diffSize := newCurrStart.Sub(time.Unix(0, c.currStart)) / c.size
	if diffSize < 1 {
		return
	}
	c.prevCount = 0
	if diffSize == 1 {
		c.prevCount = c.currCount
	}
	c.currStart = newCurrStart.UnixNano()
	c.currCount = 0
}

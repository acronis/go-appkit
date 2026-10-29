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
	"sync"
	"time"
)

// maxInt64Float is 2^63; converting a float64 >= 2^63 to int64 is implementation-defined in Go.
const maxInt64Float = float64(math.MaxInt64)

// slidingWindowCounter estimates the count in a sliding window by two adjacent fixed windows:
// floor(prevCount*(size-elapsed)/size) + currCount. Windows are aligned by time.Time.Truncate (wall clock).
// The weight is computed in float64 exactly as in the upstream implementation; exact integer arithmetic
// would round differently in some cases (e.g. 0.58*50 gives 28 in float64, not 29).
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

// allow reports whether one more event may happen at now and counts it if so.
func (c *slidingWindowCounter) allow(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.advance(now)

	elapsed := now.Sub(time.Unix(0, c.currStart))
	weight := float64(c.size-elapsed) / float64(c.size)
	weightedPrev := weight * float64(c.prevCount)

	// Same as upstream "int64(weightedPrev)+currCount+1 > limit", but without overflows.
	// currCount <= max(limit, 0), so "limit-currCount" cannot overflow.
	if weightedPrev >= maxInt64Float || int64(weightedPrev) >= c.limit-c.currCount {
		return false
	}

	c.currCount++
	return true
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

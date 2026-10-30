/*
Copyright © 2026 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"math"
	"strconv"
	"time"
)

// FormatRetryAfter formats a delay as seconds rounded toward positive infinity,
// capped at the largest whole-second delay representable by time.Duration.
// Integer division avoids losing subsecond precision for large delays, and int64
// formatting is safe even when the seconds do not fit in a platform-sized int.
func FormatRetryAfter(delay time.Duration) string {
	seconds := int64(delay / time.Second)
	if delay%time.Second > 0 {
		seconds++
	}
	// Ceil(MaxInt64 nanoseconds) would overflow clients converting seconds back to time.Duration.
	seconds = min(seconds, math.MaxInt64/int64(time.Second))
	return strconv.FormatInt(seconds, 10)
}

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

func TestFormatRetryAfter(t *testing.T) {
	for _, tt := range []struct {
		delay time.Duration
		want  string
	}{
		{0, "0"}, {time.Nanosecond, "1"}, {time.Second, "1"}, {time.Second + time.Nanosecond, "2"},
		{time.Duration(math.MaxInt64), "9223372036"},
		{9223372036 * time.Second, "9223372036"},
		{9223372036*time.Second + time.Nanosecond, "9223372036"},
		{2147483648*time.Second + time.Nanosecond, "2147483649"},
		// Preserve the previous rounding semantics for custom callbacks returning a negative delay.
		{-time.Nanosecond, "0"}, {-time.Second, "-1"}, {-time.Second - time.Nanosecond, "-1"},
		{time.Duration(math.MinInt64), "-9223372036"},
	} {
		t.Run(tt.delay.String(), func(t *testing.T) { require.Equal(t, tt.want, FormatRetryAfter(tt.delay)) })
	}
}

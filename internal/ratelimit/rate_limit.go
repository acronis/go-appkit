/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"time"
)

// Rate describes the frequency of requests.
type Rate struct {
	Count    int
	Duration time.Duration
}

// Limiter interface defines the rate limiting contract.
type Limiter interface {
	Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error)
}

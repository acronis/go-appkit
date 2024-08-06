/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// Default parameter values for RateLimitingRoundTripper.
const (
	DefaultRateLimitingBurst       = 1
	DefaultRateLimitingWaitTimeout = 15 * time.Second
)

// RateLimitingRoundTripperAdaptation represents a params to adapt rate limiting in accordance with value in response.
type RateLimitingRoundTripperAdaptation struct {
	ResponseHeaderName string
	SlackPercent       int
}

// RateLimitingRoundTripperOpts represents an options for RateLimitingRoundTripper.
type RateLimitingRoundTripperOpts struct {
	Burst       int
	WaitTimeout time.Duration
	Adaptation  RateLimitingRoundTripperAdaptation
}

// RateLimitingRoundTripper wraps implementing http.RoundTripper interface object
// and provides adaptive (can use limit from response's HTTP header) rate limiting mechanism for outgoing requests.
type RateLimitingRoundTripper struct {
	Delegate http.RoundTripper

	rateLimiter *rate.Limiter

	RateLimit   int
	Burst       int
	WaitTimeout time.Duration
	Adaptation  RateLimitingRoundTripperAdaptation
}

// NewRateLimitingRoundTripper creates a new RateLimitingRoundTripper with specified rate limit.
func NewRateLimitingRoundTripper(delegate http.RoundTripper, rateLimit int) (*RateLimitingRoundTripper, error) {
	return NewRateLimitingRoundTripperWithOpts(delegate, rateLimit, RateLimitingRoundTripperOpts{})
}

// NewRateLimitingRoundTripperWithOpts creates a new RateLimitingRoundTripper with specified rate limit and options.
// For options that are not presented, the default values will be used.
func NewRateLimitingRoundTripperWithOpts(
	delegate http.RoundTripper, rateLimit int, opts RateLimitingRoundTripperOpts,
) (*RateLimitingRoundTripper, error) {
	if rateLimit <= 0 {
		return nil, fmt.Errorf("rate limit must be positive")
	}

	if opts.Burst < 0 {
		return nil, fmt.Errorf("burst must be positive")
	}
	if opts.Burst == 0 {
		opts.Burst = DefaultRateLimitingBurst
	}

	if opts.WaitTimeout == 0 {
		opts.WaitTimeout = DefaultRateLimitingWaitTimeout
	}

	if opts.Adaptation.SlackPercent < 0 || opts.Adaptation.SlackPercent > 100 {
		return nil, fmt.Errorf("slack percent must be in range [0..100]")
	}

	return &RateLimitingRoundTripper{
		Delegate:    delegate,
		rateLimiter: rate.NewLimiter(rate.Limit(rateLimit), opts.Burst),
		RateLimit:   rateLimit,
		Burst:       opts.Burst,
		WaitTimeout: opts.WaitTimeout,
		Adaptation:  opts.Adaptation,
	}, nil
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (rt *RateLimitingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		defer func() {
			_ = r.Body.Close() // Per RoundTripper contract.
		}()
	}

	ctx, cancel := context.WithTimeout(r.Context(), rt.WaitTimeout)
	defer cancel()

	if err := rt.rateLimiter.Wait(ctx); err != nil {
		if !errors.Is(ctx.Err(), context.Canceled) {
			return nil, &RateLimitingWaitError{Inner: err}
		}
	}

	resp, err := rt.Delegate.RoundTrip(r)
	if err != nil {
		return resp, err
	}

	if rt.Adaptation.ResponseHeaderName != "" {
		rt.updateRateLimitIfNeeded(rt.getRateLimitFromResponse(resp))
	}

	return resp, nil
}

func (rt *RateLimitingRoundTripper) getRateLimitFromResponse(resp *http.Response) int {
	respLimitStr := resp.Header.Get(rt.Adaptation.ResponseHeaderName)
	if respLimitStr == "" {
		return 0
	}

	respLimit, err := strconv.Atoi(respLimitStr)
	if err != nil || respLimit < 0 {
		return 0
	}

	respLimit = (respLimit * (100 - rt.Adaptation.SlackPercent)) / 100
	if respLimit == 0 {
		return 1 // Send 1 request per second instead of stopping at all.
	}
	return respLimit
}

func (rt *RateLimitingRoundTripper) updateRateLimitIfNeeded(newRateLimit int) {
	// If rate limit was changed and in last HTTP response we didn't receive rate limiting header (newRateLimit is 0),
	// it would be better to restore default value.
	if newRateLimit == 0 || newRateLimit > rt.RateLimit {
		newRateLimit = rt.RateLimit
	}

	if rt.rateLimiter.Limit() != rate.Limit(newRateLimit) {
		rt.rateLimiter.SetLimit(rate.Limit(newRateLimit))
	}
}

// RateLimitingWaitError is returned in RoundTrip method of RateLimitingRoundTripper when rate limit is exceeded.
type RateLimitingWaitError struct {
	Inner error
}

func (e *RateLimitingWaitError) Error() string {
	return fmt.Sprintf("wait due to client side rate limiting: %s", e.Inner.Error())
}

// Unwrap returns the next error in the error chain.
func (e *RateLimitingWaitError) Unwrap() error {
	return e.Inner
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"
)

// IsRetryable defines a func that can tell if error is retryable as opposed to persistent.
type IsRetryable func(error) bool

// RetryableFunc is function that does some work and can be potentially retried.
type RetryableFunc func(ctx context.Context) error

// Policy defines backoff strategy.
type Policy interface {
	NewBackOff() backoff.BackOff
}

// DoWithRetry executes fn with retry according to policy p and with respect to context ctx.
// IsRetryable defines which errors lead to retry attempt (can be nil for any error).
// Notify can be used to receive notification on every retry with error and backoff delay
// (can be nil if no notifications required).
func DoWithRetry(ctx context.Context, p Policy, isRetryable IsRetryable, notify backoff.Notify, fn RetryableFunc) error {
	b := p.NewBackOff()
	bctx := backoff.WithContext(b, ctx)
	var op backoff.Operation = func() error {
		err := fn(bctx.Context())
		if err != nil &&
			(isRetryable != nil && !isRetryable(err)) {
			return backoff.Permanent(err)
		}
		return err
	}
	return backoff.RetryNotify(op, bctx, notify)
}

// The PolicyFunc type is an adapter to allow the use of ordinary functions as retry.Policy.
type PolicyFunc func() backoff.BackOff

// NewBackOff implements retry.Policy.
func (f PolicyFunc) NewBackOff() backoff.BackOff {
	return f()
}

// ExponentialBackoffPolicy means repeat up to max times with exponentially growing delays (1.5 multiplier).
type ExponentialBackoffPolicy struct {
	initialInterval time.Duration
	maxAttempts     int
}

// NewExponentialBackoffPolicy returns an exponential backoff policy with given initial interval and max retry attempt count.
func NewExponentialBackoffPolicy(initialInterval time.Duration, maxRetryAttempts int) ExponentialBackoffPolicy {
	return ExponentialBackoffPolicy{initialInterval, maxRetryAttempts}
}

// NewBackOff implements retry.Policy.
func (p ExponentialBackoffPolicy) NewBackOff() backoff.BackOff {
	eb := backoff.NewExponentialBackOff()
	eb.InitialInterval = p.initialInterval
	var bf backoff.BackOff = eb
	if p.maxAttempts > 0 {
		bf = backoff.WithMaxRetries(eb, uint64(p.maxAttempts))
	}
	bf.Reset()
	return bf
}

// ConstantBackoffPolicy means repeat up to max times with constant interval delays.
type ConstantBackoffPolicy struct {
	interval    time.Duration
	maxAttempts int
}

// NewConstantBackoffPolicy returns a constant backoff policy with given interval and max retry attempt count.
func NewConstantBackoffPolicy(interval time.Duration, maxRetryAttempts int) ConstantBackoffPolicy {
	return ConstantBackoffPolicy{interval, maxRetryAttempts}
}

// NewBackOff implements retry.Policy.
func (p ConstantBackoffPolicy) NewBackOff() backoff.BackOff {
	var bf backoff.BackOff = backoff.NewConstantBackOff(p.interval)
	if p.maxAttempts > 0 {
		bf = backoff.WithMaxRetries(bf, uint64(p.maxAttempts))
	}
	bf.Reset()
	return bf
}

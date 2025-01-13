/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/retry"
)

// Default parameter values for RetryableRoundTripper.
const (
	DefaultMaxRetryAttempts                  = 10
	DefaultExponentialBackoffInitialInterval = time.Second
	DefaultExponentialBackoffMultiplier      = 2
)

// UnlimitedRetryAttempts should be used as RetryableRoundTripperOpts.MaxRetryAttempts value
// when we want to stop retries only by RetryableRoundTripperOpts.BackoffPolicy.
const UnlimitedRetryAttempts = -1

// RetryAttemptNumberHeader is an HTTP header name that will contain the serial number of the retry attempt.
const RetryAttemptNumberHeader = "X-Retry-Attempt"

// CheckRetryFunc is a function that is called right after RoundTrip() method
// and determines if the next retry attempt is needed.
type CheckRetryFunc func(ctx context.Context, resp *http.Response, roundTripErr error, doneRetryAttempts int) (bool, error)

// RetryableRoundTripper wraps an object that implements http.RoundTripper interface
// and provides a retrying mechanism for HTTP requests.
type RetryableRoundTripper struct {
	// Delegate is an object that implements http.RoundTripper interface
	// and is used for sending HTTP requests under the hood.
	Delegate http.RoundTripper

	// Logger is used for logging.
	// When it's necessary to use context-specific logger, LoggerProvider should be used instead.
	Logger log.FieldLogger

	// LoggerProvider is a function that provides a context-specific logger.
	// One of the typical use cases is to use a retryable client in the context of a request handler,
	// where the logger should produce logs with request-specific information (e.g., request ID).
	LoggerProvider func(ctx context.Context) log.FieldLogger

	// MaxRetryAttempts determines how many maximum retry attempts can be done.
	// The total number of sending HTTP request may be MaxRetryAttempts + 1 (the first request is not a retry attempt).
	// If its value is UnlimitedRetryAttempts, it's supposed that retry mechanism will be stopped by BackoffPolicy.
	// By default, DefaultMaxRetryAttempts const is used.
	MaxRetryAttempts int

	// CheckRetry is called right after RoundTrip() method and determines if the next retry attempt is needed.
	// By default, DefaultCheckRetry function is used.
	CheckRetry CheckRetryFunc

	// IgnoreRetryAfter determines if Retry-After HTTP header of the response is parsed and
	// used as a wait time before doing the next retry attempt.
	// If it's true or response doesn't contain Retry-After HTTP header, BackoffPolicy will be used for computing delay.
	IgnoreRetryAfter bool

	// BackoffPolicy is used for computing wait time before doing the next retry attempt
	// when the given response doesn't contain Retry-After HTTP header or IgnoreRetryAfter is true.
	// By default, DefaultBackoffPolicy is used.
	BackoffPolicy retry.Policy
}

// RetryableRoundTripperOpts represents an options for RetryableRoundTripper.
type RetryableRoundTripperOpts struct {
	// Logger is used for logging.
	// When it's necessary to use context-specific logger, LoggerProvider should be used instead.
	Logger log.FieldLogger

	// LoggerProvider is a function that provides a context-specific logger.
	// One of the typical use cases is to use a retryable client in the context of a request handler,
	// where the logger should produce logs with request-specific information (e.g., request ID).
	LoggerProvider func(ctx context.Context) log.FieldLogger

	// MaxRetryAttempts determines how many maximum retry attempts can be done.
	// The total number of sending HTTP request may be MaxRetryAttempts + 1 (the first request is not a retry attempt).
	// If its value is UnlimitedRetryAttempts, it's supposed that retry mechanism will be stopped by BackoffPolicy.
	// By default, DefaultMaxRetryAttempts const is used.
	MaxRetryAttempts int

	// CheckRetry is called right after RoundTrip() method and determines if the next retry attempt is needed.
	// By default, DefaultCheckRetry function is used.
	CheckRetryFunc CheckRetryFunc

	// IgnoreRetryAfter determines if Retry-After HTTP header of the response is parsed and
	// used as a wait time before doing the next retry attempt.
	// If it's true or response doesn't contain Retry-After HTTP header, BackoffPolicy will be used for computing delay.
	IgnoreRetryAfter bool

	// BackoffPolicy is used for computing wait time before doing the next retry attempt
	// when the given response doesn't contain Retry-After HTTP header or IgnoreRetryAfter is true.
	// By default, DefaultBackoffPolicy is used.
	BackoffPolicy retry.Policy
}

// NewRetryableRoundTripper returns a new instance of RetryableRoundTripper.
func NewRetryableRoundTripper(delegate http.RoundTripper) (*RetryableRoundTripper, error) {
	return NewRetryableRoundTripperWithOpts(delegate, RetryableRoundTripperOpts{})
}

// NewRetryableRoundTripperWithOpts creates a new instance of RateLimitingRoundTripper with specified options.
func NewRetryableRoundTripperWithOpts(
	delegate http.RoundTripper, opts RetryableRoundTripperOpts,
) (*RetryableRoundTripper, error) {
	if opts.MaxRetryAttempts < 0 && opts.MaxRetryAttempts != UnlimitedRetryAttempts {
		return nil, fmt.Errorf("incorrect max retry attempts")
	}
	if opts.MaxRetryAttempts == 0 {
		opts.MaxRetryAttempts = DefaultMaxRetryAttempts
	}

	if opts.Logger == nil {
		opts.Logger = log.NewDisabledLogger()
	}

	if opts.CheckRetryFunc == nil {
		opts.CheckRetryFunc = DefaultCheckRetry
	}

	if opts.BackoffPolicy == nil {
		opts.BackoffPolicy = DefaultBackoffPolicy
	}

	return &RetryableRoundTripper{
		Delegate:         delegate,
		Logger:           opts.Logger,
		LoggerProvider:   opts.LoggerProvider,
		MaxRetryAttempts: opts.MaxRetryAttempts,
		CheckRetry:       opts.CheckRetryFunc,
		BackoffPolicy:    opts.BackoffPolicy,
		IgnoreRetryAfter: opts.IgnoreRetryAfter,
	}, nil
}

// RoundTrip performs request with retry logic.
// nolint: gocyclo
func (rt *RetryableRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rewindReqBody := func(r *http.Request) error { return nil }
	if req.Body != nil {
		originalReqBody := req.Body
		defer func() {
			_ = originalReqBody.Close() // Per RoundTripper contract.
		}()

		var err error
		rewindReqBody, err = makeRequestBodyRewindable(req)
		if err != nil {
			return nil, &RetryableRoundTripperError{Inner: err}
		}
	}

	getNextWaitTime := rt.makeNextWaitTimeProvider()
	reqCtx := req.Context()
	reqCloned := false

	var resp *http.Response
	var roundTripErr error
	for curRetryAttemptNum := 0; ; curRetryAttemptNum++ {
		// Rewind request body.
		if rewindErr := rewindReqBody(req); rewindErr != nil {
			if curRetryAttemptNum == 0 {
				return nil, &RetryableRoundTripperError{Inner: rewindErr}
			}
			rt.logger(reqCtx).Error(fmt.Sprintf(
				"failed to rewind request body between retry attempts, %d request(s) done", curRetryAttemptNum+1),
				log.Error(rewindErr))
			return resp, roundTripErr
		}

		// Discard and close response body before next retry.
		if resp != nil && roundTripErr == nil {
			rt.drainResponseBody(reqCtx, resp)
		}

		if curRetryAttemptNum > 0 {
			if !reqCloned {
				req, reqCloned = req.Clone(req.Context()), true // Per RoundTripper contract.
			}
			req.Header.Set(RetryAttemptNumberHeader, strconv.Itoa(curRetryAttemptNum))
		}

		// Perform request attempt.
		resp, roundTripErr = rt.Delegate.RoundTrip(req)

		// Check if another retry attempt should be done
		needRetry, checkRetryErr := rt.CheckRetry(reqCtx, resp, roundTripErr, curRetryAttemptNum)
		if checkRetryErr != nil {
			rt.logger(reqCtx).Error(fmt.Sprintf(
				"failed to check if retry is needed, %d request(s) done", curRetryAttemptNum+1),
				log.Error(checkRetryErr))
			return resp, roundTripErr
		}
		if !needRetry {
			return resp, roundTripErr
		}

		// Check should we stop (max attempts exceeded or by backoff policy).
		if rt.MaxRetryAttempts > 0 && curRetryAttemptNum >= rt.MaxRetryAttempts {
			rt.logger(reqCtx).Warnf("max retry attempts exceeded (%d), %d request(s) done",
				rt.MaxRetryAttempts, curRetryAttemptNum+1)
			return resp, roundTripErr
		}
		waitTime, stop := getNextWaitTime(resp)
		if stop {
			return resp, roundTripErr
		}

		select {
		case <-reqCtx.Done():
			rt.logger(reqCtx).Warnf("context canceled (%v) while waiting for the next retry attempt, %d request(s) done",
				reqCtx.Err(), curRetryAttemptNum+1)
			return resp, roundTripErr
		case <-time.After(waitTime):
		}
	}
}

type waitTimeProvider func(resp *http.Response) (waitTime time.Duration, stop bool)

func (rt *RetryableRoundTripper) makeNextWaitTimeProvider() waitTimeProvider {
	bf := rt.BackoffPolicy.NewBackOff()
	return func(resp *http.Response) (waitTime time.Duration, stop bool) {
		if resp != nil && !rt.IgnoreRetryAfter {
			if retryAfter, ok := parseRetryAfterFromResponse(resp); ok {
				return retryAfter, false
			}
		}
		waitTime = bf.NextBackOff()
		return waitTime, waitTime == backoff.Stop
	}
}

func (rt *RetryableRoundTripper) drainResponseBody(ctx context.Context, resp *http.Response) {
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			rt.logger(ctx).Error("failed to close previous response body between retry attempts", log.Error(closeErr))
		}
	}()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		rt.logger(ctx).Error("failed to discard previous response body between retry attempts", log.Error(err))
	}
}

func (rt *RetryableRoundTripper) logger(ctx context.Context) log.FieldLogger {
	if rt.LoggerProvider != nil {
		return rt.LoggerProvider(ctx)
	}
	return rt.Logger
}

// RetryableRoundTripperError is returned in RoundTrip method of RetryableRoundTripper
// when the original request cannot be potentially retried.
type RetryableRoundTripperError struct {
	Inner error
}

func (e *RetryableRoundTripperError) Error() string {
	return fmt.Sprintf("retryable round trip: %s", e.Inner.Error())
}

// Unwrap returns the next error in the error chain.
func (e *RetryableRoundTripperError) Unwrap() error {
	return e.Inner
}

// DefaultCheckRetry represents default function to determine either retry is needed or not.
func DefaultCheckRetry(
	ctx context.Context, resp *http.Response, roundTripErr error, doneRetryAttempts int,
) (needRetry bool, err error) {
	if roundTripErr != nil {
		return CheckErrorIsTemporary(roundTripErr), nil
	}
	if resp == nil {
		return false, fmt.Errorf("both response and round trip error are nil")
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError, nil
}

// DefaultBackoffPolicy is a default backoff policy.
var DefaultBackoffPolicy = retry.PolicyFunc(func() backoff.BackOff {
	bf := backoff.NewExponentialBackOff()
	bf.InitialInterval = DefaultExponentialBackoffInitialInterval
	bf.Multiplier = DefaultExponentialBackoffMultiplier
	bf.Reset()
	return bf
})

// CheckErrorIsTemporary checks either error is temporary or not.
func CheckErrorIsTemporary(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	var terr interface{ Temporary() bool }
	ok := errors.As(err, &terr)
	return ok && terr.Temporary()
}

func makeRequestBodyRewindable(req *http.Request) (func(*http.Request) error, error) {
	if reqBodySeeker, ok := req.Body.(io.ReadSeeker); ok {
		reqBodySeekOffset, err := reqBodySeeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("seek request body before doing first request: %w", err)
		}
		req.Body = io.NopCloser(req.Body)
		return func(r *http.Request) (err error) {
			_, err = reqBodySeeker.Seek(reqBodySeekOffset, io.SeekStart)
			if err != nil {
				return fmt.Errorf(
					"seek request body (offset=%d, whence=%d): %w", reqBodySeekOffset, io.SeekStart, err)
			}
			return nil
		}, nil
	}

	bufferedReqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read all requesty body before doing first request: %w", err)
	}
	return func(r *http.Request) error {
		r.Body = io.NopCloser(bytes.NewReader(bufferedReqBody))
		return nil
	}, nil
}

func parseRetryAfterFromResponse(resp *http.Response) (retryAfter time.Duration, ok bool) {
	retryAfterVal := resp.Header.Get("Retry-After")
	if retryAfterVal == "" {
		return 0, false
	}

	parsedInt, parseIntErr := strconv.Atoi(retryAfterVal)
	if parseIntErr != nil {
		parsedTime, parsedTimeErr := time.Parse(time.RFC1123, retryAfterVal)
		if parsedTimeErr != nil {
			return 0, false
		}
		return time.Until(parsedTime), true
	}
	if parsedInt < 0 {
		return 0, false
	}
	return time.Duration(parsedInt) * time.Second, true
}

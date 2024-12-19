/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"fmt"
	"net/http"
)

// CloneHTTPRequest creates a shallow copy of the request along with a deep copy of the Headers.
func CloneHTTPRequest(req *http.Request) *http.Request {
	r := new(http.Request)
	*r = *req
	r.Header = CloneHTTPHeader(req.Header)
	return r
}

// CloneHTTPHeader creates a deep copy of an http.Header.
func CloneHTTPHeader(in http.Header) http.Header {
	out := make(http.Header, len(in))
	for key, values := range in {
		newValues := make([]string, len(values))
		copy(newValues, values)
		out[key] = newValues
	}
	return out
}

// MustHTTPClient wraps delegate transports with logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func MustHTTPClient(cfg *Config, userAgent, reqType string, delegate http.RoundTripper) *http.Client {
	if delegate == nil {
		delegate = http.DefaultTransport
	}

	delegate = NewLoggingRoundTripper(delegate, reqType)
	delegate = NewMetricsRoundTripper(delegate, reqType)

	delegate, err := NewRateLimitingRoundTripperWithOpts(
		delegate, cfg.RateLimits.Limit, cfg.RateLimits.TransportOpts(),
	)
	if err != nil {
		panic(fmt.Errorf("create rate limiting round tripper: %w", err))
	}

	delegate, err = NewRetryableRoundTripperWithOpts(delegate, cfg.Retries.TransportOpts())
	if err != nil {
		panic(fmt.Errorf("create retryable round tripper: %w", err))
	}

	delegate = NewUserAgentRoundTripper(delegate, userAgent)
	delegate = NewRequestIDRoundTripper(delegate)
	return &http.Client{Transport: delegate, Timeout: cfg.Timeout}
}

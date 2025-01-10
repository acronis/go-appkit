/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"github.com/acronis/go-appkit/log"
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

// New wraps delegate transports with logging, rate limiting, retryable, request id
// and returns an error if any occurs.
func New(cfg *Config) (*http.Client, error) {
	return NewWithOpts(cfg, Opts{})
}

// Must wraps delegate transports with logging, rate limiting, retryable, request id
// and panics if any error occurs.
func Must(cfg *Config) *http.Client {
	client, err := New(cfg)
	if err != nil {
		panic(err)
	}

	return client
}

// Opts provides options for NewWithOpts and MustWithOpts functions.
type Opts struct {
	// UserAgent is a user agent string.
	UserAgent string

	// RequestType is a type of request. e.g. service 'auth-service', an action 'login' or specific information to correlate.
	RequestType string

	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// LoggerProvider is a function that provides a context-specific logger.
	LoggerProvider func(ctx context.Context) log.FieldLogger

	// RequestIDProvider is a function that provides a request ID.
	RequestIDProvider func(ctx context.Context) string

	// Collector is a metrics collector.
	Collector MetricsCollector
}

// NewWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and returns an error if any occurs.
func NewWithOpts(cfg *Config, opts Opts) (*http.Client, error) {
	var err error
	delegate := opts.Delegate

	if delegate == nil {
		delegate = http.DefaultTransport.(*http.Transport).Clone()
	}

	if cfg.Log.Enabled {
		logOpts := cfg.Log.TransportOpts()
		logOpts.LoggerProvider = opts.LoggerProvider
		logOpts.RequestType = opts.RequestType
		delegate = NewLoggingRoundTripperWithOpts(delegate, logOpts)
	}

	if cfg.Metrics.Enabled {
		delegate = NewMetricsRoundTripperWithOpts(delegate, MetricsRoundTripperOpts{
			RequestType: opts.RequestType,
			Collector:   opts.Collector,
		})
	}

	if cfg.RateLimits.Enabled {
		delegate, err = NewRateLimitingRoundTripperWithOpts(
			delegate, cfg.RateLimits.Limit, cfg.RateLimits.TransportOpts(),
		)
		if err != nil {
			return nil, fmt.Errorf("create rate limiting round tripper: %w", err)
		}
	}

	if opts.UserAgent != "" {
		delegate = NewUserAgentRoundTripper(delegate, opts.UserAgent)
	}

	delegate = NewRequestIDRoundTripperWithOpts(delegate, RequestIDRoundTripperOpts{
		RequestIDProvider: opts.RequestIDProvider,
	})

	if cfg.Retries.Enabled {
		retryOpts := cfg.Retries.TransportOpts()
		retryOpts.LoggerProvider = opts.LoggerProvider
		retryOpts.BackoffPolicy = cfg.Retries.GetPolicy()
		delegate, err = NewRetryableRoundTripperWithOpts(delegate, retryOpts)
		if err != nil {
			return nil, fmt.Errorf("create retryable round tripper: %w", err)
		}
	}

	return &http.Client{Transport: delegate, Timeout: cfg.Timeout}, nil
}

// MustWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func MustWithOpts(cfg *Config, opts Opts) *http.Client {
	client, err := NewWithOpts(cfg, opts)
	if err != nil {
		panic(err)
	}

	return client
}

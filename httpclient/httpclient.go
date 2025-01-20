/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/acronis/go-appkit/log"
)

// New wraps delegate transports with logging, rate limiting, retryable, request id
// and returns an error if any occurs.
func New(cfg *Config) (*http.Client, error) {
	return NewWithOpts(cfg, Opts{})
}

// MustNew wraps delegate transports with logging, rate limiting, retryable, request id
// and panics if any error occurs.
func MustNew(cfg *Config) *http.Client {
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

	// ClientType represents a type of client, it's a service component reference. e.g. 'auth-service'
	ClientType string

	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// LoggerProvider is a function that provides a context-specific logger.
	LoggerProvider func(ctx context.Context) log.FieldLogger

	// RequestIDProvider is a function that provides a request ID.
	RequestIDProvider func(ctx context.Context) string

	// MetricsCollector is a metrics collector.
	MetricsCollector MetricsCollector

	// ClassifyRequest does request classification, producing non-parameterized summary for given request.
	ClassifyRequest func(r *http.Request, clientType string) string
}

// NewWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and returns an error if any occurs.
func NewWithOpts(cfg *Config, opts Opts) (*http.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must be provided")
	}

	var err error
	delegate := opts.Delegate

	if delegate == nil {
		delegate = http.DefaultTransport.(*http.Transport).Clone()
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

	if cfg.Metrics.Enabled {
		delegate = NewMetricsRoundTripperWithOpts(delegate, opts.MetricsCollector, MetricsRoundTripperOpts{
			ClientType:      opts.ClientType,
			ClassifyRequest: opts.ClassifyRequest,
		})
	}

	if cfg.Log.Enabled {
		logOpts := cfg.Log.TransportOpts()
		logOpts.LoggerProvider = opts.LoggerProvider
		logOpts.ClientType = opts.ClientType
		delegate = NewLoggingRoundTripperWithOpts(delegate, logOpts)
	}

	return &http.Client{Transport: delegate, Timeout: cfg.Timeout}, nil
}

// MustNewWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func MustNewWithOpts(cfg *Config, opts Opts) *http.Client {
	client, err := NewWithOpts(cfg, opts)
	if err != nil {
		panic(err)
	}

	return client
}

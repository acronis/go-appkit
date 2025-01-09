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

// ClientProviders for further customization of the client logging and request id.
type ClientProviders struct {
	// Logger is a function that provides a context-specific logger.
	Logger func(ctx context.Context) log.FieldLogger

	// RequestID is a function that provides a request ID.
	RequestID func(ctx context.Context) string
}

// New wraps delegate transports with logging, metrics, rate limiting, retryable, user agent, request id
// and returns an error if any occurs.
func New(
	cfg *Config,
	userAgent string,
	reqType string,
	delegate http.RoundTripper,
	providers ClientProviders,
	collector MetricsCollector,
) (*http.Client, error) {
	var err error

	if delegate == nil {
		delegate = http.DefaultTransport.(*http.Transport).Clone()
	}

	if cfg.Log.Enabled {
		opts := cfg.Log.TransportOpts()
		opts.LoggerProvider = providers.Logger
		opts.ReqType = reqType
		delegate = NewLoggingRoundTripperWithOpts(delegate, opts)
	}

	if cfg.Metrics.Enabled {
		delegate = NewMetricsRoundTripperWithOpts(delegate, MetricsRoundTripperOpts{
			ReqType:   reqType,
			Collector: collector,
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

	if userAgent != "" {
		delegate = NewUserAgentRoundTripper(delegate, userAgent)
	}

	delegate = NewRequestIDRoundTripperWithOpts(delegate, RequestIDRoundTripperOpts{
		RequestIDProvider: providers.RequestID,
	})

	if cfg.Retries.Enabled {
		opts := cfg.Retries.TransportOpts()
		opts.LoggerProvider = providers.Logger
		opts.BackoffPolicy = cfg.Retries.GetPolicy()
		delegate, err = NewRetryableRoundTripperWithOpts(delegate, opts)
		if err != nil {
			return nil, fmt.Errorf("create retryable round tripper: %w", err)
		}
	}

	return &http.Client{Transport: delegate, Timeout: cfg.Timeout}, nil
}

// Must wraps delegate transports with logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func Must(
	cfg *Config,
	userAgent string,
	reqType string,
	delegate http.RoundTripper,
	providers ClientProviders,
	collector MetricsCollector,
) *http.Client {
	client, err := New(cfg, userAgent, reqType, delegate, providers, collector)
	if err != nil {
		panic(err)
	}

	return client
}

// Opts provides options for NewWithOpts and MustWithOpts functions.
type Opts struct {
	// UserAgent is a user agent string.
	UserAgent string

	// ReqType is a type of request. e.g. service 'auth-service', an action 'login' or specific information to correlate.
	ReqType string

	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// Providers are the functions that provide a context-specific logger and request ID.
	Providers ClientProviders

	// Collector is a metrics collector.
	Collector MetricsCollector
}

// NewWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and returns an error if any occurs.
func NewWithOpts(cfg *Config, opts Opts) (*http.Client, error) {
	return New(cfg, opts.UserAgent, opts.ReqType, opts.Delegate, opts.Providers, opts.Collector)
}

// MustWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func MustWithOpts(cfg *Config, opts Opts) *http.Client {
	return Must(cfg, opts.UserAgent, opts.ReqType, opts.Delegate, opts.Providers, opts.Collector)
}

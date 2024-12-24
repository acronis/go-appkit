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
	Logger    func(ctx context.Context) log.FieldLogger
	RequestID func(ctx context.Context) string
}

// NewHTTPClient wraps delegate transports with logging, metrics, rate limiting, retryable, user agent, request id
// and returns an error if any occurs.
func NewHTTPClient(
	cfg *Config,
	userAgent string,
	reqType string,
	delegate http.RoundTripper,
	providers ClientProviders,
) (*http.Client, error) {
	var err error

	if delegate == nil {
		delegate = http.DefaultTransport.(*http.Transport).Clone()
	}

	if cfg.Logger.Enabled {
		opts := cfg.Logger.TransportOpts()
		opts.LoggerProvider = providers.Logger
		delegate = NewLoggingRoundTripperWithOpts(delegate, reqType, opts)
	}

	if cfg.Metrics.Enabled {
		delegate = NewMetricsRoundTripper(delegate, reqType)
	}

	if cfg.RateLimits.Enabled {
		delegate, err = NewRateLimitingRoundTripperWithOpts(
			delegate, cfg.RateLimits.Limit, cfg.RateLimits.TransportOpts(),
		)
		if err != nil {
			return nil, fmt.Errorf("create rate limiting round tripper: %w", err)
		}
	}

	if cfg.Retries.Enabled {
		opts := cfg.Retries.TransportOpts()
		opts.LoggerProvider = providers.Logger
		delegate, err = NewRetryableRoundTripperWithOpts(delegate, opts)
		if err != nil {
			return nil, fmt.Errorf("create retryable round tripper: %w", err)
		}
	}

	delegate = NewUserAgentRoundTripper(delegate, userAgent)
	delegate = NewRequestIDRoundTripperWithOpts(delegate, RequestIDRoundTripperOpts{
		RequestIDProvider: providers.RequestID,
	})

	return &http.Client{Transport: delegate, Timeout: cfg.Timeout}, nil
}

// MustHTTPClient wraps delegate transports with logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func MustHTTPClient(
	cfg *Config,
	userAgent,
	reqType string,
	delegate http.RoundTripper,
	providers ClientProviders,
) *http.Client {
	client, err := NewHTTPClient(cfg, userAgent, reqType, delegate, providers)
	if err != nil {
		panic(err)
	}

	return client
}

// ClientOpts provides options for NewHTTPClientWithOpts and MustHTTPClientWithOpts functions.
type ClientOpts struct {
	Config    Config
	UserAgent string
	ReqType   string
	Delegate  http.RoundTripper
	Providers ClientProviders
}

// NewHTTPClientWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and returns an error if any occurs.
func NewHTTPClientWithOpts(opts ClientOpts) (*http.Client, error) {
	return NewHTTPClient(&opts.Config, opts.UserAgent, opts.ReqType, opts.Delegate, opts.Providers)
}

// MustHTTPClientWithOpts wraps delegate transports with options
// logging, metrics, rate limiting, retryable, user agent, request id
// and panics if any error occurs.
func MustHTTPClientWithOpts(opts ClientOpts) *http.Client {
	return MustHTTPClient(&opts.Config, opts.UserAgent, opts.ReqType, opts.Delegate, opts.Providers)
}

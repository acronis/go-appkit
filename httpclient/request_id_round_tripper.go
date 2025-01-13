/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"github.com/rs/xid"
	"net/http"

	"github.com/acronis/go-appkit/httpserver/middleware"
)

// RequestIDRoundTripper for X-Request-ID header to the request.
type RequestIDRoundTripper struct {
	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// Opts are the options for the request ID round tripper.
	Opts RequestIDRoundTripperOpts
}

// RequestIDRoundTripperOpts for X-Request-ID header to the request options.
type RequestIDRoundTripperOpts struct {
	// RequestIDProvider is a function that provides a request ID.
	// middleware.GetRequestIDFromContext is used by default.
	RequestIDProvider func(ctx context.Context) string
}

// NewRequestIDRoundTripper creates an HTTP transport with X-Request-ID header support.
func NewRequestIDRoundTripper(delegate http.RoundTripper) http.RoundTripper {
	return &RequestIDRoundTripper{
		Delegate: delegate,
	}
}

// NewRequestIDRoundTripperWithOpts creates an HTTP transport with X-Request-ID header support with options.
func NewRequestIDRoundTripperWithOpts(delegate http.RoundTripper, opts RequestIDRoundTripperOpts) http.RoundTripper {
	return &RequestIDRoundTripper{
		Delegate: delegate,
		Opts:     opts,
	}
}

// getRequestIDProvider returns a function with the request ID provider.
func (rt *RequestIDRoundTripper) getRequestIDProvider() func(ctx context.Context) string {
	if rt.Opts.RequestIDProvider != nil {
		return rt.Opts.RequestIDProvider
	}

	return middleware.GetRequestIDFromContext
}

// RoundTrip adds X-Request-ID header to the request.
func (rt *RequestIDRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Header.Get("X-Request-ID") != "" {
		return rt.Delegate.RoundTrip(r)
	}

	requestID := rt.getRequestIDProvider()(r.Context())
	if requestID == "" {
		requestID = xid.New().String()
	}

	r = r.Clone(r.Context())
	r.Header.Set("X-Request-ID", requestID)
	return rt.Delegate.RoundTrip(r)
}

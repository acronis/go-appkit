/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"github.com/acronis/go-appkit/httpserver/middleware"
	"net/http"
)

// RequestIDRoundTripper for X-Request-ID header to the request.
type RequestIDRoundTripper struct {
	Delegate http.RoundTripper
}

// NewRequestIDRoundTripper creates an HTTP transport with X-Request-ID header support.
func NewRequestIDRoundTripper(delegate http.RoundTripper) http.RoundTripper {
	return &RequestIDRoundTripper{
		Delegate: delegate,
	}
}

// RoundTrip adds X-Request-ID header to the request.
func (rt *RequestIDRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	requestID := middleware.GetRequestIDFromContext(r.Context())
	if r.Header.Get("X-Request-ID") != "" || requestID == "" {
		return rt.Delegate.RoundTrip(r)
	}

	r = CloneHTTPRequest(r)
	r.Header.Set("X-Request-ID", requestID)
	return rt.Delegate.RoundTrip(r)
}

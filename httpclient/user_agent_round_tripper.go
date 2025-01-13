/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import "net/http"

// UserAgentRoundTripper implements http.RoundTripper interface
// and sets User-Agent HTTP header in all outgoing requests.
type UserAgentRoundTripper struct {
	Delegate       http.RoundTripper
	UserAgent      string
	UpdateStrategy UserAgentUpdateStrategy
}

// UserAgentUpdateStrategy represents a strategy for updating User-Agent HTTP header.
type UserAgentUpdateStrategy int

// User-Agent update strategies.
const (
	UserAgentUpdateStrategySetIfEmpty UserAgentUpdateStrategy = iota
	UserAgentUpdateStrategyAppend
	UserAgentUpdateStrategyPrepend
)

// UserAgentRoundTripperOpts represents an options for UserAgentRoundTripper.
type UserAgentRoundTripperOpts struct {
	UpdateStrategy UserAgentUpdateStrategy
}

// NewUserAgentRoundTripper creates a new UserAgentRoundTripper.
func NewUserAgentRoundTripper(delegate http.RoundTripper, userAgent string) *UserAgentRoundTripper {
	return NewUserAgentRoundTripperWithOpts(delegate, userAgent, UserAgentRoundTripperOpts{})
}

// NewUserAgentRoundTripperWithOpts creates a new UserAgentRoundTripper with specified options.
func NewUserAgentRoundTripperWithOpts(
	delegate http.RoundTripper, userAgent string, opts UserAgentRoundTripperOpts,
) *UserAgentRoundTripper {
	return &UserAgentRoundTripper{delegate, userAgent, opts.UpdateStrategy}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (rt *UserAgentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	userAgent := req.Header.Get("User-Agent")

	switch rt.UpdateStrategy {
	case UserAgentUpdateStrategySetIfEmpty:
		if userAgent != "" {
			return rt.Delegate.RoundTrip(req)
		}
		userAgent = rt.UserAgent

	case UserAgentUpdateStrategyAppend:
		if userAgent == "" {
			userAgent = rt.UserAgent
		} else {
			userAgent += " " + rt.UserAgent
		}

	case UserAgentUpdateStrategyPrepend:
		if userAgent == "" {
			userAgent = rt.UserAgent
		} else {
			userAgent = rt.UserAgent + " " + userAgent
		}
	}

	req = req.Clone(req.Context()) // Per RoundTripper contract.
	req.Header.Set("User-Agent", userAgent)
	return rt.Delegate.RoundTrip(req)
}

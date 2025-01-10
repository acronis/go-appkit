/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"net/http"
)

// AuthBearerRoundTripperError is returned in RoundTrip method of AuthBearerRoundTripper
// when the original request cannot be potentially retried.
type AuthBearerRoundTripperError struct {
	Inner error
}

func (e *AuthBearerRoundTripperError) Error() string {
	return fmt.Sprintf("auth bearer round trip: %s", e.Inner.Error())
}

// Unwrap returns the next error in the error chain.
func (e *AuthBearerRoundTripperError) Unwrap() error {
	return e.Inner
}

// AuthProvider provide auth information that used for barear authrization
type AuthProvider interface {
	GetToken(ctx context.Context, scope ...string) (string, error)
}

// AuthBearerRoundTripperOpts is options for AuthBearerRoundTripper.
type AuthBearerRoundTripperOpts struct {
	TokenScope []string
}

// AuthBearerRoundTripper implements http.RoundTripper interface
// and sets Authorization HTTP header in all outgoing requests.
type AuthBearerRoundTripper struct {
	Delegate     http.RoundTripper
	AuthProvider AuthProvider
	opts         AuthBearerRoundTripperOpts
}

// NewAuthBearerRoundTripper creates a new AuthBearerRoundTripper.
func NewAuthBearerRoundTripper(delegate http.RoundTripper, authProvider AuthProvider) *AuthBearerRoundTripper {
	return NewAuthBearerRoundTripperWithOpts(delegate, authProvider, AuthBearerRoundTripperOpts{})
}

// NewAuthBearerRoundTripperWithOpts creates a new AuthBearerRoundTripper with options.
func NewAuthBearerRoundTripperWithOpts(delegate http.RoundTripper, authProvider AuthProvider,
	opts AuthBearerRoundTripperOpts) *AuthBearerRoundTripper {
	return &AuthBearerRoundTripper{delegate, authProvider, opts}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
func (rt *AuthBearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer func() {
			_ = req.Body.Close() // Per RoundTripper contract.
		}()
	}
	if req.Header.Get("Authorization") != "" {
		return rt.Delegate.RoundTrip(req)
	}
	token, err := rt.AuthProvider.GetToken(req.Context(), rt.opts.TokenScope...)
	if err != nil {
		return nil, &AuthBearerRoundTripperError{Inner: err}
	}
	req = req.Clone(req.Context()) // Per RoundTripper contract.
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	return rt.Delegate.RoundTrip(req)
}

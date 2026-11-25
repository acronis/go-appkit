/*
Copyright © 2026 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"net/http"

	"github.com/acronis/go-appkit/log"
)

// InvalidatableAuthProvider provides auth information used for bearer authorization
// together with a function that invalidates only the returned token.
type InvalidatableAuthProvider interface {
	// GetTokenWithInvalidator returns a token for the scope and a function that invalidates it.
	// The invalidate function removes only the cache entry of the returned token, so other tokens of
	// the provider (e.g., tokens with other scopes) stay valid. It returns true if the entry was removed.
	// The provider decides whether the removal happens (e.g., it may throttle invalidations of young tokens).
	// The invalidate function must be idempotent and safe for concurrent use.
	// It must not remove a cache entry that holds another token. For example, requests A and B get a 401
	// for the same token T; A invalidates T and gets a new token T2; the invalidator of B must then keep T2
	// and return false. It may be nil if the token cannot be invalidated.
	GetTokenWithInvalidator(ctx context.Context, scope ...string) (token string, invalidate func() bool, err error)
}

// InvalidatableAuthProviderFunc allows to define get token logic with an invalidator in functional way.
type InvalidatableAuthProviderFunc func(ctx context.Context, scope ...string) (string, func() bool, error)

// GetTokenWithInvalidator implements InvalidatableAuthProvider interface.
func (f InvalidatableAuthProviderFunc) GetTokenWithInvalidator(
	ctx context.Context, scope ...string,
) (token string, invalidate func() bool, err error) {
	return f(ctx, scope...)
}

// InvalidatableAuthBearerRoundTripperOpts is options for InvalidatableAuthBearerRoundTripper.
// Unlike AuthBearerRoundTripperOpts, it has no MinInvalidationInterval:
// the InvalidatableAuthProvider throttles the invalidation of its tokens.
type InvalidatableAuthBearerRoundTripperOpts struct {
	// TokenScope is an optional list of scopes to pass to InvalidatableAuthProvider.GetTokenWithInvalidator.
	// By default, no scopes are passed.
	TokenScope []string

	// ShouldRefreshTokenAndRetry is called after the first attempt and decides whether a token refresh attempt
	// should be performed and the request should be retried once more (token invalidation + token refresh + single retry).
	// By default, it returns true only for HTTP 401 responses.
	ShouldRefreshTokenAndRetry func(ctx context.Context, resp *http.Response) bool

	// LoggerProvider is a function that provides a context-specific logger.
	LoggerProvider func(ctx context.Context) log.FieldLogger
}

// InvalidatableAuthBearerRoundTripper implements http.RoundTripper interface
// and sets the "Authorization: Bearer <token>" HTTP header in outgoing requests
// using the provided InvalidatableAuthProvider.
// It behaves as AuthBearerRoundTripper, with one difference: on an authentication failure (by default HTTP 401),
// it invalidates only the token that failed (using the invalidator returned together with this token)
// instead of the whole provider cache. Then it obtains a new token and retries the request once if the token changed.
// Use it when one provider holds many tokens (e.g., one token per tenant scope),
// so a failure of one token does not cause the reissue of all the others.
type InvalidatableAuthBearerRoundTripper struct {
	Delegate     http.RoundTripper
	AuthProvider InvalidatableAuthProvider
	opts         InvalidatableAuthBearerRoundTripperOpts
}

// NewInvalidatableAuthBearerRoundTripper creates a new InvalidatableAuthBearerRoundTripper.
func NewInvalidatableAuthBearerRoundTripper(
	delegate http.RoundTripper, authProvider InvalidatableAuthProvider,
) *InvalidatableAuthBearerRoundTripper {
	return NewInvalidatableAuthBearerRoundTripperWithOpts(delegate, authProvider, InvalidatableAuthBearerRoundTripperOpts{})
}

// NewInvalidatableAuthBearerRoundTripperWithOpts creates a new InvalidatableAuthBearerRoundTripper with options.
func NewInvalidatableAuthBearerRoundTripperWithOpts(
	delegate http.RoundTripper, authProvider InvalidatableAuthProvider, opts InvalidatableAuthBearerRoundTripperOpts,
) *InvalidatableAuthBearerRoundTripper {
	if opts.ShouldRefreshTokenAndRetry == nil {
		opts.ShouldRefreshTokenAndRetry = func(ctx context.Context, resp *http.Response) bool {
			return resp != nil && resp.StatusCode == http.StatusUnauthorized
		}
	}
	return &InvalidatableAuthBearerRoundTripper{
		Delegate:     delegate,
		AuthProvider: authProvider,
		opts:         opts,
	}
}

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
// It behaves as AuthBearerRoundTripper.RoundTrip, but on a refresh it calls the invalidator
// of the failed token instead of invalidating the whole provider cache.
func (rt *InvalidatableAuthBearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return roundTripWithBearer(req, rt.Delegate, rt.AuthProvider.GetTokenWithInvalidator, nil, bearerRetryOpts{
		tokenScope:                 rt.opts.TokenScope,
		shouldRefreshTokenAndRetry: rt.opts.ShouldRefreshTokenAndRetry,
		loggerProvider:             rt.opts.LoggerProvider,
		invalidatedLogMsg:          "auth token invalidated after 401 response",
	})
}

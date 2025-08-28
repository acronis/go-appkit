/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/acronis/go-appkit/log"
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

// AuthProvider provides auth information used for bearer authorization.
type AuthProvider interface {
	GetToken(ctx context.Context, scope ...string) (string, error)
}

// AuthProviderInvalidator is implemented by AuthProviders that support cache invalidation.
type AuthProviderInvalidator interface {
	Invalidate()
}

// AuthProviderFunc allows to define get token logic in functional way.
type AuthProviderFunc func(ctx context.Context, scope ...string) (string, error)

func (f AuthProviderFunc) GetToken(ctx context.Context, scope ...string) (string, error) {
	return f(ctx, scope...)
}

// DefaultAuthProviderMinInvalidationInterval is the default minimum time between cache invalidation attempts for the same AuthProvider.
const DefaultAuthProviderMinInvalidationInterval = 15 * time.Minute

// AuthBearerRoundTripperOpts is options for AuthBearerRoundTripper.
type AuthBearerRoundTripperOpts struct {
	TokenScope []string

	// MinInvalidationInterval determines the minimum time between cache invalidation attempts
	// for the same AuthProvider. This prevents excessive calls to token endpoints.
	// By default, DefaultAuthProviderMinInvalidationInterval const is used.
	MinInvalidationInterval time.Duration

	// ShouldRefreshTokenAndRetry is called after the first attempt and decides whether a token refresh attempt
	// should be performed and the request should be retried once more (cache invalidation + token refresh + single retry).
	// By default, it returns true only for HTTP 401 responses.
	// If you need to retry only for specific endpoints or HTTP methods, you can provide your own implementation
	// and use http.Response.Request field to make the decision.
	ShouldRefreshTokenAndRetry func(ctx context.Context, resp *http.Response) bool

	// Logger is used for logging cache invalidation and retry attempts.
	// When it's necessary to use context-specific logger, LoggerProvider should be used instead.
	Logger log.FieldLogger

	// LoggerProvider is a function that provides a context-specific logger.
	// One of the typical use cases is to use an auth client in the context of a request handler,
	// where the logger should produce logs with request-specific information (e.g., request ID).
	LoggerProvider func(ctx context.Context) log.FieldLogger
}

// AuthBearerRoundTripper implements http.RoundTripper interface
// and sets the "Authorization: Bearer <token>" HTTP header in outgoing requests using the provided AuthProvider.
// Notes and caveats:
//   - If the request already contains the Authorization header, this RoundTripper delegates to the underlying
//     transport without any token refresh or retry logic.
//   - On an authentication failure (by default HTTP 401), it will optionally invalidate the provider cache (if the
//     provider implements AuthProviderInvalidator), obtain a new token, and retry the request once with the new token.
//   - Only a single retry attempt is performed by design. There is no backoff or multi‑step retry here.
//     focuses strictly on auth-related retries).
//   - For requests with bodies, the component attempts to make the body rewindable to safely retry once (prefers
//     Request.GetBody, then io.ReadSeeker, and as a last resort buffers the entire body in memory). For very large
//     payloads, callers should prefer providing Request.GetBody or a seekable reader to avoid high memory usage.
//   - MinInvalidationInterval throttles how often the provider cache invalidation can happen to prevent hammering token
//     endpoints under bursts of 401s; the throttling is maintained per RoundTripper instance.
type AuthBearerRoundTripper struct {
	Delegate             http.RoundTripper
	AuthProvider         AuthProvider
	opts                 AuthBearerRoundTripperOpts
	mu                   sync.Mutex
	lastInvalidationTime time.Time
}

// NewAuthBearerRoundTripper creates a new AuthBearerRoundTripper.
func NewAuthBearerRoundTripper(delegate http.RoundTripper, authProvider AuthProvider) *AuthBearerRoundTripper {
	return NewAuthBearerRoundTripperWithOpts(delegate, authProvider, AuthBearerRoundTripperOpts{})
}

// NewAuthBearerRoundTripperWithOpts creates a new AuthBearerRoundTripper with options.
func NewAuthBearerRoundTripperWithOpts(
	delegate http.RoundTripper, authProvider AuthProvider, opts AuthBearerRoundTripperOpts,
) *AuthBearerRoundTripper {
	if opts.MinInvalidationInterval == 0 {
		opts.MinInvalidationInterval = DefaultAuthProviderMinInvalidationInterval
	}
	if opts.ShouldRefreshTokenAndRetry == nil {
		opts.ShouldRefreshTokenAndRetry = func(ctx context.Context, resp *http.Response) bool {
			return resp != nil && resp.StatusCode == http.StatusUnauthorized
		}
	}
	if opts.Logger == nil {
		opts.Logger = log.NewDisabledLogger()
	}
	return &AuthBearerRoundTripper{
		Delegate:     delegate,
		AuthProvider: authProvider,
		opts:         opts,
	}
}

func (rt *AuthBearerRoundTripper) logger(ctx context.Context) log.FieldLogger {
	if rt.opts.LoggerProvider != nil {
		return rt.opts.LoggerProvider(ctx)
	}
	return rt.opts.Logger
}

const authorizationHeader = "Authorization"

// RoundTrip executes a single HTTP transaction, returning a Response for the provided Request.
// Behavior overview:
//   - If Authorization header is already present, the request is passed through without any retry/refresh.
//   - Otherwise, it adds a bearer token from AuthProvider and sends the request.
//   - If ShouldRefreshTokenAndRetry returns true (default: on 401), it may invalidate provider cache (throttled by
//     MinInvalidationInterval), fetch a new token, safely rewind the request body if needed, and retry once.
//   - On transport errors (no response), no auth retry is attempted here.
//   - If token refresh fails or token stays the same after invalidation, the original response is returned unchanged.
func (rt *AuthBearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get(authorizationHeader) != "" {
		return rt.Delegate.RoundTrip(req)
	}

	token, err := rt.AuthProvider.GetToken(req.Context(), rt.opts.TokenScope...)
	if err != nil {
		return nil, &AuthBearerRoundTripperError{Inner: err}
	}

	// Prepare to rewind body if needed
	rewindReqBody := func(r *http.Request) error { return nil }
	if req.Body != nil {
		originalReqBody := req.Body
		defer func() {
			_ = originalReqBody.Close() // per RoundTripper contract.
		}()
		if rewindReqBody, err = makeRequestBodyRewindable(req); err != nil {
			return nil, &AuthBearerRoundTripperError{Inner: err}
		}
	}

	req = req.Clone(req.Context()) // Per RoundTripper contract.
	req.Header.Set(authorizationHeader, "Bearer "+token)

	resp, err := rt.Delegate.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if !rt.opts.ShouldRefreshTokenAndRetry(req.Context(), resp) {
		return resp, nil
	}

	authProviderCacheInvalidated := rt.invalidateAuthProviderCacheIfNeeded()
	if authProviderCacheInvalidated {
		rt.logger(req.Context()).Info("auth provider cache invalidated after 401 response")
	}

	newToken, newTokenErr := rt.AuthProvider.GetToken(req.Context(), rt.opts.TokenScope...)
	if newTokenErr != nil {
		rt.logger(req.Context()).Error("failed to get new token after 401 response", log.Error(newTokenErr))
		return resp, nil
	}

	if newToken == token {
		if authProviderCacheInvalidated {
			rt.logger(req.Context()).Warn("auth provider cache invalidated after 401 response, but token is unchanged")
		}
		return resp, nil
	}

	// Rewind body before retry; if it fails, return original response intact.
	if rewindErr := rewindReqBody(req); rewindErr != nil {
		rt.logger(req.Context()).Error("token changed after 401 response, but failed to rewind request body for retry",
			log.Error(rewindErr))
		return resp, nil
	}

	if resp.Body != nil {
		drainResponseBody(resp, rt.logger(req.Context()))
	}
	req.Header.Set(authorizationHeader, "Bearer "+newToken)
	return rt.Delegate.RoundTrip(req)
}

func (rt *AuthBearerRoundTripper) invalidateAuthProviderCacheIfNeeded() (invalidated bool) {
	invalidator, ok := rt.AuthProvider.(AuthProviderInvalidator)
	if !ok {
		return false
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	last := rt.lastInvalidationTime
	if !last.IsZero() {
		if time.Since(last) < rt.opts.MinInvalidationInterval {
			return false
		}
	}

	invalidator.Invalidate()
	rt.lastInvalidationTime = time.Now()
	return true
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

// AuthBearerRoundTripperTestSuite is a test suite for AuthBearerRoundTripper
type AuthBearerRoundTripperTestSuite struct {
	suite.Suite
}

func TestAuthBearerRoundTripperSuite(t *testing.T) {
	suite.Run(t, &AuthBearerRoundTripperTestSuite{})
}

func (s *AuthBearerRoundTripperTestSuite) TestRoundTrip() {
	const reqAuthorizationHeader = "Authorization"

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(reqAuthorizationHeader, r.Header.Get(reqAuthorizationHeader))
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// success
	authProvider := &testAuthProvider{token: "xxx"}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)
	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}
	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())
	s.Require().Equal("Bearer "+authProvider.token, resp.Header.Get(reqAuthorizationHeader))

	// error
	authProviderError := errors.New("auth provider error")
	failingAuthProvider := AuthProviderFunc(func(ctx context.Context, scope ...string) (string, error) {
		return "", authProviderError
	})
	rt = NewAuthBearerRoundTripper(http.DefaultTransport, failingAuthProvider)
	client = http.Client{Transport: rt}
	resp, err = client.Do(req)
	if resp != nil {
		s.Require().NoError(resp.Body.Close())
	}
	var authError *AuthBearerRoundTripperError
	s.Require().ErrorAs(err, &authError)
	s.Require().Equal(&AuthBearerRoundTripperError{Inner: authProviderError}, authError)
}

func (s *AuthBearerRoundTripperTestSuite) TestNoInvalidationWithoutInterface() {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		requestCount++
		rw.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	// Test with regular AuthProvider (no AuthProviderInvalidator interface)
	authProvider := &testAuthProvider{token: "test-token"}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}

	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())

	// Should return 401 without retry
	s.Require().Equal(http.StatusUnauthorized, resp.StatusCode)
	s.Require().Equal(1, requestCount)
}

func (s *AuthBearerRoundTripperTestSuite) TestCacheInvalidationOn401() {
	const reqAuthorizationHeader = "Authorization"
	const oldToken = "old-token"
	const newToken = "new-token"

	// Track request count and return 401 for first request with old token, 200 for new token
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		requestCount++
		authHeader := r.Header.Get(reqAuthorizationHeader)
		rw.Header().Set(reqAuthorizationHeader, authHeader)
		switch authHeader {
		case "Bearer " + oldToken:
			rw.WriteHeader(http.StatusUnauthorized)
		case "Bearer " + newToken:
			rw.WriteHeader(http.StatusOK)
		default:
			rw.WriteHeader(http.StatusInternalServerError) // should not happen
		}
	}))
	defer server.Close()

	// Test with AuthProvider that supports invalidation
	authProvider := &testAuthProviderWithInvalidation{
		tokens: []string{oldToken, newToken},
	}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}

	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())

	// Should have succeeded after retry with new token
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	s.Require().Equal("Bearer "+newToken, resp.Header.Get(reqAuthorizationHeader))

	// Should have made 2 requests (initial 401 + retry with new token)
	s.Require().Equal(2, requestCount)

	// Should have invalidated cache once
	s.Require().Equal(1, authProvider.InvalidateCount())
}

func (s *AuthBearerRoundTripperTestSuite) TestSameTokenAfterInvalidation() {
	const token = "test-token"

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		requestCount++
		rw.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	authProvider := &testAuthProviderWithInvalidation{tokens: []string{token}}

	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}

	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())

	s.Require().Equal(http.StatusUnauthorized, resp.StatusCode)

	// Should have made 1 request since token is the same after invalidation
	s.Require().Equal(1, requestCount)

	// Should have invalidated cache once
	s.Require().Equal(1, authProvider.InvalidateCount())
}

func (s *AuthBearerRoundTripperTestSuite) TestThrottlingInvalidation() {
	const token = "test-token"
	const minInvalidationInterval = 100 * time.Millisecond

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		requestCount++
		rw.WriteHeader(http.StatusUnauthorized) // always return 401
	}))
	defer server.Close()

	authProvider := &testAuthProviderWithInvalidation{
		tokens: []string{token},
	}

	opts := AuthBearerRoundTripperOpts{MinInvalidationInterval: minInvalidationInterval}
	rt := NewAuthBearerRoundTripperWithOpts(http.DefaultTransport, authProvider, opts)
	client := http.Client{Transport: rt}

	doRequest := func() {
		req1, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
		s.Require().NoError(err)
		resp1, err := client.Do(req1)
		s.Require().NoError(err)
		s.Require().NoError(resp1.Body.Close())
		s.Require().Equal(http.StatusUnauthorized, resp1.StatusCode)
	}

	// First request - should trigger invalidation
	doRequest()
	s.Require().Equal(1, authProvider.InvalidateCount())

	// Second request immediately - should NOT trigger invalidation (throttled)
	doRequest()
	s.Require().Equal(1, authProvider.InvalidateCount())

	// Wait for throttle period to pass
	time.Sleep(minInvalidationInterval * 2)

	// Third request - should trigger invalidation again
	doRequest()
	s.Require().Equal(2, authProvider.InvalidateCount())
}

func (s *AuthBearerRoundTripperTestSuite) TestConcurrentInvalidation() {
	const reqAuthorizationHeader = "Authorization"
	const oldToken = "old-token"
	const newToken = "new-token"
	const numGoroutines = 5

	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		authHeader := r.Header.Get(reqAuthorizationHeader)
		switch authHeader {
		case "Bearer " + oldToken:
			rw.WriteHeader(http.StatusUnauthorized)
		case "Bearer " + newToken:
			rw.WriteHeader(http.StatusOK)
		default:
			rw.WriteHeader(http.StatusInternalServerError) // should not happen
		}
	}))
	defer server.Close()

	authProvider := &testAuthProviderWithInvalidation{tokens: []string{oldToken, newToken}}

	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}

	// Launch concurrent requests
	var wg sync.WaitGroup
	var successCount int32
	errs := make(chan error, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
			if err != nil {
				errs <- err
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				errs <- err
				return
			}
			if err = resp.Body.Close(); err != nil {
				errs <- err
				return
			}
			if resp.StatusCode == http.StatusOK {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	select {
	case err := <-errs:
		s.Fail("request failed", "error: %v", err)
	default:
	}
	s.Require().Equal(1, authProvider.InvalidateCount())
	s.Require().Equal(numGoroutines, int(successCount))
	s.Require().GreaterOrEqual(int(requestCount), numGoroutines+1)
}

func (s *AuthBearerRoundTripperTestSuite) TestRequestBodyRewind() {
	const reqAuthorizationHeader = "Authorization"
	const oldToken = "old-token"
	const newToken = "new-token"
	const requestBody = `{"test":"data","retry":"should work"}`

	var receivedBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(reqAuthorizationHeader)
		rw.Header().Set(reqAuthorizationHeader, authHeader)

		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))

		switch authHeader {
		case "Bearer " + oldToken:
			rw.WriteHeader(http.StatusUnauthorized)
		case "Bearer " + newToken:
			rw.WriteHeader(http.StatusOK)
		default:
			rw.WriteHeader(http.StatusInternalServerError) // should not happen
		}
	}))
	defer server.Close()

	authProvider := &testAuthProviderWithInvalidation{tokens: []string{oldToken, newToken}}

	req, err := http.NewRequest(http.MethodPost, server.URL+"/", strings.NewReader(requestBody))
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}

	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())

	// Should have succeeded after retry with new token
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	s.Require().Equal("Bearer "+newToken, resp.Header.Get(reqAuthorizationHeader))

	// Should have received the same body content twice (initial request + retry)
	s.Require().Len(receivedBodies, 2)
	s.Require().Equal(requestBody, receivedBodies[0])
	s.Require().Equal(requestBody, receivedBodies[1])

	// Should have invalidated cache once
	s.Require().Equal(1, authProvider.InvalidateCount())
}

func (s *AuthBearerRoundTripperTestSuite) TestRequestBodyWithGetBody() {
	const reqAuthorizationHeader = "Authorization"
	const oldToken = "old-token"
	const newToken = "new-token"
	const requestBody = `{"mode":"getbody","retry":"works"}`

	var receivedBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(reqAuthorizationHeader)
		rw.Header().Set(reqAuthorizationHeader, authHeader)

		body, _ := io.ReadAll(r.Body)
		receivedBodies = append(receivedBodies, string(body))

		switch authHeader {
		case "Bearer " + oldToken:
			rw.WriteHeader(http.StatusUnauthorized)
		case "Bearer " + newToken:
			rw.WriteHeader(http.StatusOK)
		default:
			rw.WriteHeader(http.StatusInternalServerError) // should not happen
		}
	}))
	defer server.Close()

	authProvider := &testAuthProviderWithInvalidation{tokens: []string{oldToken, newToken}}

	// Build request with Body and GetBody; Body is a non-seekable counting wrapper around strings.NewReader.
	var readCalls int32
	orig := &countingReadCloser{inner: strings.NewReader(requestBody), reads: &readCalls}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/", orig)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")
	// Provide GetBody to recreate the reader on demand.
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(requestBody)), nil
	}

	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProvider)
	client := http.Client{Transport: rt}
	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())

	// Expect retry success and identical bodies
	s.Require().Equal(http.StatusOK, resp.StatusCode)
	s.Require().Len(receivedBodies, 2)
	s.Require().Equal(requestBody, receivedBodies[0])
	s.Require().Equal(requestBody, receivedBodies[1])

	// Because GetBody is used, the initial provided Body should not be consumed at all.
	s.Require().Equal(0, int(readCalls))
}

func (s *AuthBearerRoundTripperTestSuite) TestShouldRefreshTokenAndRetry() {
	const oldToken = "old-token"
	const newToken = "new-token"

	tests := []struct {
		name                       string
		shouldRefreshTokenAndRetry func(ctx context.Context, resp *http.Response) bool
		method                     string
		statusCode                 int
		expectRetry                bool
	}{
		{
			name:        "default behavior - retry on 401",
			statusCode:  http.StatusUnauthorized,
			method:      http.MethodGet,
			expectRetry: true,
		},
		{
			name:        "default behavior - no retry on 403",
			statusCode:  http.StatusForbidden,
			method:      http.MethodGet,
			expectRetry: false,
		},
		{
			name: "method-specific retry - only GET requests",
			shouldRefreshTokenAndRetry: func(ctx context.Context, resp *http.Response) bool {
				if resp.Request.Method != http.MethodGet {
					return false
				}
				return resp != nil && resp.StatusCode == http.StatusUnauthorized
			},
			method:      http.MethodGet,
			statusCode:  http.StatusUnauthorized,
			expectRetry: true,
		},
		{
			name: "method-specific retry - skip POST requests",
			shouldRefreshTokenAndRetry: func(ctx context.Context, resp *http.Response) bool {
				if resp.Request.Method != http.MethodGet {
					return false
				}
				return resp != nil && resp.StatusCode == http.StatusUnauthorized
			},
			method:      http.MethodPost,
			statusCode:  http.StatusUnauthorized,
			expectRetry: false,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				requestCount++
				rw.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			authProvider := &testAuthProviderWithInvalidation{tokens: []string{oldToken, newToken}}

			opts := AuthBearerRoundTripperOpts{ShouldRefreshTokenAndRetry: tt.shouldRefreshTokenAndRetry}
			rt := NewAuthBearerRoundTripperWithOpts(http.DefaultTransport, authProvider, opts)

			req, err := http.NewRequest(tt.method, server.URL, nil)
			s.Require().NoError(err)

			client := http.Client{Transport: rt}
			resp, err := client.Do(req)
			s.Require().NoError(err)
			s.Require().NoError(resp.Body.Close())

			if tt.expectRetry {
				s.Require().Equal(1, authProvider.InvalidateCount())
				s.Require().Equal(2, requestCount)
			} else {
				s.Require().Equal(0, authProvider.InvalidateCount())
				s.Require().Equal(1, requestCount)
			}
		})
	}
}

// countingReadCloser is a non-seekable io.ReadCloser that counts Read calls via an external counter.
type countingReadCloser struct {
	inner io.Reader
	reads *int32
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	atomic.AddInt32(c.reads, 1)
	return c.inner.Read(p)
}

func (c *countingReadCloser) Close() error { return nil }

type testAuthProvider struct {
	err   error
	token string
}

func (p *testAuthProvider) GetToken(ctx context.Context, scope ...string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.token, nil
}

// testAuthProviderWithInvalidation implements AuthProvider with cache invalidation support
type testAuthProviderWithInvalidation struct {
	err             error
	tokens          []string // tokens[0] is initial token, tokens[1] is after the 1st invalidation, etc.
	invalidateCount int32
}

func (p *testAuthProviderWithInvalidation) GetToken(ctx context.Context, scope ...string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	index := atomic.LoadInt32(&p.invalidateCount)
	if int(index) >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1], nil
	}
	return p.tokens[index], nil
}

func (p *testAuthProviderWithInvalidation) Invalidate() {
	atomic.AddInt32(&p.invalidateCount, 1)
}

func (p *testAuthProviderWithInvalidation) InvalidateCount() int {
	return int(atomic.LoadInt32(&p.invalidateCount))
}

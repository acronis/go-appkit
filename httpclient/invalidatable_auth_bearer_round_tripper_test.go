/*
Copyright © 2026 Acronis International GmbH.

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

// InvalidatableAuthBearerRoundTripperTestSuite is a test suite for InvalidatableAuthBearerRoundTripper
type InvalidatableAuthBearerRoundTripperTestSuite struct {
	suite.Suite
}

func TestInvalidatableAuthBearerRoundTripperSuite(t *testing.T) {
	suite.Run(t, &InvalidatableAuthBearerRoundTripperTestSuite{})
}

// newTokenEchoServer returns 401 for the rejected tokens and 200 otherwise, and echoes the Authorization header.
func newTokenEchoServer(rejectedTokens ...string) (server *httptest.Server, requestCount func() int) {
	var mu sync.Mutex
	count := 0
	server = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		mu.Lock()
		count++
		mu.Unlock()
		authHeader := r.Header.Get(authorizationHeader)
		rw.Header().Set(authorizationHeader, authHeader)
		for _, token := range rejectedTokens {
			if authHeader == "Bearer "+token {
				rw.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		rw.WriteHeader(http.StatusOK)
	}))
	return server, func() int {
		mu.Lock()
		defer mu.Unlock()
		return count
	}
}

// doRequest returns the status code and the echoed Authorization header of the response.
func (s *InvalidatableAuthBearerRoundTripperTestSuite) doRequest(
	rt http.RoundTripper, req *http.Request,
) (statusCode int, authHeader string) {
	client := http.Client{Transport: rt}
	resp, err := client.Do(req)
	s.Require().NoError(err)
	s.Require().NoError(resp.Body.Close())
	return resp.StatusCode, resp.Header.Get(authorizationHeader)
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestRoundTrip() {
	server, requestCount := newTokenEchoServer()
	defer server.Close()

	authProvider := &testInvalidatableAuthProvider{tokens: []string{"xxx"}}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, authHeader := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal("Bearer xxx", authHeader)
	s.Require().Equal(1, requestCount())
	s.Require().Empty(authProvider.InvalidatedTokens())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestAuthProviderError() {
	authProviderError := errors.New("auth provider error")
	failingAuthProvider := InvalidatableAuthProviderFunc(
		func(ctx context.Context, scope ...string) (string, func() bool, error) {
			return "", nil, authProviderError
		})

	req, err := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	s.Require().NoError(err)

	client := http.Client{Transport: NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, failingAuthProvider)}
	resp, err := client.Do(req)
	if resp != nil {
		s.Require().NoError(resp.Body.Close())
	}
	var authError *AuthBearerRoundTripperError
	s.Require().ErrorAs(err, &authError)
	s.Require().Equal(&AuthBearerRoundTripperError{Inner: authProviderError}, authError)
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestAuthorizationHeaderPassThrough() {
	server, requestCount := newTokenEchoServer("preset-token")
	defer server.Close()

	authProvider := &testInvalidatableAuthProvider{tokens: []string{"old-token", "new-token"}}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)
	req.Header.Set(authorizationHeader, "Bearer preset-token")

	status, authHeader := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusUnauthorized, status)
	s.Require().Equal("Bearer preset-token", authHeader)
	s.Require().Equal(1, requestCount())
	s.Require().Equal(0, authProvider.GetTokenCount())
	s.Require().Empty(authProvider.InvalidatedTokens())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestInvalidationOn401() {
	const oldToken = "old-token"
	const newToken = "new-token"

	server, requestCount := newTokenEchoServer(oldToken)
	defer server.Close()

	authProvider := &testInvalidatableAuthProvider{tokens: []string{oldToken, newToken}}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, authHeader := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal("Bearer "+newToken, authHeader)
	s.Require().Equal(2, requestCount())
	s.Require().Equal([]string{oldToken}, authProvider.InvalidatedTokens())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestNotInvalidatedAndTokenUnchanged() {
	const token = "test-token"

	server, requestCount := newTokenEchoServer(token)
	defer server.Close()

	// The provider refuses the invalidation (e.g., throttled), so the token stays the same.
	authProvider := &testInvalidatableAuthProvider{tokens: []string{token, "new-token"}, refuseInvalidation: true}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, _ := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusUnauthorized, status)
	s.Require().Equal(1, requestCount())
	s.Require().Equal(1, authProvider.InvalidateCallCount())
	s.Require().Empty(authProvider.InvalidatedTokens())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestSameTokenAfterInvalidation() {
	const token = "test-token"

	server, requestCount := newTokenEchoServer(token)
	defer server.Close()

	authProvider := &testInvalidatableAuthProvider{tokens: []string{token}}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, _ := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusUnauthorized, status)
	s.Require().Equal(1, requestCount())
	s.Require().Equal([]string{token}, authProvider.InvalidatedTokens())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestNilInvalidator() {
	const oldToken = "old-token"
	const newToken = "new-token"

	server, requestCount := newTokenEchoServer(oldToken)
	defer server.Close()

	// The token changes without an invalidator (e.g., it was reissued in the background).
	var mu sync.Mutex
	calls := 0
	authProvider := InvalidatableAuthProviderFunc(func(ctx context.Context, scope ...string) (string, func() bool, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			return oldToken, nil, nil
		}
		return newToken, nil, nil
	})
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, authHeader := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal("Bearer "+newToken, authHeader)
	s.Require().Equal(2, requestCount())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestNewTokenError() {
	const oldToken = "old-token"

	server, requestCount := newTokenEchoServer(oldToken)
	defer server.Close()

	authProvider := &testInvalidatableAuthProvider{
		tokens: []string{oldToken, "new-token"}, errAfterInvalidation: errors.New("idp is down"),
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, _ := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusUnauthorized, status)
	s.Require().Equal(1, requestCount())
	s.Require().Equal([]string{oldToken}, authProvider.InvalidatedTokens())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestTransportErrorNoRetry() {
	transportErr := errors.New("connection refused")
	transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	authProvider := &testInvalidatableAuthProvider{tokens: []string{"old-token", "new-token"}}
	req, err := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	s.Require().NoError(err)

	rt := NewInvalidatableAuthBearerRoundTripper(transport, authProvider)
	resp, err := rt.RoundTrip(req)
	if resp != nil {
		s.Require().NoError(resp.Body.Close())
	}

	s.Require().ErrorIs(err, transportErr)
	s.Require().Equal(1, authProvider.GetTokenCount())
	s.Require().Equal(0, authProvider.InvalidateCallCount())
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestRequestBodyRewind() {
	const oldToken = "old-token"
	const newToken = "new-token"
	const requestBody = `{"test":"data","retry":"should work"}`

	var mu sync.Mutex
	var receivedBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		if r.Header.Get(authorizationHeader) == "Bearer "+oldToken {
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		rw.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	authProvider := &testInvalidatableAuthProvider{tokens: []string{oldToken, newToken}}
	// A non-seekable body without GetBody makes the round tripper buffer it.
	var readCalls int32
	body := &countingReadCloser{inner: strings.NewReader(requestBody), reads: &readCalls}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/", body)
	s.Require().NoError(err)

	status, _ := s.doRequest(NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal([]string{requestBody, requestBody}, receivedBodies)
	s.Require().Equal([]string{oldToken}, authProvider.InvalidatedTokens())
	s.Require().Positive(atomic.LoadInt32(&readCalls)) // The original body was read into the buffer.
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestShouldRefreshTokenAndRetry() {
	const oldToken = "old-token"
	const newToken = "new-token"

	onlyGet := func(ctx context.Context, resp *http.Response) bool {
		return resp.Request.Method == http.MethodGet && resp.StatusCode == http.StatusUnauthorized
	}
	tests := []struct {
		name                       string
		shouldRefreshTokenAndRetry func(ctx context.Context, resp *http.Response) bool
		method                     string
		statusCode                 int
		expectRetry                bool
	}{
		{name: "default behavior - retry on 401", method: http.MethodGet, statusCode: http.StatusUnauthorized, expectRetry: true},
		{name: "default behavior - no retry on 403", method: http.MethodGet, statusCode: http.StatusForbidden},
		{
			name: "method-specific retry - only GET requests", shouldRefreshTokenAndRetry: onlyGet,
			method: http.MethodGet, statusCode: http.StatusUnauthorized, expectRetry: true,
		},
		{
			name: "method-specific retry - skip POST requests", shouldRefreshTokenAndRetry: onlyGet,
			method: http.MethodPost, statusCode: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			var mu sync.Mutex
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requestCount++
				mu.Unlock()
				rw.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			authProvider := &testInvalidatableAuthProvider{tokens: []string{oldToken, newToken}}
			opts := InvalidatableAuthBearerRoundTripperOpts{ShouldRefreshTokenAndRetry: tt.shouldRefreshTokenAndRetry}
			rt := NewInvalidatableAuthBearerRoundTripperWithOpts(http.DefaultTransport, authProvider, opts)

			req, err := http.NewRequest(tt.method, server.URL, nil)
			s.Require().NoError(err)
			s.doRequest(rt, req)

			if tt.expectRetry {
				s.Require().Equal([]string{oldToken}, authProvider.InvalidatedTokens())
				s.Require().Equal(2, requestCount)
			} else {
				s.Require().Empty(authProvider.InvalidatedTokens())
				s.Require().Equal(1, requestCount)
			}
		})
	}
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestTokenScope() {
	server, _ := newTokenEchoServer()
	defer server.Close()

	var mu sync.Mutex
	var gotScopes [][]string
	authProvider := InvalidatableAuthProviderFunc(func(ctx context.Context, scope ...string) (string, func() bool, error) {
		mu.Lock()
		defer mu.Unlock()
		gotScopes = append(gotScopes, scope)
		return "token", nil, nil
	})
	opts := InvalidatableAuthBearerRoundTripperOpts{TokenScope: []string{"scope-a", "scope-b"}}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	s.doRequest(NewInvalidatableAuthBearerRoundTripperWithOpts(http.DefaultTransport, authProvider, opts), req)

	s.Require().Equal([][]string{{"scope-a", "scope-b"}}, gotScopes)
}

// TestAuthBearerRoundTripperIgnoresInvalidator checks that AuthBearerRoundTripper keeps its behavior
// for a provider that implements both AuthProvider and InvalidatableAuthProvider.
func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestAuthBearerRoundTripperIgnoresInvalidator() {
	const oldToken = "old-token"
	const newToken = "new-token"

	server, requestCount := newTokenEchoServer(oldToken)
	defer server.Close()

	authProvider := &testDualAuthProvider{
		testAuthProviderWithInvalidation: testAuthProviderWithInvalidation{tokens: []string{oldToken, newToken}},
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	s.Require().NoError(err)

	status, authHeader := s.doRequest(NewAuthBearerRoundTripper(http.DefaultTransport, authProvider), req)

	s.Require().Equal(http.StatusOK, status)
	s.Require().Equal("Bearer "+newToken, authHeader)
	s.Require().Equal(2, requestCount())
	s.Require().Equal(1, authProvider.InvalidateCount())
	s.Require().Equal(0, authProvider.getTokenWithInvalidatorCount)
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// testInvalidatableAuthProvider returns tokens[i] after i successful invalidations
// and records the tokens it invalidated.
type testInvalidatableAuthProvider struct {
	mu                   sync.Mutex
	tokens               []string
	refuseInvalidation   bool
	errAfterInvalidation error
	getTokenCount        int
	invalidateCallCount  int
	invalidatedTokens    []string
}

func (p *testInvalidatableAuthProvider) GetTokenWithInvalidator(
	ctx context.Context, scope ...string,
) (string, func() bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.getTokenCount++
	index := len(p.invalidatedTokens)
	if index > 0 && p.errAfterInvalidation != nil {
		return "", nil, p.errAfterInvalidation
	}
	token := p.tokens[min(index, len(p.tokens)-1)]
	invalidated := false
	return token, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.invalidateCallCount++
		if p.refuseInvalidation || invalidated {
			return false
		}
		invalidated = true
		p.invalidatedTokens = append(p.invalidatedTokens, token)
		return true
	}, nil
}

func (p *testInvalidatableAuthProvider) GetTokenCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getTokenCount
}

func (p *testInvalidatableAuthProvider) InvalidateCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.invalidateCallCount
}

func (p *testInvalidatableAuthProvider) InvalidatedTokens() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.invalidatedTokens...)
}

// testDualAuthProvider implements AuthProvider, AuthProviderInvalidator and InvalidatableAuthProvider.
type testDualAuthProvider struct {
	testAuthProviderWithInvalidation
	getTokenWithInvalidatorCount int
}

func (p *testDualAuthProvider) GetTokenWithInvalidator(
	ctx context.Context, scope ...string,
) (string, func() bool, error) {
	p.getTokenWithInvalidatorCount++
	return "", nil, errors.New("must not be called")
}

func (s *InvalidatableAuthBearerRoundTripperTestSuite) TestConcurrent401OnSameToken() {
	const goroutines = 10
	const oldToken = "old-token"
	const newToken = "new-token"

	// The server holds the 401 responses until all requests arrive with the old token.
	var oldTokenRequests atomic.Int32
	allArrived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(authorizationHeader)
		rw.Header().Set(authorizationHeader, authHeader)
		if authHeader == "Bearer "+oldToken {
			if oldTokenRequests.Add(1) == goroutines {
				close(allArrived)
			}
			select {
			case <-allArrived:
			case <-time.After(5 * time.Second):
			}
			rw.WriteHeader(http.StatusUnauthorized)
			return
		}
		rw.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	authProvider := &testRotatingAuthProvider{tokens: []string{oldToken, newToken}}
	rt := NewInvalidatableAuthBearerRoundTripper(http.DefaultTransport, authProvider)

	var wg sync.WaitGroup
	statuses := make([]int, goroutines)
	authHeaders := make([]string, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
			if err != nil {
				return
			}
			resp, err := (&http.Client{Transport: rt}).Do(req)
			if err != nil {
				return
			}
			_ = resp.Body.Close()
			statuses[i], authHeaders[i] = resp.StatusCode, resp.Header.Get(authorizationHeader)
		}()
	}
	wg.Wait()

	for i := range goroutines {
		s.Require().Equal(http.StatusOK, statuses[i])
		s.Require().Equal("Bearer "+newToken, authHeaders[i])
	}
	s.Require().EqualValues(goroutines, oldTokenRequests.Load())
	s.Require().Equal(1, authProvider.SuccessfulInvalidations())
}

// testRotatingAuthProvider holds one current token. An invalidator replaces the current token with the next one
// only if the current token is still its token, as a cache with compare-and-delete does.
type testRotatingAuthProvider struct {
	mu                      sync.Mutex
	tokens                  []string
	current                 int
	successfulInvalidations int
}

func (p *testRotatingAuthProvider) GetTokenWithInvalidator(
	ctx context.Context, scope ...string,
) (string, func() bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	index := p.current
	return p.tokens[index], func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.current != index || index == len(p.tokens)-1 {
			return false
		}
		p.current++
		p.successfulInvalidations++
		return true
	}, nil
}

func (p *testRotatingAuthProvider) SuccessfulInvalidations() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.successfulInvalidations
}

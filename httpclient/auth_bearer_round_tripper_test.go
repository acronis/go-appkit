/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type testAuthProfider struct {
	err   error
	token string
}

func (p *testAuthProfider) GetToken(ctx context.Context, scope ...string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.token, nil
}

func TestAuthBearerRoundTripper_RoundTrip(t *testing.T) {
	const (
		reqAuthorizationHeader = "Authorization"
	)

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get(reqAuthorizationHeader)
		rw.Header().Set(reqAuthorizationHeader, auth)
		rw.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// success
	authProfider := &testAuthProfider{token: "xxx"}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/", nil)
	require.NoError(t, err)
	rt := NewAuthBearerRoundTripper(http.DefaultTransport, authProfider)
	client := http.Client{Transport: rt}
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, fmt.Sprintf("Bearer %s", authProfider.token), resp.Header.Get(reqAuthorizationHeader))

	// error
	authProviderError := errors.New("auth provider error")
	failingAuthProvider := AuthProviderFunc(func(ctx context.Context, scope ...string) (string, error) {
		return "", authProviderError
	})
	rt = NewAuthBearerRoundTripper(http.DefaultTransport, failingAuthProvider)
	client = http.Client{Transport: rt}
	resp, err = client.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	var authError *AuthBearerRoundTripperError
	require.ErrorAs(t, err, &authError)
	require.Equal(t, &AuthBearerRoundTripperError{Inner: authProviderError}, authError)
}

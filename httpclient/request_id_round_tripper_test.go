/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewRequestIDRoundTripper(t *testing.T) {
	requestID := "12345"

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		require.Equal(t, requestID, r.Header.Get("X-Request-ID"))
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	requestIDRoundTripper := NewRequestIDRoundTripper(http.DefaultTransport)
	client := &http.Client{Transport: requestIDRoundTripper}
	ctx := middleware.NewContextWithRequestID(context.Background(), requestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
}

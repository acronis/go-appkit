/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/httpserver/middleware"
)

func TestNewRequestIDRoundTripper(t *testing.T) {
	requestID := "12345"
	var receivedRequestID string

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		receivedRequestID = r.Header.Get("X-Request-ID")
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
	require.Equal(t, requestID, receivedRequestID)
}

func TestNewRequestIDRoundTripperWithOpts(t *testing.T) {
	requestID := "12345"
	prefix := "my_custom_request_provider"
	var receivedRequestID string

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		receivedRequestID = prefix + requestID
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	requestIDRoundTripper := NewRequestIDRoundTripperWithOpts(http.DefaultTransport, RequestIDRoundTripperOpts{
		RequestIDProvider: func(ctx context.Context) string {
			return prefix + requestID
		},
	})
	client := &http.Client{Transport: requestIDRoundTripper}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.Equal(t, prefix+requestID, receivedRequestID)
}

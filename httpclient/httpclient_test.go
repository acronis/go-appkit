/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log/logtest"
)

func TestNewHTTPClientLoggingRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Log.Enabled = true
	client, err := New(cfg)
	require.NoError(t, err)

	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())
}

func TestMustHTTPClientLoggingRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Log.Enabled = true
	client := Must(cfg)
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())
}

func TestNewHTTPClientWithOptsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Log.Enabled = true
	client, err := NewWithOpts(cfg, Opts{
		UserAgent:  "test-agent",
		ClientType: "test-client-type",
	})
	require.NoError(t, err)
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())
	require.Contains(
		t, logger.Entries()[0].Text, fmt.Sprintf(
			"client http request POST %s status code 418", server.URL,
		),
	)
}

func TestMustHTTPClientWithOptsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Log.Enabled = true
	client := MustWithOpts(cfg, Opts{
		UserAgent:  "test-agent",
		ClientType: "test-client-type",
	})
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())
	require.Contains(
		t, logger.Entries()[0].Text, fmt.Sprintf("client http request POST %s status code 418", server.URL),
	)
	require.Contains(t, logger.Entries()[0].Text, "client type test-client-type")
}

func TestMustHTTPClientWithOptsRoundTripperPolicy(t *testing.T) {
	var retriesCount int
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		retriesCount++
		rw.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.Retries.Enabled = true
	cfg.Retries.MaxAttempts = 1
	cfg.Retries.Policy = RetryPolicyExponential
	cfg.Retries.ExponentialBackoff = ExponentialBackoffConfig{
		InitialInterval: 2 * time.Millisecond,
		Multiplier:      1.1,
	}

	client := MustWithOpts(cfg, Opts{
		UserAgent:  "test-agent",
		ClientType: "test-client-type",
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.Equal(t, 2, retriesCount)
}

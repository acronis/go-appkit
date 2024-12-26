/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log/logtest"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHTTPClientLoggingRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Logger.Enabled = true
	client, err := NewHTTPClient(cfg, "test-agent", "test-request", nil, ClientProviders{})
	require.NoError(t, err)

	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())

	loggerEntry := logger.Entries()[0]
	require.Contains(t, loggerEntry.Text, "client http request POST "+server.URL+" req type test-request status code 418")
}

func TestMustHTTPClientLoggingRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Logger.Enabled = true
	client := MustHTTPClient(cfg, "test-agent", "test-request", nil, ClientProviders{})
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())

	loggerEntry := logger.Entries()[0]
	require.Contains(t, loggerEntry.Text, "client http request POST "+server.URL+" req type test-request status code 418")
}

func TestNewHTTPClientWithOptsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Logger.Enabled = true
	client, err := NewHTTPClientWithOpts(ClientOpts{
		Config:    *cfg,
		UserAgent: "test-agent",
		ReqType:   "test-request",
		Delegate:  nil,
	})
	require.NoError(t, err)
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())
}

func TestMustHTTPClientWithOptsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Logger.Enabled = true
	client := MustHTTPClientWithOpts(ClientOpts{
		Config:    *cfg,
		UserAgent: "test-agent",
		ReqType:   "test-request",
		Delegate:  nil,
	})
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())
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
	cfg.Retries.Policy.Strategy = RetryPolicyExponential
	cfg.Retries.Policy.ExponentialBackoffInitialInterval = 2 * time.Millisecond
	cfg.Retries.Policy.ExponentialBackoffMultiplier = 1.1

	client := MustHTTPClientWithOpts(ClientOpts{
		Config:    *cfg,
		UserAgent: "test-agent",
		ReqType:   "test-request",
		Delegate:  nil,
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.Equal(t, 2, retriesCount)
}

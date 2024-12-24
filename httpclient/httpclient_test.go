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

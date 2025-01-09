/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
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
	require.Contains(
		t, logger.Entries()[0].Text,
		fmt.Sprintf("client http request POST %s req type %s status code 418", server.URL, DefaultReqType),
	)
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
	require.Contains(
		t, logger.Entries()[0].Text,
		fmt.Sprintf("client http request POST %s req type %s status code 418", server.URL, DefaultReqType),
	)
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
	require.Contains(
		t, logger.Entries()[0].Text, fmt.Sprintf(
			"client http request POST %s req type test-request status code 418", server.URL,
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
	require.Contains(
		t, logger.Entries()[0].Text, fmt.Sprintf(
			"client http request POST %s req type test-request status code 418", server.URL,
		),
	)
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
	cfg.Retries.Policy.Policy = RetryPolicyExponential
	cfg.Retries.Policy.ExponentialBackoff = ExponentialBackoffConfig{
		InitialInterval: 2 * time.Millisecond,
		Multiplier:      1.1,
	}

	client := MustWithOpts(cfg, Opts{
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

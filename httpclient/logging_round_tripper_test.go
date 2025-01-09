/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
	"github.com/stretchr/testify/require"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewLoggingRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	loggerRoundTripper := NewLoggingRoundTripper(http.DefaultTransport)
	client := &http.Client{Transport: loggerRoundTripper}
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())

	loggerEntry := logger.Entries()[0]
	require.Contains(
		t, loggerEntry.Text,
		fmt.Sprintf("client http request POST %s req type %s status code 418", server.URL, DefaultReqType),
	)
}

func TestMustHTTPClientLoggingRoundTripperError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Log.Enabled = true
	client := Must(cfg)
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	require.Error(t, err)
	require.Nil(t, r)
	require.NotEmpty(t, logger.Entries())

	loggerEntry := logger.Entries()[0]
	require.Contains(
		t, loggerEntry.Text,
		fmt.Sprintf("client http request POST %s req type %s", serverURL, DefaultReqType),
	)
	require.Contains(t, loggerEntry.Text, "err dial tcp "+ln.Addr().String()+": connect: connection refused")
	require.NotContains(t, loggerEntry.Text, "status code")
}

func TestMustHTTPClientLoggingRoundTripperDisabled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	client := Must(cfg)
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	require.Error(t, err)
	require.Nil(t, r)
	require.Empty(t, logger.Entries())
}

func TestNewLoggingRoundTripperModes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			rw.WriteHeader(http.StatusBadRequest)
		} else {
			rw.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	tests := []struct {
		name   string
		method string
		mode   LoggingMode
		want   int
	}{
		{
			name:   "no requests are logged because of 'none' mode",
			method: http.MethodGet,
			mode:   "none",
		},
		{
			name:   "4xx and 5xx requests are logged because of 'failed' mode",
			method: http.MethodPost,
			mode:   "failed",
			want:   1,
		},
		{
			name:   "only 4xx and 5xx requests so no logs because of 'failed' mode for 2xx",
			method: http.MethodGet,
			mode:   "failed",
		},
		{
			name:   "success requests are logged because of 'all' mode",
			method: http.MethodGet,
			mode:   "all",
			want:   1,
		},
		{
			name:   "failed requests are logged because of 'all' mode",
			method: http.MethodPost,
			mode:   "all",
			want:   1,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			logger := logtest.NewRecorder()
			loggerRoundTripper := NewLoggingRoundTripperWithOpts(
				http.DefaultTransport,
				LoggingRoundTripperOpts{
					ReqType: "test-request",
					Mode:    tt.mode,
				},
			)
			client := &http.Client{Transport: loggerRoundTripper}
			ctx := middleware.NewContextWithLogger(context.Background(), logger)
			req, err := http.NewRequestWithContext(ctx, tt.method, server.URL, nil)
			require.NoError(t, err)

			r, err := client.Do(req)
			defer func() { _ = r.Body.Close() }()
			require.NoError(t, err)
			require.Len(t, logger.Entries(), tt.want)
		})
	}
}

func TestMustHTTPClientLoggingRoundTripperOpts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	serverURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	logger := logtest.NewRecorder()
	cfg := NewConfig()
	cfg.Log.Enabled = true

	var loggerProviderCalled bool
	client := MustWithOpts(cfg, Opts{
		UserAgent: "test-agent",
		ReqType:   "test-request",
		LoggerProvider: func(ctx context.Context) log.FieldLogger {
			loggerProviderCalled = true
			return logger
		},
	})

	ctx := middleware.NewContextWithLogger(context.Background(), nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	require.Error(t, err)
	require.Nil(t, r)
	require.True(t, loggerProviderCalled)
	require.Len(t, logger.Entries(), 1)
}

func TestNewLoggingRoundTripperWithRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	requestID := "12345"
	logger := logtest.NewRecorder()
	loggerRoundTripper := NewLoggingRoundTripper(http.DefaultTransport)
	client := &http.Client{Transport: loggerRoundTripper}
	ctx := middleware.NewContextWithLogger(context.Background(), logger)
	ctx = middleware.NewContextWithRequestID(ctx, requestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())

	loggerEntry := logger.Entries()[0]
	require.Contains(t, loggerEntry.Text, fmt.Sprintf("request id %s", requestID))
}

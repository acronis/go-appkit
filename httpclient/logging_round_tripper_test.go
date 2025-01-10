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
	"time"
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
		fmt.Sprintf("client http request POST %s req type %s status code 418", server.URL, DefaultRequestType),
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
		fmt.Sprintf("client http request POST %s req type %s", serverURL, DefaultRequestType),
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
		UserAgent:   "test-agent",
		RequestType: "test-request",
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Request-ID", requestID)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)
	require.NotEmpty(t, logger.Entries())

	loggerEntry := logger.Entries()[0]
	require.Contains(t, loggerEntry.Text, fmt.Sprintf("request id %s", requestID))
}

func TestNewLoggingRoundTripperLevels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			rw.WriteHeader(http.StatusTeapot)
		} else {
			rw.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	tests := []struct {
		name   string
		opts   LoggingRoundTripperOpts
		level  log.Level
		method string
		noLogs bool
	}{
		{
			name:   "log mode 'all' failed requests with error level",
			opts:   LoggingRoundTripperOpts{Mode: LoggingModeAll},
			level:  log.LevelError,
			method: http.MethodPost,
		},
		{
			name:   "log mode 'failed' failed requests with error level",
			opts:   LoggingRoundTripperOpts{Mode: LoggingModeFailed},
			level:  log.LevelError,
			method: http.MethodPost,
		},
		{
			name:   "log mode 'all' slow successful requests with warn level",
			opts:   LoggingRoundTripperOpts{Mode: LoggingModeAll},
			level:  log.LevelWarn,
			method: http.MethodGet,
		},
		{
			name:   "log mode 'failed' slow successful requests with warn level",
			opts:   LoggingRoundTripperOpts{Mode: LoggingModeFailed},
			level:  log.LevelWarn,
			method: http.MethodGet,
		},
		{
			name:   "log mode 'all' requests with info level under slow threshold",
			opts:   LoggingRoundTripperOpts{Mode: LoggingModeAll, SlowRequestThreshold: 5 * time.Second},
			level:  log.LevelInfo,
			method: http.MethodGet,
		},
		{
			name:   "log mode 'failed' do not log requests for successful requests under slow threshold",
			opts:   LoggingRoundTripperOpts{Mode: LoggingModeFailed, SlowRequestThreshold: 5 * time.Second},
			method: http.MethodGet,
			noLogs: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			logger := logtest.NewRecorder()
			loggerRoundTripper := NewLoggingRoundTripperWithOpts(http.DefaultTransport, tt.opts)
			client := &http.Client{Transport: loggerRoundTripper}
			ctx := middleware.NewContextWithLogger(context.Background(), logger)
			req, err := http.NewRequestWithContext(ctx, tt.method, server.URL, nil)
			require.NoError(t, err)

			r, err := client.Do(req)
			defer func() { _ = r.Body.Close() }()
			require.NoError(t, err)

			entries := logger.Entries()
			if tt.noLogs {
				require.Empty(t, entries)
				return
			}
			require.NotEmpty(t, entries)
			require.Equal(t, tt.level, entries[0].Level)
		})
	}
}

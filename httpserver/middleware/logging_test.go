/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
)

type mockLoggingNextHandler struct {
	called                   int
	lastContextLogger        log.FieldLogger
	lastContextLoggingParams *LoggingParams
	respStatusCode           int
}

func (h *mockLoggingNextHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.called++
	h.lastContextLogger = GetLoggerFromContext(r.Context())
	h.lastContextLoggingParams = GetLoggingParamsFromContext(r.Context())
	rw.WriteHeader(h.respStatusCode)
	_, _ = rw.Write([]byte(http.StatusText(h.respStatusCode)))
}

func TestLoggingHandler_ServeHTTP(t *testing.T) {
	const (
		extReqID    = "external-request-id"
		intReqID    = "internal-request-id"
		userAgent   = "http-client"
		urlPath     = "/endpoint"
		bodyContent = "body-content"
	)

	requireCommonFields := func(t *testing.T, logEntry logtest.RecordedEntry) {
		requireLogFieldString(t, logEntry, "request_id", extReqID)
		requireLogFieldString(t, logEntry, "int_request_id", intReqID)
		requireLogFieldString(t, logEntry, "method", http.MethodPost)
		requireLogFieldString(t, logEntry, "uri", urlPath)
		requireLogFieldInt(t, logEntry, "content_length", len(bodyContent))
		requireLogFieldString(t, logEntry, "user_agent", userAgent)
	}

	// Create request.
	req := httptest.NewRequest(http.MethodPost, urlPath, bytes.NewReader([]byte(bodyContent)))
	req.Header.Set("X-Header-1", "header1-value")
	req.Header.Set("X-Header-2", "header2-value")
	req = req.WithContext(NewContextWithRequestID(req.Context(), extReqID))
	req = req.WithContext(NewContextWithInternalRequestID(req.Context(), intReqID))
	req.Header.Set("User-Agent", userAgent)

	tests := []struct {
		Name              string
		Opts              LoggingOpts
		StatusCode        int
		WantLoggedHeaders map[string]string
	}{
		{
			Name:       "RequestStart is false, RequestHeaders is empty",
			Opts:       LoggingOpts{},
			StatusCode: http.StatusInternalServerError,
		},
		{
			Name:       "RequestStart is true, RequestHeaders is empty",
			Opts:       LoggingOpts{RequestStart: true},
			StatusCode: http.StatusBadRequest,
		},
		{
			Name:              "RequestStart is false, RequestHeaders is not empty",
			Opts:              LoggingOpts{RequestHeaders: map[string]string{"X-Header-1": "header_1", "X-Header-3": "header_3"}},
			StatusCode:        http.StatusOK,
			WantLoggedHeaders: map[string]string{"header_1": "header1-value", "header_3": ""},
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			logger := logtest.NewRecorder()
			handler := &mockLoggingNextHandler{respStatusCode: tt.StatusCode}
			resp := httptest.NewRecorder()
			LoggingWithOpts(logger, tt.Opts)(handler).ServeHTTP(resp, req)
			require.Equal(t, 1, handler.called)
			require.NotNil(t, handler.lastContextLogger)

			wantLoggedLines := 1
			if tt.Opts.RequestStart {
				wantLoggedLines++
			}
			require.Equal(t, wantLoggedLines, len(logger.Entries()))

			if tt.Opts.RequestStart {
				logEntry := logger.Entries()[0]
				require.True(t, strings.Contains(logEntry.Text, "request started"))
				require.Equal(t, log.LevelInfo, logEntry.Level)
				requireCommonFields(t, logEntry)
			}

			logEntry := logger.Entries()[wantLoggedLines-1]
			require.True(t, strings.Contains(logEntry.Text, "response completed"))
			require.Equal(t, log.LevelInfo, logEntry.Level)
			requireCommonFields(t, logEntry)
			requireLogFieldInt(t, logEntry, "status", tt.StatusCode)
			requireLogFieldInt(t, logEntry, "bytes_sent", len(http.StatusText(tt.StatusCode)))
			for logKey, logVal := range tt.WantLoggedHeaders {
				requireLogFieldString(t, logEntry, logKey, logVal)
			}

			requireRemoteAddrIPAndPort(t, logEntry, req.RemoteAddr)
		})
	}
}

func TestLoggingHandler_ServeHTTP_ExcludedEndpoints(t *testing.T) {
	const excludedEndpoint = "/endpoint"

	tests := []struct {
		Name       string
		URLPath    string
		StatusCode int
		LogsCount  int
	}{
		{
			Name:       "Do not log successful excluded endpoint",
			URLPath:    "/endpoint",
			StatusCode: http.StatusOK,
			LogsCount:  0,
		},
		{
			Name:       "Do not log successful excluded endpoint with query params",
			URLPath:    "/endpoint?foo=bar",
			StatusCode: http.StatusOK,
			LogsCount:  0,
		},
		{
			Name:       "Log request errors for excluded endpoint",
			URLPath:    "/endpoint",
			StatusCode: http.StatusBadRequest,
			LogsCount:  1,
		},
		{
			Name:       "Other endpoint not affected by this settings",
			URLPath:    "/otherendpoint",
			StatusCode: http.StatusOK,
			LogsCount:  2,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.URLPath, bytes.NewReader([]byte("body-content")))
			logger := logtest.NewRecorder()
			next := &mockLoggingNextHandler{respStatusCode: tt.StatusCode}
			resp := httptest.NewRecorder()
			h := LoggingWithOpts(logger, LoggingOpts{
				RequestStart:      true,
				ExcludedEndpoints: []string{excludedEndpoint},
			})(next)

			h.ServeHTTP(resp, req)

			require.Equal(t, 1, next.called)
			require.Equal(t, tt.LogsCount, len(logger.Entries()))
		})
	}
}

func TestLoggingHandler_ServeHTTP_SecretQueryParams(t *testing.T) {
	tests := []struct {
		Name              string
		URI               string
		SecretQueryParams []string
		WantLoggedURI     string
	}{
		{
			Name:          "no query params, no secret params",
			URI:           "/endpoint",
			WantLoggedURI: "/endpoint",
		},
		{
			Name:          "query params, no secret params",
			URI:           "/endpoint?foo=bar",
			WantLoggedURI: "/endpoint?foo=bar",
		},
		{
			Name:              "no query params, secret params",
			URI:               "/endpoint",
			SecretQueryParams: []string{"token", "secret"},
			WantLoggedURI:     "/endpoint",
		},
		{
			Name:              "query params, single secret param",
			URI:               "/endpoint?foo=bar&token=702b9bfc05a1bd74",
			SecretQueryParams: []string{"token"},
			WantLoggedURI:     "/endpoint?foo=bar&token=_HIDDEN_",
		},
		{
			Name:              "query params, multiple secret params",
			URI:               "/endpoint?foo=bar&secret=cc754e768394e7d6&token=702b9bfc05a1bd74",
			SecretQueryParams: []string{"token", "secret"},
			WantLoggedURI:     "/endpoint?foo=bar&secret=_HIDDEN_&token=_HIDDEN_",
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.URI, bytes.NewReader([]byte("body-content")))
			logger := logtest.NewRecorder()
			next := &mockLoggingNextHandler{respStatusCode: http.StatusOK}
			resp := httptest.NewRecorder()
			h := LoggingWithOpts(logger, LoggingOpts{
				RequestStart:      true,
				SecretQueryParams: tt.SecretQueryParams,
			})(next)

			h.ServeHTTP(resp, req)

			require.Equal(t, 1, next.called)
			require.Equal(t, 2, len(logger.Entries()))
			logEntry := logger.Entries()[0]
			require.True(t, strings.Contains(logEntry.Text, "request started"))
			logField, found := logEntry.FindField("uri")
			require.True(t, found)
			wantParsedURL, err := url.Parse(tt.WantLoggedURI)
			require.NoError(t, err)
			parsedURL, err := url.Parse(string(logField.Bytes))
			require.NoError(t, err)
			require.Equal(t, wantParsedURL.Path, parsedURL.Path)
			require.EqualValues(t, wantParsedURL.Query(), parsedURL.Query())
		})
	}
}

func TestLoggingHandler_ServeHTTP_LoggingParams(t *testing.T) {
	const (
		extReqID    = "external-request-id"
		intReqID    = "internal-request-id"
		urlPath     = "/endpoint"
		bodyContent = "body-content"
	)

	req := httptest.NewRequest(http.MethodGet, urlPath, bytes.NewReader([]byte(bodyContent)))
	req = req.WithContext(NewContextWithRequestID(req.Context(), extReqID))
	req = req.WithContext(NewContextWithInternalRequestID(req.Context(), intReqID))

	logger := logtest.NewRecorder()
	handler := &mockLoggingNextHandler{respStatusCode: http.StatusOK}
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			GetLoggingParamsFromContext(r.Context()).ExtendFields(
				log.String("next_middleware_key_1", "value"),
				log.Int("next_middleware_key_2", 100500),
			)
			next.ServeHTTP(rw, r)
		})
	}
	resp := httptest.NewRecorder()
	Logging(logger)(mw(handler)).ServeHTTP(resp, req)
	require.Equal(t, 1, handler.called)
	require.NotNil(t, handler.lastContextLogger)
	require.NotNil(t, handler.lastContextLoggingParams)

	require.Len(t, logger.Entries(), 1)
	logEntry := logger.Entries()[0]
	require.True(t, strings.Contains(logEntry.Text, "response completed"))
	require.Equal(t, log.LevelInfo, logEntry.Level)

	requireLogFieldString(t, logEntry, "request_id", extReqID)
	requireLogFieldString(t, logEntry, "int_request_id", intReqID)
	requireLogFieldString(t, logEntry, "method", http.MethodGet)
	requireLogFieldString(t, logEntry, "uri", urlPath)
	requireLogFieldInt(t, logEntry, "content_length", len(bodyContent))
	requireLogFieldInt(t, logEntry, "status", http.StatusOK)
	requireLogFieldInt(t, logEntry, "bytes_sent", len(http.StatusText(http.StatusOK)))

	requireLogFieldString(t, logEntry, "next_middleware_key_1", "value")
	requireLogFieldInt(t, logEntry, "next_middleware_key_2", 100500)
}

func TestLoggingHandler_ServeHTTP_CustomLogger(t *testing.T) {
	const (
		extReqID    = "external-request-id"
		intReqID    = "internal-request-id"
		urlPath     = "/endpoint"
		bodyContent = "body-content"
	)

	req := httptest.NewRequest(http.MethodGet, urlPath, bytes.NewReader([]byte(bodyContent)))
	req = req.WithContext(NewContextWithRequestID(req.Context(), extReqID))
	req = req.WithContext(NewContextWithInternalRequestID(req.Context(), intReqID))

	logger := logtest.NewRecorder()
	customLogger := logtest.NewRecorder()
	handler := &mockLoggingNextHandler{respStatusCode: http.StatusOK}
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			GetLoggingParamsFromContext(r.Context()).ExtendFields(
				log.String("next_middleware_key_1", "value"),
				log.Int("next_middleware_key_2", 100500),
			)
			next.ServeHTTP(rw, r)
		})
	}
	useCustomLogger := false
	customLoggerProvider := func(_ *http.Request) log.FieldLogger {
		if useCustomLogger {
			return customLogger
		}
		return nil
	}
	resp := httptest.NewRecorder()
	LoggingWithOpts(logger, LoggingOpts{
		CustomLoggerProvider: customLoggerProvider,
	})(mw(handler)).ServeHTTP(resp, req)
	require.Equal(t, 1, handler.called)
	require.NotNil(t, handler.lastContextLogger)
	require.NotNil(t, handler.lastContextLoggingParams)

	require.Empty(t, customLogger.Entries())
	require.Len(t, logger.Entries(), 1)

	useCustomLogger = true
	LoggingWithOpts(logger, LoggingOpts{
		CustomLoggerProvider: customLoggerProvider,
	})(mw(handler)).ServeHTTP(resp, req)
	require.Equal(t, 2, handler.called)

	require.Len(t, customLogger.Entries(), 1)
}

func TestLoggingHandler_ServeHTTP_HeadersOriginAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("payload")))

	tests := []struct {
		name string
		args map[string]string
		want string
	}{
		{
			name: "get origin address from X-Forwarded-For",
			args: map[string]string{headerForwardedFor: "192.0.0.1:1234"},
			want: "192.0.0.1:1234",
		},
		{
			name: "get origin address from X-Forwarded-For many",
			args: map[string]string{headerForwardedFor: "192.0.0.1:1234,192.0.0.2:2345"},
			want: "192.0.0.1:1234",
		},
		{
			name: "get origin address from X-Real-IP",
			args: map[string]string{headerRealIP: "192.0.0.3:4321"},
			want: "192.0.0.3:4321",
		},
		{
			name: "get origin address from X-Forwarded-For instead of X-Real-IP",
			args: map[string]string{
				headerForwardedFor: "192.0.0.4:3456",
				headerRealIP:       "192.0.0.5:6789",
			},
			want: "192.0.0.4:3456",
		},
		{
			name: "no origin address",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.args != nil {
				for k, v := range test.args {
					req.Header.Set(k, v)
				}
			}

			logger := logtest.NewRecorder()
			handler := &mockLoggingNextHandler{respStatusCode: http.StatusOK}
			resp := httptest.NewRecorder()
			LoggingWithOpts(logger, LoggingOpts{})(handler).ServeHTTP(resp, req)
			require.Equal(t, 1, handler.called)
			require.NotNil(t, handler.lastContextLogger)

			entry := logger.Entries()[0]
			requireRemoteAddrIPAndPort(t, entry, req.RemoteAddr)

			if test.want != "" {
				requireLogFieldString(t, entry, "origin_addr", test.want)
			} else {
				_, found := entry.FindField("origin_addr")
				require.False(t, found)
			}

			if test.args != nil {
				for k := range test.args {
					req.Header.Del(k)
				}
			}
		})
	}
}

func requireLogFieldString(t *testing.T, logEntry logtest.RecordedEntry, key, want string) {
	t.Helper()
	logField, found := logEntry.FindField(key)
	require.True(t, found)
	require.Equal(t, want, string(logField.Bytes))
}

func requireLogFieldInt(t *testing.T, logEntry logtest.RecordedEntry, key string, want int) {
	t.Helper()
	logField, found := logEntry.FindField(key)
	require.True(t, found)
	require.Equal(t, want, int(logField.Int))
}

func requireRemoteAddrIPAndPort(t *testing.T, logEntry logtest.RecordedEntry, want string) {
	ipField, found := logEntry.FindField("remote_addr_ip")
	require.True(t, found)

	portField, found := logEntry.FindField("remote_addr_port")
	require.True(t, found)

	got := fmt.Sprintf("%s:%d", string(ipField.Bytes), uint16(portField.Int))
	require.Equal(t, want, got)
}

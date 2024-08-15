/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
	"github.com/acronis/go-appkit/restapi"
)

func TestHealthCheckHandler_ServeHTTP(t *testing.T) {
	t.Run("health-check returns error", func(t *testing.T) {
		var internalError = errors.New("internal error")

		h := NewHealthCheckHandler(func() (HealthCheckResult, error) {
			return nil, internalError
		})
		resp := httptest.NewRecorder()
		logRecorder := logtest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), logRecorder))

		require.Equal(t, http.StatusInternalServerError, resp.Code)
		requireHealthCheckErrorWasLogged(t, logRecorder, internalError)
	})

	t.Run("health-check with empty components", func(t *testing.T) {
		h := NewHealthCheckHandler(func() (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), log.NewDisabledLogger()))

		require.Equal(t, http.StatusOK, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
	})

	t.Run("health-check returns unhealthy components", func(t *testing.T) {
		h := NewHealthCheckHandler(func() (HealthCheckResult, error) {
			return HealthCheckResult{
				"db":   HealthCheckStatusOK,
				"amqp": HealthCheckStatusFail,
			}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), log.NewDisabledLogger()))

		require.Equal(t, http.StatusServiceUnavailable, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
		var gotRespData healthCheckResponseData
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&gotRespData))
		wantRespData := healthCheckResponseData{Components: map[string]bool{"db": true, "amqp": false}}
		require.Equal(t, wantRespData, gotRespData)
	})

	t.Run("health-check returns healthy components", func(t *testing.T) {
		h := NewHealthCheckHandler(func() (HealthCheckResult, error) {
			return HealthCheckResult{
				"db":   HealthCheckStatusOK,
				"amqp": HealthCheckStatusOK,
			}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), log.NewDisabledLogger()))

		require.Equal(t, http.StatusOK, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
		var gotRespData healthCheckResponseData
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&gotRespData))
		wantRespData := healthCheckResponseData{Components: map[string]bool{"db": true, "amqp": true}}
		require.Equal(t, wantRespData, gotRespData)
	})

	t.Run("Default HealthCheck responds error on client cancel", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(nil)
		resp := httptest.NewRecorder()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel context immediately so default handler detects error

		h.ServeHTTP(resp, makeHealthCheckRequest(ctx, log.NewDisabledLogger()))

		require.Equal(t, StatusClientClosedRequest, resp.Code)
	})
}

func TestHealthCheckHandlerContext_ServeHTTP(t *testing.T) {
	t.Run("health-check returns error", func(t *testing.T) {
		var internalError = errors.New("internal error")

		h := NewHealthCheckHandlerContext(func(_ context.Context) (HealthCheckResult, error) {
			return nil, internalError
		})
		resp := httptest.NewRecorder()
		logRecorder := logtest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), logRecorder))

		require.Equal(t, http.StatusInternalServerError, resp.Code)
		requireHealthCheckErrorWasLogged(t, logRecorder, internalError)
	})

	t.Run("health-check with empty components", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(_ context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), log.NewDisabledLogger()))

		require.Equal(t, http.StatusOK, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
	})

	t.Run("health-check returns unhealthy components", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(_ context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{
				"db":   HealthCheckStatusOK,
				"amqp": HealthCheckStatusFail,
			}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), log.NewDisabledLogger()))

		require.Equal(t, http.StatusServiceUnavailable, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
		var gotRespData healthCheckResponseData
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&gotRespData))
		wantRespData := healthCheckResponseData{Components: map[string]bool{"db": true, "amqp": false}}
		require.Equal(t, wantRespData, gotRespData)
	})

	t.Run("health-check returns healthy components", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(_ context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{
				"db":   HealthCheckStatusOK,
				"amqp": HealthCheckStatusOK,
			}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeHealthCheckRequest(context.Background(), log.NewDisabledLogger()))

		require.Equal(t, http.StatusOK, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
		var gotRespData healthCheckResponseData
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&gotRespData))
		wantRespData := healthCheckResponseData{Components: map[string]bool{"db": true, "amqp": true}}
		require.Equal(t, wantRespData, gotRespData)
	})

	t.Run("health-check responds error on client timeout", func(t *testing.T) {
		const requestTimeout = 10 * time.Millisecond

		h := NewHealthCheckHandlerContext(func(ctx context.Context) (HealthCheckResult, error) {
			select {
			case <-ctx.Done():
			case <-time.After(requestTimeout * 2):
			}
			return HealthCheckResult{}, ctx.Err()
		})
		resp := httptest.NewRecorder()
		logRecorder := logtest.NewRecorder()

		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		h.ServeHTTP(resp, makeHealthCheckRequest(ctx, logRecorder))

		require.Equal(t, http.StatusInternalServerError, resp.Code)
		require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		requireHealthCheckErrorWasLogged(t, logRecorder, ctx.Err())
	})

	t.Run("default HealthCheckContext function responds error on client cancel", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(ctx context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, ctx.Err()
		})
		resp := httptest.NewRecorder()
		logRecorder := logtest.NewRecorder()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel context immediately so default handler detects error

		h.ServeHTTP(resp, makeHealthCheckRequest(ctx, logRecorder))

		require.Equal(t, StatusClientClosedRequest, resp.Code)
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		requireHealthCheckErrorWasLogged(t, logRecorder, ctx.Err())
	})

	t.Run("HealthCheckContext that doesn't return ctx.Err() responds error on client cancel", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(ctx context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		})
		resp := httptest.NewRecorder()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel context immediately so default handler detects error

		ctx = middleware.NewContextWithLogger(ctx, log.NewDisabledLogger())
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		h.ServeHTTP(resp, req)

		require.Equal(t, StatusClientClosedRequest, resp.Code)
	})
}

func makeHealthCheckRequest(ctx context.Context, logger log.FieldLogger) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	return req.WithContext(middleware.NewContextWithLogger(ctx, logger))
}

func requireHealthCheckErrorWasLogged(t *testing.T, logRecorder *logtest.Recorder, wantErr error) {
	t.Helper()
	loggedEntries := logRecorder.Entries()
	require.Len(t, loggedEntries, 1)
	require.Equal(t, log.LevelError, loggedEntries[0].Level)
	require.Contains(t, loggedEntries[0].Text, "error while checking health")
	loggedErrorField, found := loggedEntries[0].FindField("error")
	require.True(t, found)
	require.Equal(t, wantErr, loggedErrorField.Any)
}

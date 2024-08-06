/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/httpserver/middleware"
	"git.acronis.com/abc/go-libs/v2/log"
	"git.acronis.com/abc/go-libs/v2/restapi"
)

func TestHealthCheckHandler_ServeHTTP(t *testing.T) {
	makeRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		return req.WithContext(middleware.NewContextWithLogger(req.Context(), log.NewDisabledLogger()))
	}

	t.Run("health-check returns error", func(t *testing.T) {
		h := NewHealthCheckHandler(func() (HealthCheckResult, error) {
			return nil, fmt.Errorf("internal error")
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeRequest())

		require.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("health-check with empty components", func(t *testing.T) {
		h := NewHealthCheckHandler(func() (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeRequest())

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

		h.ServeHTTP(resp, makeRequest())

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

		h.ServeHTTP(resp, makeRequest())

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
		// cancel context immediately so default handler detects error
		cancel()

		ctx = middleware.NewContextWithLogger(ctx, log.NewDisabledLogger())
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		h.ServeHTTP(resp, req)

		require.Equal(t, StatusClientClosedRequest, resp.Code)
	})
}

func TestHealthCheckHandlerContext_ServeHTTP(t *testing.T) {
	makeRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		return req.WithContext(middleware.NewContextWithLogger(req.Context(), log.NewDisabledLogger()))
	}

	t.Run("health-check returns error", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(_ context.Context) (HealthCheckResult, error) {
			return nil, fmt.Errorf("internal error")
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeRequest())

		require.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("health-check with empty components", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(_ context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		})
		resp := httptest.NewRecorder()

		h.ServeHTTP(resp, makeRequest())

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

		h.ServeHTTP(resp, makeRequest())

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

		h.ServeHTTP(resp, makeRequest())

		require.Equal(t, http.StatusOK, resp.Code)
		require.Equal(t, restapi.ContentTypeAppJSON, resp.Header().Get("Content-Type"))
		var gotRespData healthCheckResponseData
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&gotRespData))
		wantRespData := healthCheckResponseData{Components: map[string]bool{"db": true, "amqp": true}}
		require.Equal(t, wantRespData, gotRespData)
	})

	t.Run("health-check responds error on client timeout", func(t *testing.T) {
		timeout := 1 * time.Millisecond

		h := NewHealthCheckHandlerContext(func(ctx context.Context) (HealthCheckResult, error) {
			time.Sleep(timeout + 1*time.Millisecond)
			return HealthCheckResult{}, ctx.Err()
		})
		resp := httptest.NewRecorder()

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		ctx = middleware.NewContextWithLogger(ctx, log.NewDisabledLogger())
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		h.ServeHTTP(resp, req)

		require.Equal(t, http.StatusInternalServerError, resp.Code)
	})

	t.Run("default HealthCheckContext function responds error on client cancel", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(ctx context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, ctx.Err()
		})
		resp := httptest.NewRecorder()

		ctx, cancel := context.WithCancel(context.Background())
		// cancel context immediately so default handler detects error
		cancel()

		ctx = middleware.NewContextWithLogger(ctx, log.NewDisabledLogger())
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		h.ServeHTTP(resp, req)

		require.Equal(t, StatusClientClosedRequest, resp.Code)
	})

	t.Run("HealthCheckContext that doesn't return ctx.Err() responds error on client cancel", func(t *testing.T) {
		h := NewHealthCheckHandlerContext(func(ctx context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		})
		resp := httptest.NewRecorder()

		ctx, cancel := context.WithCancel(context.Background())
		// cancel context immediately so default handler detects error
		cancel()

		ctx = middleware.NewContextWithLogger(ctx, log.NewDisabledLogger())
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

		h.ServeHTTP(resp, req)

		require.Equal(t, StatusClientClosedRequest, resp.Code)
	})
}

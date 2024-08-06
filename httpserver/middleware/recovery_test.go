/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/log"
	"git.acronis.com/abc/go-libs/v2/log/logtest"
	"git.acronis.com/abc/go-libs/v2/restapi"
	"git.acronis.com/abc/go-libs/v2/testutil"
)

type mockRecoveryNextHandler struct {
	called     int
	panicValue interface{}
}

func (h *mockRecoveryNextHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.called++
	if h.panicValue != nil {
		panic(h.panicValue)
	}
	panic("test")
}

func TestRecoveryHandler_ServeHTTP(t *testing.T) {
	const errDomain = "MainDomain"

	t.Run("recovery w/o logging", func(t *testing.T) {
		next := &mockRecoveryNextHandler{}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		resp := httptest.NewRecorder()
		handler := Recovery(errDomain)(next)

		require.NotPanics(t, func() { handler.ServeHTTP(resp, req) })

		require.Equal(t, 1, next.called)
		testutil.RequireErrorInRecorder(t, resp, http.StatusInternalServerError, errDomain, restapi.ErrCodeInternal)
	})

	t.Run("recovery with logging", func(t *testing.T) {
		const stackSize = 10

		next := &mockRecoveryNextHandler{}

		logger := logtest.NewRecorder()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(NewContextWithLogger(req.Context(), logger))
		resp := httptest.NewRecorder()
		handler := RecoveryWithOpts(errDomain, RecoveryOpts{StackSize: stackSize})(next)

		require.NotPanics(t, func() { handler.ServeHTTP(resp, req) })
		require.Equal(t, 1, next.called)
		testutil.RequireErrorInRecorder(t, resp, http.StatusInternalServerError, errDomain, restapi.ErrCodeInternal)

		mockEntry, found := logger.FindEntry("Panic: test")
		require.True(t, found)
		require.Equal(t, log.LevelError, mockEntry.Level)
		logField, found := mockEntry.FindField("stack")
		require.True(t, found)
		require.Equal(t, stackSize, len(logField.Bytes))
	})

	t.Run("recovery with logging, http.ErrAbortHandler", func(t *testing.T) {
		next := &mockRecoveryNextHandler{panicValue: http.ErrAbortHandler}

		logger := logtest.NewRecorder()

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = req.WithContext(NewContextWithLogger(req.Context(), logger))
		resp := httptest.NewRecorder()
		handler := Recovery(errDomain)(next)

		require.Panics(t, func() { handler.ServeHTTP(resp, req) })
		require.Equal(t, 1, next.called)

		_, found := logger.FindEntry("Panic:")
		require.False(t, found)

		mockEntry, found := logger.FindEntry("request has been aborted")
		require.True(t, found)
		require.Equal(t, log.LevelWarn, mockEntry.Level)
	})
}

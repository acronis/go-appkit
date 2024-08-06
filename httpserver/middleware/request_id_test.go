/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockRequestIDNextHandler struct {
	called  int
	request *http.Request
}

func (h *mockRequestIDNextHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.called++
	h.request = r
}

func TestRequestIDHandler_ServeHTTP(t *testing.T) {
	const genExtReqID = "generated-external-request-id"
	const genIntReqID = "generated-internal-request-id"

	reqIDOpts := RequestIDOpts{
		GenerateID:         func() string { return genExtReqID },
		GenerateInternalID: func() string { return genIntReqID },
	}

	t.Run("use external requestID from request", func(t *testing.T) {
		const headerReqID = "header-request-id"
		next := &mockRequestIDNextHandler{}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(headerRequestID, headerReqID)
		req.Header.Set(headerInternalRequestID, headerReqID)
		h := RequestIDWithOpts(reqIDOpts)(next)
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)

		assert.Equal(t, 1, next.called)
		assert.Equal(t, headerReqID, GetRequestIDFromContext(next.request.Context()))
		assert.Equal(t, headerReqID, resp.Header().Get(headerRequestID))
		assert.Equal(t, genIntReqID, GetInternalRequestIDFromContext(next.request.Context()))
		assert.Equal(t, genIntReqID, resp.Header().Get(headerInternalRequestID))
	})

	t.Run("generate new external requestID", func(t *testing.T) {
		next := &mockRequestIDNextHandler{}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h := RequestIDWithOpts(reqIDOpts)(next)
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)

		assert.Equal(t, 1, next.called)
		assert.Equal(t, genExtReqID, GetRequestIDFromContext(next.request.Context()))
		assert.Equal(t, genExtReqID, resp.Header().Get(headerRequestID))
		assert.Equal(t, genIntReqID, GetInternalRequestIDFromContext(next.request.Context()))
		assert.Equal(t, genIntReqID, resp.Header().Get(headerInternalRequestID))
	})

	t.Run("generate external and internal requestIDs using default (xid) alg", func(t *testing.T) {
		next := &mockRequestIDNextHandler{}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h := RequestID()(next)
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)

		assert.Equal(t, 1, next.called)
		assert.Greater(t, len(GetRequestIDFromContext(next.request.Context())), 0)
		assert.Greater(t, len(resp.Header().Get(headerRequestID)), 0)
		assert.Greater(t, len(GetInternalRequestIDFromContext(next.request.Context())), 0)
		assert.Greater(t, len(resp.Header().Get(headerInternalRequestID)), 0)
	})
}

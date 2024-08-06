/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-libs/restapi"
)

type mockRequestBodyLimitNextHandler struct {
	called int
}

func (h *mockRequestBodyLimitNextHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.called++

	if _, err := io.ReadAll(r.Body); err != nil {
		var reqTooLargeErr *restapi.RequestBodyTooLargeError
		if errors.As(err, &reqTooLargeErr) {
			rw.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}

		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func TestRequestBodyLimitHandler_ServeHTTP(t *testing.T) {
	type testData struct {
		ReqBodyMaxSize    uint64
		ReqBody           string
		SendContentLength bool
		WantRespHTTPCode  int
	}

	tests := []testData{
		{
			ReqBodyMaxSize:    32,
			ReqBody:           strings.Repeat("a", 64),
			SendContentLength: true,
			WantRespHTTPCode:  http.StatusRequestEntityTooLarge,
		},
		{
			ReqBodyMaxSize:   32,
			ReqBody:          strings.Repeat("a", 10),
			WantRespHTTPCode: http.StatusOK,
		},
		{
			ReqBodyMaxSize:   32,
			ReqBody:          strings.Repeat("a", 32),
			WantRespHTTPCode: http.StatusOK,
		},
		{
			ReqBodyMaxSize:   32,
			ReqBody:          strings.Repeat("a", 33),
			WantRespHTTPCode: http.StatusRequestEntityTooLarge,
		},
		{
			ReqBodyMaxSize:   0,
			ReqBody:          "a",
			WantRespHTTPCode: http.StatusRequestEntityTooLarge,
		},
		{
			ReqBodyMaxSize:   0,
			ReqBody:          "",
			WantRespHTTPCode: http.StatusOK,
		},
	}

	for _, test := range tests {
		next := &mockRequestBodyLimitNextHandler{}

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(test.ReqBody))
		if !test.SendContentLength {
			req.ContentLength = -1
		}
		h := RequestBodyLimit(test.ReqBodyMaxSize, "MyService")(next)
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)

		wantNextCalled := 1
		if test.SendContentLength && test.WantRespHTTPCode == http.StatusRequestEntityTooLarge {
			wantNextCalled = 0
		}
		require.Equal(t, wantNextCalled, next.called)

		require.Equal(t, test.WantRespHTTPCode, resp.Code)
	}
}

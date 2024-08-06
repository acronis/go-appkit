/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/stretchr/testify/require"
)

const contentTypeAppJSON = "application/json"

type errorRespData struct {
	Domain string `json:"domain"`
	Code   string `json:"code"`
}

type wrappedErrorRespData struct {
	Error errorRespData `json:"error"`
}

var (
	requireWrappedErrorInResponse   = makeRequireWrappedErrorInResponse(true)
	requireNoWrappedErrorInResponse = makeRequireWrappedErrorInResponse(false)
	requireErrorInResponse          = requireWrappedErrorInResponse
)

// DisableWrappingErrorInResponse disables expecting wrapped error ({"error": {"domain": "{domain}", ...} -> {"domain": "{domain}", ...})
// in response body.
func DisableWrappingErrorInResponse() {
	requireErrorInResponse = requireNoWrappedErrorInResponse
}

// EnableWrappingErrorInResponse enabled expecting wrapped error ({"domain": "{domain}", ...} -> {"error": {"domain": "{domain}", ...})
// in response body.
func EnableWrappingErrorInResponse() {
	requireErrorInResponse = requireWrappedErrorInResponse
}

// RequireErrorInRecorder asserts that passing httptest.ResponseRecorder contains error.
func RequireErrorInRecorder(t require.TestingT, resp *httptest.ResponseRecorder, wantHTTPCode int, wantErrDomain, wantErrCode string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireErrorInResponse(t, resp.Code, resp.Header(), resp.Body, wantHTTPCode, wantErrDomain, wantErrCode)
}

// RequireErrorInResponse asserts that passing http.Response contains the error.
func RequireErrorInResponse(t require.TestingT, resp *http.Response, wantHTTPCode int, wantErrDomain, wantErrCode string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireErrorInResponse(t, resp.StatusCode, resp.Header, resp.Body, wantHTTPCode, wantErrDomain, wantErrCode)
}

// RequireWrappedErrorInRecorder asserts that passing httptest.ResponseRecorder contains wrapped error.
func RequireWrappedErrorInRecorder(
	t require.TestingT, resp *httptest.ResponseRecorder, wantHTTPCode int, wantErrDomain, wantErrCode string,
) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireWrappedErrorInResponse(t, resp.Code, resp.Header(), resp.Body, wantHTTPCode, wantErrDomain, wantErrCode)
}

// RequireWrappedErrorInResponse asserts that passing http.Response contains the wrapped error.
func RequireWrappedErrorInResponse(t require.TestingT, resp *http.Response, wantHTTPCode int, wantErrDomain, wantErrCode string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireWrappedErrorInResponse(t, resp.StatusCode, resp.Header, resp.Body, wantHTTPCode, wantErrDomain, wantErrCode)
}

// RequireNoWrappedErrorInRecorder asserts that passing httptest.ResponseRecorder contains no wrapped error.
func RequireNoWrappedErrorInRecorder(
	t require.TestingT, resp *httptest.ResponseRecorder, wantHTTPCode int, wantErrDomain, wantErrCode string,
) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireNoWrappedErrorInResponse(t, resp.Code, resp.Header(), resp.Body, wantHTTPCode, wantErrDomain, wantErrCode)
}

// RequireNoWrappedErrorInResponse asserts that passing http.Response contains the no wrapped error.
func RequireNoWrappedErrorInResponse(t require.TestingT, resp *http.Response, wantHTTPCode int, wantErrDomain, wantErrCode string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireNoWrappedErrorInResponse(t, resp.StatusCode, resp.Header, resp.Body, wantHTTPCode, wantErrDomain, wantErrCode)
}

func makeRequireWrappedErrorInResponse(wrap bool) func(
	t require.TestingT, code int, header http.Header, body io.Reader, wantHTTPCode int, wantErrDomain, wantErrCode string,
) {
	return func(t require.TestingT, code int, header http.Header, body io.Reader, wantHTTPCode int, wantErrDomain, wantErrCode string) {
		if h, ok := t.(tHelper); ok {
			h.Helper()
		}
		require.Equal(t, wantHTTPCode, code)
		require.Equal(t, contentTypeAppJSON, header.Get("Content-Type"))
		var errResp errorRespData
		if wrap {
			var wrappedErrResp wrappedErrorRespData
			require.NoError(t, json.NewDecoder(body).Decode(&wrappedErrResp))
			errResp = wrappedErrResp.Error
		} else {
			require.NoError(t, json.NewDecoder(body).Decode(&errResp))
		}
		require.Equal(t, wantErrDomain, errResp.Domain)
		require.Equal(t, wantErrCode, errResp.Code)
	}
}

// RequireEmptyBodyInRecorder asserts that passing httptest.ResponseRecorder contains empty body.
func RequireEmptyBodyInRecorder(t require.TestingT, resp *httptest.ResponseRecorder) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireEmptyBodyInResponse(t, resp.Body)
}

// RequireEmptyBodyInResponse asserts that passing http.Responsecontains empty body.
func RequireEmptyBodyInResponse(t require.TestingT, resp *http.Response) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireEmptyBodyInResponse(t, resp.Body)
}

func requireEmptyBodyInResponse(t require.TestingT, body io.Reader) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	bodyBytes, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, 0, len(bodyBytes))
}

// RequireJSONInRecorder asserts that passing httptest.ResponseRecorder contains the data in json format.
func RequireJSONInRecorder(t require.TestingT, resp *httptest.ResponseRecorder, want, dest interface{}) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireJSONInResponse(t, resp.Header(), resp.Body, want, dest)
}

// RequireJSONInResponse asserts that passing http.Response contains the data in json format.
func RequireJSONInResponse(t require.TestingT, resp *http.Response, want, dest interface{}) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireJSONInResponse(t, resp.Header, resp.Body, want, dest)
}

func requireJSONInResponse(t require.TestingT, header http.Header, body io.Reader, want, dest interface{}) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	require.Equal(t, contentTypeAppJSON, header.Get("Content-Type"))
	bodyBytes, err := io.ReadAll(body)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bodyBytes, dest))
	require.Equal(t, want, dest)
}

// RequireStringJSONInRecorder asserts that passing httptest.ResponseRecorder contains the json string.
func RequireStringJSONInRecorder(t require.TestingT, resp *httptest.ResponseRecorder, want string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireStringJSONInResponse(t, resp.Header(), resp.Body, want)
}

// RequireStringJSONInResponse asserts that passing http.Response contains the json string.
func RequireStringJSONInResponse(t require.TestingT, resp *http.Response, want string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	requireStringJSONInResponse(t, resp.Header, resp.Body, want)
}

func requireStringJSONInResponse(t require.TestingT, header http.Header, body io.Reader, want string) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	require.Equal(t, contentTypeAppJSON, header.Get("Content-Type"))
	bodyBytes, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, want, string(bodyBytes))
}

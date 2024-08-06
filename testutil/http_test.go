/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type ErrorType struct {
	Error struct {
		Domain string
		Code   string
	}
}

var errorTests = []struct {
	Name             string
	RespCode         int
	RespBody         string
	RespContentType  string
	RequireCode      int
	RequireErrDomain string
	RequireErrCode   string
	WantFailed       bool
}{
	{
		Name:             "ok",
		RespCode:         404,
		RespContentType:  contentTypeAppJSON,
		RespBody:         `{"error":{"domain":"MyService","code":"notFound"}}`,
		RequireCode:      404,
		RequireErrDomain: "MyService",
		RequireErrCode:   "notFound",
		WantFailed:       false,
	},
	{
		Name:             "invalid code",
		RespCode:         400,
		RespContentType:  contentTypeAppJSON,
		RespBody:         `{"error":{"domain":"MyService","code":"notFound"}}`,
		RequireCode:      404,
		RequireErrDomain: "MyService",
		RequireErrCode:   "notFound",
		WantFailed:       true,
	},
	{
		Name:             "invalid code",
		RespCode:         404,
		RespContentType:  "text/html",
		RespBody:         `{"error":{"domain":"MyService","code":"notFound"}}`,
		RequireCode:      404,
		RequireErrDomain: "MyService",
		RequireErrCode:   "notFound",
		WantFailed:       true,
	},
	{
		Name:             "invalid err domain",
		RespCode:         404,
		RespContentType:  contentTypeAppJSON,
		RespBody:         `{"error":{"domain":"NotMyService","code":"notFound"}}`,
		RequireCode:      404,
		RequireErrDomain: "MyService",
		RequireErrCode:   "notFound",
		WantFailed:       true,
	},

	{
		Name:             "invalid err code",
		RespCode:         404,
		RespContentType:  contentTypeAppJSON,
		RespBody:         `{"error":{"domain":"MyService","code":"otherError"}}`,
		RequireCode:      404,
		RequireErrDomain: "MyService",
		RequireErrCode:   "notFound",
		WantFailed:       true,
	},
}

func TestRequireErrorInRecorder(t *testing.T) {
	for i := range errorTests {
		tt := errorTests[i]
		t.Run(tt.Name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("Content-Type", tt.RespContentType)
			rec.WriteHeader(tt.RespCode)
			_, _ = rec.Write([]byte(tt.RespBody))
			mockT := &MockT{}
			RequireErrorInRecorder(mockT, rec, tt.RequireCode, tt.RequireErrDomain, tt.RequireErrCode)
			require.Equal(t, tt.WantFailed, mockT.Failed)
		})
	}
}

func TestRequireErrorInResponse(t *testing.T) {
	for i := range errorTests {
		tt := errorTests[i]
		t.Run(tt.Name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.Header().Set("Content-Type", tt.RespContentType)
				rw.WriteHeader(tt.RespCode)
				_, _ = rw.Write([]byte(tt.RespBody))
			}))
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			require.NoError(t, err)

			mockT := &MockT{}
			RequireErrorInResponse(mockT, resp, tt.RequireCode, tt.RequireErrDomain, tt.RequireErrCode)
			require.Equal(t, tt.WantFailed, mockT.Failed)
			require.NoError(t, resp.Body.Close())
		})
	}
}

var jsonErrTests = []struct {
	Name            string
	RespBody        string
	WantRespBody    string
	RespContentType string
	WantFailed      bool
}{
	{
		Name:            "invalid content type",
		RespContentType: "text/html",
		RespBody:        `{"error":{"domain":"MyService","code":"notFound"}}`,
		WantRespBody:    `{"error":{"domain":"MyService","code":"notFound"}}`,
		WantFailed:      true,
	},
	{
		Name:            "valid JSON",
		RespContentType: contentTypeAppJSON,
		RespBody:        `{"error":{"domain":"NotMyService","code":"notFound"}}`,
		WantRespBody:    `{"error":{"domain":"NotMyService","code":"notFound"}}`,
		WantFailed:      false,
	},
	{
		Name:            "invalid JSON",
		RespContentType: contentTypeAppJSON,
		RespBody:        `{"error":"domain":"NotMyService","code":"notFound"}}`,
		WantRespBody:    `{"error":{"domain":"NotMyService","code":"notFound"}}`,
		WantFailed:      true,
	},
	{
		Name:            "unexpected JSON",
		RespContentType: contentTypeAppJSON,
		RespBody:        `{"error":"domain":"NotMyService","code":"notFound"}}`,
		WantRespBody:    `{"error":{"domain":"Foo","code":"bar"}}`,
		WantFailed:      true,
	},
}

func TestRequireJSONInRecorder(t *testing.T) {
	for i := range jsonErrTests {
		tt := jsonErrTests[i]
		t.Run(tt.Name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rec.Header().Set("Content-Type", tt.RespContentType)
			rec.WriteHeader(http.StatusOK)
			_, _ = rec.Write([]byte(tt.RespBody))

			var want, dest ErrorType
			require.NoError(t, json.Unmarshal([]byte(tt.WantRespBody), &want))

			mockT := &MockT{}
			RequireJSONInRecorder(mockT, rec, &want, dest)
		})
	}
}

func TestRequireJSONInResponse(t *testing.T) {
	for i := range jsonErrTests {
		tt := jsonErrTests[i]
		t.Run(tt.Name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.Header().Set("Content-Type", tt.RespContentType)
				rw.WriteHeader(http.StatusOK)
				_, _ = rw.Write([]byte(tt.RespBody))
			}))
			defer srv.Close()

			resp, err := http.Get(srv.URL)
			require.NoError(t, err)

			var want, dest ErrorType
			require.NoError(t, json.Unmarshal([]byte(tt.WantRespBody), &want))

			mockT := &MockT{}
			RequireJSONInResponse(mockT, resp, &want, &dest)
			require.Equal(t, tt.WantFailed, mockT.Failed)
			require.NoError(t, resp.Body.Close())
		})
	}
}

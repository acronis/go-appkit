/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeRequestJSON(t *testing.T) {
	type testData struct {
		ReqContentType        string
		ReqBody               string
		ReqMaxBodySize        uint64
		ResErr                error
		DisallowUnknownFields bool
	}

	type person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	tests := []testData{
		{
			ReqContentType: "",
			ReqBody:        "text",
			ResErr:         &MalformedRequestError{http.StatusBadRequest, "Request body contains badly-formed JSON (at position 2)."},
		},
		{
			ReqContentType: "text/html",
			ReqBody:        "text",
			ResErr:         &MalformedRequestError{http.StatusUnsupportedMediaType, `Content-Type "text/html" is not supported.`},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        "",
			ResErr:         &MalformedRequestError{http.StatusBadRequest, "Request body must not be empty."},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"name":"John"`,
			ResErr:         &MalformedRequestError{http.StatusBadRequest, "Request body contains badly-formed JSON."},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"name":John`,
			ResErr:         &MalformedRequestError{http.StatusBadRequest, "Request body contains badly-formed JSON (at position 9)."},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"name":[]}`,
			ResErr: &MalformedRequestError{
				http.StatusBadRequest,
				`Request body contains an invalid value for the "name" field (at position 9).`,
			},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"name":"malicious request with very large body","age":10}`,
			ReqMaxBodySize: 20,
			ResErr:         &MalformedRequestError{http.StatusRequestEntityTooLarge, "Request body must not be larger than 20B."},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"name":"John","age":10}{"name":"Bob","age":20}`,
			ResErr:         &MalformedRequestError{http.StatusBadRequest, "Request body must only contain a single JSON object."},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"name":"John","age":10}`,
			ResErr:         nil,
		},
		{
			ReqContentType: ContentTypeAppSCIMJSON,
			ReqBody:        `{"name":"John","age":10}`,
			ResErr:         nil,
		},
		{
			ReqContentType: "invalid content type",
			ReqBody:        `{"name":"John","age":10}`,
			ResErr: &MalformedRequestError{
				http.StatusUnsupportedMediaType,
				"failed to parse Content-Type header for request: mime: expected slash after first token",
			},
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"login":"John","password":10}`,
			ResErr:         nil,
		},
		{
			ReqContentType: ContentTypeAppJSON,
			ReqBody:        `{"login":"John","password":10}`,
			ResErr: &MalformedRequestError{
				http.StatusBadRequest,
				"Payload does not match the scheme",
			},
			DisallowUnknownFields: true,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(fmt.Sprintf("test #%d", i), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tt.ReqBody))
			req.Header.Set("Content-Type", tt.ReqContentType)
			if tt.ReqMaxBodySize != 0 {
				SetRequestMaxBodySize(nil, req, tt.ReqMaxBodySize)
			}
			var p person
			var err error
			if tt.DisallowUnknownFields {
				err = DecodeRequestJSONStrict(req, &p, true)
			} else {
				err = DecodeRequestJSON(req, &p)
			}
			assert.Equal(t, tt.ResErr, err)
		})
	}
}

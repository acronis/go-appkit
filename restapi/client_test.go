/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

// Package client provide default client that can be used for do requests
package restapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"

	"github.com/acronis/go-libs/log"
	"github.com/acronis/go-libs/log/logtest"

	"github.com/stretchr/testify/assert"
)

func TestNewJSONRequest(t *testing.T) {
	requestData := &struct {
		Name string
	}{
		Name: "request",
	}
	buf, err := json.Marshal(requestData)
	assert.NoError(t, err)
	expectedReqest, err := http.NewRequest(http.MethodPost, "/", bytes.NewReader(buf))
	assert.NoError(t, err)
	expectedReqest.Header.Set("Content-Type", ContentTypeAppJSON)

	tests := []struct {
		name string

		method  string
		address string
		data    interface{}

		req *http.Request
		err bool
	}{
		{
			name: "Nil data",
			data: nil,

			err: true,
			req: nil,
		},
		{
			name:   "Method not allowed",
			method: http.MethodDelete,
			data:   requestData,

			err: true,
			req: nil,
		},
		{
			name:    "Valid request",
			method:  http.MethodPost,
			address: "/",
			data:    requestData,

			err: false,
			req: expectedReqest,
		},
	}

	for i := range tests {
		tt := tests[i]
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewJSONRequest(tt.method, tt.address, tt.data)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.req != nil {
				assert.Equal(t, tt.req.Body, req.Body)
				assert.Equal(t, tt.req.Header.Get("Content-Type"), ContentTypeAppJSON)
			}
		})
	}
}

func TestDoRequest(t *testing.T) {
	requestData := []byte(`{"type":"request"}`)
	responseData := []byte(`{"type":"response"}`)
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, requestData, buf)
		_, err = rw.Write(responseData)
		assert.NoError(t, err)
		rw.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	client := &http.Client{
		Transport: http.DefaultTransport,
	}
	req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBuffer(requestData))
	assert.NoError(t, err)
	resp, err := DoRequest(client, req, logger)
	assert.NoError(t, err)
	buf, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.NoError(t, resp.Body.Close())
	assert.Equal(t, responseData, buf)
}

func TestDoRequestAndUnmarshalJSON(t *testing.T) {
	type RequestData struct {
		ContentType string
		StatusCode  int
		Data        interface{}
		Text        string
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		req := &RequestData{}
		err = json.Unmarshal(buf, req)
		assert.NoError(t, err)
		var respData []byte
		if req.Text != "" {
			respData = []byte(req.Text)
		} else {
			respData, err = json.Marshal(req.Data)
			assert.NoError(t, err)
		}
		contentType := req.ContentType
		if contentType == "" {
			contentType = "application/json"
		}
		rw.Header().Set("Content-Type", contentType)
		rw.WriteHeader(req.StatusCode)
		_, err = rw.Write(respData)
		assert.NoError(t, err)
	}))
	defer server.Close()

	logger := logtest.NewRecorder()
	client := &http.Client{
		Transport: http.DefaultTransport,
	}

	t.Run("success", func(t *testing.T) {
		type ResponseData struct {
			Name string
		}
		reqData := RequestData{
			StatusCode: 200,
			Data: ResponseData{
				Name: "success",
			},
		}
		buf, err := json.Marshal(reqData)
		assert.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBuffer(buf))
		assert.NoError(t, err)
		gotResp := ResponseData{}
		err = DoRequestAndUnmarshalJSON(client, req, &gotResp, logger)
		assert.NoError(t, err)
		assert.Equal(t, reqData.Data, gotResp)
	})

	t.Run("success with malformed response", func(t *testing.T) {
		type ResponseData struct {
			Name string
		}
		reqData := RequestData{
			StatusCode: 200,
			Text:       "|",
		}
		buf, err := json.Marshal(reqData)
		assert.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBuffer(buf))
		assert.NoError(t, err)
		var gotResp *ResponseData
		err = DoRequestAndUnmarshalJSON(client, req, gotResp, logger)
		assert.Error(t, err)
		var clientError *ClientError
		assert.ErrorAs(t, err, &clientError)
		assert.Equal(t, reqData.StatusCode, clientError.StatusCode)
		assert.ErrorContains(t, clientError.Err, "invalid character")
	})

	t.Run("ErrorResponseData with code 400", func(t *testing.T) {
		resp := &ErrorResponseData{
			Err: &Error{
				Domain: "go-libs",
			},
		}
		reqData := RequestData{
			StatusCode: 400,
			Data:       resp,
		}
		buf, err := json.Marshal(reqData)
		assert.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBuffer(buf))
		assert.NoError(t, err)
		var gotResp *ErrorResponseData
		err = DoRequestAndUnmarshalJSON(client, req, gotResp, logger)
		var clientError *ClientError
		assert.Error(t, err)
		assert.ErrorAs(t, err, &clientError)
		assert.Equal(t, reqData.Data, clientError.Err)
	})
	t.Run("ErrorResponseData with code 403 and text", func(t *testing.T) {
		reqData := RequestData{
			ContentType: "text/plain",
			StatusCode:  403,
			Text:        "some text that should explain the error",
		}
		buf, err := json.Marshal(reqData)
		assert.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBuffer(buf))
		assert.NoError(t, err)
		var gotResp *ErrorResponseData
		err = DoRequestAndUnmarshalJSON(client, req, gotResp, logger)
		var clientError *ClientError
		var restError *ErrorResponseData
		assert.Error(t, err)
		assert.ErrorAs(t, err, &clientError)
		assert.Equal(t, reqData.StatusCode, clientError.StatusCode)
		assert.ErrorAs(t, clientError.Err, &restError)
		assert.Equal(t, reqData.Text, restError.Err.Debug["body"].(string))
	})
	t.Run("ErrorResponseData with code 403 and malformed JSON", func(t *testing.T) {
		reqData := RequestData{
			StatusCode: 403,
			Text:       "|",
		}
		buf, err := json.Marshal(reqData)
		assert.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, server.URL+"/", bytes.NewBuffer(buf))
		assert.NoError(t, err)
		var gotResp *ErrorResponseData
		err = DoRequestAndUnmarshalJSON(client, req, gotResp, logger)
		var clientError *ClientError
		assert.Error(t, err)
		assert.ErrorAs(t, err, &clientError)
		assert.Equal(t, reqData.StatusCode, clientError.StatusCode)
		assert.ErrorContains(t, clientError.Err, "invalid character")
	})
}

func Test_readResponseBody(t *testing.T) {
	type args struct {
		resp   *http.Response
		logger log.FieldLogger
		e      *ClientError
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "test error in reader",
			args: args{
				resp: &http.Response{
					Body: io.NopCloser(iotest.ErrReader(errors.New("some error"))),
				},
				logger: logtest.NewRecorder(),
				e:      &ClientError{},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "no content",
			args: args{
				resp: &http.Response{
					Body: io.NopCloser(bytes.NewReader([]byte{})),
				},
				logger: logtest.NewRecorder(),
				e:      &ClientError{},
			},
			want:    nil,
			wantErr: assert.Error,
		},
		{
			name: "with content",
			args: args{
				resp: &http.Response{
					Body: io.NopCloser(bytes.NewReader([]byte{'{', '}'})),
				},
				logger: logtest.NewRecorder(),
				e:      &ClientError{},
			},
			want:    []byte{'{', '}'},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readResponseBody(tt.args.resp, tt.args.logger, tt.args.e)
			if !tt.wantErr(t, err, fmt.Sprintf("readResponseBody(%v, %v, %v)", tt.args.resp, tt.args.logger, tt.args.e)) {
				return
			}
			assert.Equalf(t, tt.want, got, "readResponseBody(%v, %v, %v)", tt.args.resp, tt.args.logger, tt.args.e)
		})
	}
}

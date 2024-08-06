/*
Copyright © 2019-2024 Acronis International GmbH.
*/

package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/acronis/go-libs/log"
	"github.com/acronis/go-libs/log/logtest"
	"github.com/acronis/go-libs/testutil"
)

const testDomain = "TestDomain"

type responseRecorderReturnedErrorOnWrite struct {
	*httptest.ResponseRecorder
}

func (rw *responseRecorderReturnedErrorOnWrite) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("error on write")
}

func TestRespondJSON(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		resp := httptest.NewRecorder()
		logger := logtest.NewRecorder()
		p := &Person{"Bob", 12}
		require.Empty(t, resp.Header().Get("Content-Type"))
		RespondJSON(resp, p, logger)
		testutil.RequireJSONInRecorder(t, resp, p, &Person{})
		require.Equal(t, 0, len(logger.Entries()))
		require.Equal(t, ContentTypeAppJSON, resp.Header().Get("Content-Type"))
	})

	t.Run("marshaling error", func(t *testing.T) {
		var resp *httptest.ResponseRecorder

		// Without logging.
		resp = httptest.NewRecorder()
		RespondJSON(resp, make(chan bool), nil)
		require.Equal(t, http.StatusInternalServerError, resp.Code)
		testutil.RequireEmptyBodyInRecorder(t, resp)

		// With logging.
		resp = httptest.NewRecorder()
		logger := logtest.NewRecorder()
		RespondJSON(resp, make(chan bool), logger)
		require.Equal(t, http.StatusInternalServerError, resp.Code)
		testutil.RequireEmptyBodyInRecorder(t, resp)
		require.Equal(t, 1, len(logger.Entries()))
		require.Equal(t, log.LevelError, logger.Entries()[0].Level)
	})

	t.Run("writing error", func(t *testing.T) {
		resp := &responseRecorderReturnedErrorOnWrite{httptest.NewRecorder()}
		logger := logtest.NewRecorder()
		RespondJSON(resp, "foo", logger)
		require.Equal(t, 1, len(logger.Entries()))
		require.Equal(t, log.LevelError, logger.Entries()[0].Level)
	})

	t.Run("change Content-Type", func(t *testing.T) {
		resp := httptest.NewRecorder()
		logger := logtest.NewRecorder()
		resp.Header().Set("Content-Type", "something completely different")
		RespondJSON(resp, "nothing", logger)
		require.Equal(t, 0, len(logger.Entries()))
		require.Equal(t, "something completely different", resp.Header().Get("Content-Type"))
	})
}

func TestRespondError(t *testing.T) {
	var resp *httptest.ResponseRecorder

	tests := []struct {
		name           string
		httpStatusCode int
		apiErr         *Error
		useLogger      bool
	}{
		{
			name:           "Without logging",
			httpStatusCode: http.StatusInternalServerError,
			apiErr:         NewInternalError("serviceA"),
			useLogger:      false,
		},
		{
			name:           "With logging",
			httpStatusCode: http.StatusBadRequest,
			apiErr:         NewError("serviceB", "errCode", "Error message."),
			useLogger:      true,
		},
		{
			name:           "With logging and context",
			httpStatusCode: http.StatusBadRequest,
			apiErr:         NewError("serviceC", "errCode", "Error message.").AddContext("err", "json validation error"),
			useLogger:      true,
		},
	}
	runTests := func() {
		for i := range tests {
			tt := tests[i]
			t.Run(tt.name, func(t *testing.T) {
				MustInitAndRegisterMetrics("")
				defer UnregisterMetrics()

				var logger log.FieldLogger
				if tt.useLogger {
					logger = logtest.NewRecorder()
				}
				resp = httptest.NewRecorder()
				RespondError(resp, tt.httpStatusCode, tt.apiErr, logger)

				testutil.RequireErrorInRecorder(t, resp, tt.httpStatusCode, tt.apiErr.Domain, tt.apiErr.Code)

				if logger != nil {
					logRecorder := logger.(*logtest.Recorder)
					require.Equal(t, 1, len(logRecorder.Entries()))
					logEntry := logRecorder.Entries()[0]
					require.Equal(t, log.LevelError, logEntry.Level)
					logField, found := logEntry.FindField("error_code")
					require.True(t, found)
					require.Equal(t, tt.apiErr.Code, string(logField.Bytes))

					if tt.apiErr.Context != nil {
						logField, found = logEntry.FindField("error_context")
						require.True(t, found)
						toStrings := func(m map[string]interface{}) []string {
							var res []string
							for k, v := range m {
								res = append(res, fmt.Sprintf("%s: %v", k, v))
							}
							return res
						}

						// This function is needed because we don't have access
						// to underlying type of logf slice of strings (logf.stringArray)
						fromInternalStrArray := func(s interface{}) []string {
							var res []string
							value := reflect.ValueOf(s)
							if value.Kind() != reflect.Slice {
								t.Errorf("expected slice, got %v", value.Kind())
							}
							for i := 0; i < value.Len(); i++ {
								elem := value.Index(i)
								if elem.Kind() != reflect.String {
									t.Errorf("expected string, got %v", elem.Kind())
								}
								// Now you can work with each string element
								res = append(res, elem.String())
							}
							return res
						}

						require.Equal(t, toStrings(tt.apiErr.Context), fromInternalStrArray(logField.Any))
					}
				}

				labels := prometheus.Labels{metricsLabelResponseErrorDomain: tt.apiErr.Domain, metricsLabelResponseErrorCode: tt.apiErr.Code}
				testutil.RequireSamplesCountInCounter(t, metricsResponseErrors.With(labels), 1)
			})
		}
	}

	runTests()

	defer func() {
		respondError = RespondWrappedError
		testutil.EnableWrappingErrorInResponse()
	}()
	DisableWrappingErrorInResponse()
	testutil.DisableWrappingErrorInResponse()
	runTests()
}

func TestRespondWrappedError(t *testing.T) {
	resp := httptest.NewRecorder()
	RespondWrappedError(resp, http.StatusInternalServerError, NewInternalError(testDomain), nil)
	testutil.RequireWrappedErrorInRecorder(t, resp, http.StatusInternalServerError, testDomain, "internalError")
}

func TestRespondNoWrappedError(t *testing.T) {
	resp := httptest.NewRecorder()
	RespondNoWrappedError(resp, http.StatusInternalServerError, NewInternalError(testDomain), nil)
	testutil.RequireNoWrappedErrorInRecorder(t, resp, http.StatusInternalServerError, testDomain, "internalError")
}

func TestRespondInternalError(t *testing.T) {
	resp := httptest.NewRecorder()
	RespondInternalError(resp, testDomain, nil)
	testutil.RequireErrorInRecorder(t, resp, http.StatusInternalServerError, testDomain, "internalError")
}

func TestRespondMalformedRequestError(t *testing.T) {
	resp := httptest.NewRecorder()
	malformedReqErr := NewTooLargeMalformedRequestError(1024 * 1024)
	RespondMalformedRequestError(resp, testDomain, malformedReqErr, nil)
	testutil.RequireErrorInRecorder(t, resp, http.StatusRequestEntityTooLarge, testDomain, "requestEntityTooLarge")
}

func TestRespondMalformedRequestOrInternalError(t *testing.T) {
	t.Run("internal error", func(t *testing.T) {
		resp := httptest.NewRecorder()
		err := errors.New("unexpected error")
		RespondMalformedRequestOrInternalError(resp, testDomain, err, nil)
		testutil.RequireErrorInRecorder(t, resp, http.StatusInternalServerError, testDomain, "internalError")
	})

	t.Run("malformed error", func(t *testing.T) {
		resp := httptest.NewRecorder()
		err := NewTooLargeMalformedRequestError(1024 * 1024)
		RespondMalformedRequestOrInternalError(resp, testDomain, err, nil)
		testutil.RequireErrorInRecorder(t, resp, http.StatusRequestEntityTooLarge, testDomain, "requestEntityTooLarge")
	})
}

func TestRespondCodeAndJSON(t *testing.T) {
	rr := httptest.NewRecorder()

	logger := logtest.NewRecorder()
	// Test case 1: Valid response data
	data := map[string]string{"message": "Hello, World!"}
	RespondCodeAndJSON(rr, http.StatusOK, data, logger)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, ContentTypeAppJSON, rr.Header().Get("Content-Type"))
	var respData map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &respData)
	require.NoError(t, err)
	require.Equal(t, data, respData)

	// Test case 2: Nil response data, body should be empty and have no content type
	rr = httptest.NewRecorder()
	RespondCodeAndJSON(rr, http.StatusNoContent, nil, logger)
	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Equal(t, "", rr.Header().Get("Content-Type"))
	require.Empty(t, rr.Body.String())

	// Test case 3: Error while marshaling JSON
	rr = httptest.NewRecorder()
	invalidData := make(chan int)
	RespondCodeAndJSON(rr, http.StatusOK, invalidData, logger)
	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Equal(t, ContentTypeAppJSON, rr.Header().Get("Content-Type"))
	require.Empty(t, rr.Body.String())
}

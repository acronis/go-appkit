/*
Copyright © 2019-2024 Acronis International GmbH.
*/

package restapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/acronis/go-appkit/log"
)

// ContentTypeAppJSON represents MIME media type for JSON.
const ContentTypeAppJSON = "application/json"

// ContentTypeAppSCIMJSON represents MIME media type for SCIM JSON.
const ContentTypeAppSCIMJSON = "application/scim+json"

// Does JSON marshaling with disabled HTML escaping
func jsonMarshal(v interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(v)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes()[:buffer.Len()-1], nil
}

// RespondJSON sends response with 200 HTTP status code, does JSON marshaling of data and writes result in response's body.
func RespondJSON(rw http.ResponseWriter, respData interface{}, logger log.FieldLogger) {
	RespondCodeAndJSON(rw, http.StatusOK, respData, logger)
}

// RespondCodeAndJSON  sends a response with the passed status code and sets the "Content-Type"
// to "application/json" if it's not already set. It performs JSON marshaling of the data and
// writes the result to the response's body.
func RespondCodeAndJSON(rw http.ResponseWriter, statusCode int, respData interface{}, logger log.FieldLogger) {
	if respData == nil {
		rw.WriteHeader(statusCode)
		return
	}

	if rw.Header().Get("Content-Type") == "" {
		rw.Header().Set("Content-Type", ContentTypeAppJSON)
	}

	respJSON, err := jsonMarshal(respData)
	if err != nil {
		if logger != nil {
			logger.Error("error while marshaling json for response body", log.Error(err))
		}
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(statusCode)
	if _, err = rw.Write(respJSON); err != nil {
		if logger != nil {
			logger.Error("error while writing response body", log.Error(err))
		}
	}
}

// ErrorResponseData is used for answer on requests with error
type ErrorResponseData struct {
	Err *Error `json:"error"`
}

func (e *ErrorResponseData) Error() string {
	return fmt.Sprintf("HTTP error occurs: %v", e.Err)
}

var respondError = RespondWrappedError

// DisableWrappingErrorInResponse disables wrapping error ({"error": {"domain": "{domain}", ...} -> {"domain": "{domain}", ...})
// in response body.
func DisableWrappingErrorInResponse() {
	respondError = RespondNoWrappedError
}

// RespondError sets HTTP status code in response and writes error in body in JSON format.
// Also, it logs info (code and message) about error.
func RespondError(rw http.ResponseWriter, httpStatusCode int, err *Error, logger log.FieldLogger) {
	respondError(rw, httpStatusCode, err, logger)
}

// RespondWrappedError sets HTTP status code in response and writes wrapped error in body in JSON format.
// Also, it logs info (code and message) about error.
func RespondWrappedError(rw http.ResponseWriter, httpStatusCode int, err *Error, logger log.FieldLogger) {
	logAndCollectMetricsForErrorIfNeeded(err, logger)
	RespondCodeAndJSON(rw, httpStatusCode, ErrorResponseData{err}, logger)
}

// RespondNoWrappedError sets HTTP status code in response and writes non-wrapped error in body in JSON format.
// Also, it logs info (code and message) about error.
func RespondNoWrappedError(rw http.ResponseWriter, httpStatusCode int, err *Error, logger log.FieldLogger) {
	logAndCollectMetricsForErrorIfNeeded(err, logger)
	RespondCodeAndJSON(rw, httpStatusCode, err, logger)
}

// RespondInternalError sends response with 500 HTTP status code and internal error in body in JSON format.
func RespondInternalError(rw http.ResponseWriter, domain string, logger log.FieldLogger) {
	RespondError(rw, http.StatusInternalServerError, NewInternalError(domain), logger)
}

// RespondMalformedRequestError creates Error from passed MalformedRequestError and then call RespondError.
func RespondMalformedRequestError(rw http.ResponseWriter, domain string, reqErr *MalformedRequestError, logger log.FieldLogger) {
	err := NewError(domain, httpCode2ErrorCode(reqErr.HTTPStatusCode), reqErr.Message)
	RespondError(rw, reqErr.HTTPStatusCode, err, logger)
}

// RespondMalformedRequestOrInternalError calls RespondMalformedRequestError (if passed error is *MalformedRequestError)
// or RespondInternalError (in other cases).
func RespondMalformedRequestOrInternalError(rw http.ResponseWriter, domain string, err error, logger log.FieldLogger) {
	var reqErr *MalformedRequestError
	if errors.As(err, &reqErr) {
		RespondMalformedRequestError(rw, domain, reqErr, logger)
		return
	}
	RespondInternalError(rw, domain, logger)
}

func logAndCollectMetricsForErrorIfNeeded(err *Error, logger log.FieldLogger) {
	if logger != nil {
		flds := []log.Field{log.String("error_code", err.Code), log.String("error_message", err.Message)}
		if err.Context != nil {
			ctxLines := make([]string, 0, len(err.Context))
			for k, v := range err.Context {
				ctxLines = append(ctxLines, fmt.Sprintf("%s: %v", k, v))
			}
			flds = append(flds, log.Strings("error_context", ctxLines))
		}
		logger.Error("error in response", flds...)
	}
	if metricsResponseErrors != nil {
		metricsResponseErrors.With(prometheus.Labels{
			metricsLabelResponseErrorDomain: err.Domain,
			metricsLabelResponseErrorCode:   err.Code,
		}).Inc()
	}
}

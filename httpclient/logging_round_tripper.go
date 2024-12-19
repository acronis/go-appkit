/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"fmt"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"net/http"
	"time"
)

// LoggingRoundTripper implements http.RoundTripper for logging requests.
type LoggingRoundTripper struct {
	Delegate http.RoundTripper
	ReqType  string
}

// NewLoggingRoundTripper creates an HTTP transport that log requests.
func NewLoggingRoundTripper(delegate http.RoundTripper, reqType string) http.RoundTripper {
	return &LoggingRoundTripper{
		Delegate: delegate,
		ReqType:  reqType,
	}
}

// RoundTrip adds logging capabilities to the HTTP transport.
func (rt *LoggingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	logger := middleware.GetLoggerFromContext(ctx)
	start := time.Now()
	
	resp, err := rt.Delegate.RoundTrip(r)
	if logger != nil {
		common := "external request %s %s req type %s "
		elapsed := time.Since(start)

		args := []interface{}{r.Method, r.URL.String(), rt.ReqType, elapsed.Seconds(), err}
		message := common + "time taken %.3f, err %+v"

		if resp != nil {
			args = []interface{}{r.Method, r.URL.String(), rt.ReqType, resp.StatusCode, elapsed.Seconds(), err}
			message = common + "status code %d, time taken %.3f, err %+v"
		}

		logger.Infof(message, args...)

		loggingParams := middleware.GetLoggingParamsFromContext(ctx)
		if loggingParams != nil {
			loggingParams.AddTimeSlotDurationInMs(fmt.Sprintf("external_request_%s_ms", rt.ReqType), elapsed)
		}
	}

	return resp, err
}

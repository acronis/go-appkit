/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"net/http"
	"time"
)

// LoggingMode represents a mode of logging.
type LoggingMode string

const (
	LoggingModeNone   LoggingMode = "none"
	LoggingModeAll    LoggingMode = "all"
	LoggingModeFailed LoggingMode = "failed"
)

// IsValid checks if the logger mode is valid.
func (lm LoggingMode) IsValid() bool {
	switch lm {
	case LoggingModeNone, LoggingModeAll, LoggingModeFailed:
		return true
	}
	return false
}

// LoggingRoundTripper implements http.RoundTripper for logging requests.
type LoggingRoundTripper struct {
	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// ReqType is a type of request. e.g. service 'auth-service', an action 'login' or specific information to correlate.
	ReqType string

	// Mode of logging: none, all, failed.
	Mode LoggingMode

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold time.Duration

	// LoggerProvider is a function that provides a context-specific logger.
	// middleware.GetLoggerFromContext is used by default.
	LoggerProvider func(ctx context.Context) log.FieldLogger
}

// LoggingRoundTripperOpts represents an options for LoggingRoundTripper.
type LoggingRoundTripperOpts struct {
	// ReqType is a type of request. e.g. service 'auth-service', an action 'login' or specific information to correlate.
	ReqType string

	// Mode of logging: none, all, failed.
	Mode LoggingMode

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold time.Duration

	// LoggerProvider is a function that provides a context-specific logger.
	// middleware.GetLoggerFromContext is used by default.
	LoggerProvider func(ctx context.Context) log.FieldLogger
}

// NewLoggingRoundTripper creates an HTTP transport that log requests.
func NewLoggingRoundTripper(delegate http.RoundTripper) http.RoundTripper {
	return NewLoggingRoundTripperWithOpts(delegate, LoggingRoundTripperOpts{
		ReqType: DefaultReqType,
	})
}

// NewLoggingRoundTripperWithOpts creates an HTTP transport that log requests with options.
func NewLoggingRoundTripperWithOpts(
	delegate http.RoundTripper, opts LoggingRoundTripperOpts,
) http.RoundTripper {
	reqType := opts.ReqType
	if reqType == "" {
		reqType = DefaultReqType
	}

	return &LoggingRoundTripper{
		Delegate:             delegate,
		ReqType:              reqType,
		Mode:                 opts.Mode,
		SlowRequestThreshold: opts.SlowRequestThreshold,
		LoggerProvider:       opts.LoggerProvider,
	}
}

// getLogger returns a logger from the context or from the options.
func (rt *LoggingRoundTripper) getLogger(ctx context.Context) log.FieldLogger {
	if rt.LoggerProvider != nil {
		return rt.LoggerProvider(ctx)
	}

	return middleware.GetLoggerFromContext(ctx)
}

// RoundTrip adds logging capabilities to the HTTP transport.
func (rt *LoggingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.Mode == LoggingModeNone {
		return rt.Delegate.RoundTrip(r)
	}

	ctx := r.Context()
	logger := rt.getLogger(ctx)
	start := time.Now()

	resp, err := rt.Delegate.RoundTrip(r)
	elapsed := time.Since(start)
	if logger != nil && elapsed >= rt.SlowRequestThreshold {
		common := "client http request %s %s req type %s "
		args := []interface{}{r.Method, r.URL.String(), rt.ReqType, elapsed.Seconds(), err}
		message := common + "time taken %.3f, err %+v"
		loggerAtLevel := logger.Infof

		if resp != nil {
			if rt.Mode == LoggingModeFailed && resp.StatusCode < http.StatusBadRequest {
				return resp, err
			}

			args = []interface{}{r.Method, r.URL.String(), rt.ReqType, resp.StatusCode, elapsed.Seconds(), err}
			message = common + "status code %d, time taken %.3f, err %+v"
		}

		if err != nil {
			loggerAtLevel = logger.Errorf
		}

		loggerAtLevel(message, args...)
		loggingParams := middleware.GetLoggingParamsFromContext(ctx)
		if loggingParams != nil {
			loggingParams.AddTimeSlotDurationInMs(fmt.Sprintf("external_request_%s_ms", rt.ReqType), elapsed)
			requestID := middleware.GetRequestIDFromContext(ctx)
			if requestID != "" {
				loggingParams.ExtendFields(log.String("request_id", requestID))
			}
		}
	}

	return resp, err
}

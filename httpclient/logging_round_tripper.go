/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
)

// LoggingMode represents a mode of logging.
type LoggingMode string

const (
	LoggingModeAll    LoggingMode = "all"
	LoggingModeFailed LoggingMode = "failed"
)

// IsValid checks if the logger mode is valid.
func (lm LoggingMode) IsValid() bool {
	switch lm {
	case LoggingModeAll, LoggingModeFailed:
		return true
	}
	return false
}

// LoggingRoundTripper implements http.RoundTripper for logging requests.
type LoggingRoundTripper struct {
	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// ClientType represents a type of client, it's a service component reference. e.g. 'auth-service'
	ClientType string

	// Mode of logging: [all, failed]. 'all' by default.
	Mode LoggingMode

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold time.Duration

	// LoggerProvider is a function that provides a context-specific logger.
	// middleware.GetLoggerFromContext is used by default.
	LoggerProvider func(ctx context.Context) log.FieldLogger
}

// LoggingRoundTripperOpts represents an options for LoggingRoundTripper.
type LoggingRoundTripperOpts struct {
	// ClientType represents a type of client, it's a service component reference. e.g. 'auth-service'
	ClientType string

	// Mode of logging: [all, failed]. 'all' by default.
	Mode LoggingMode

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold time.Duration

	// LoggerProvider is a function that provides a context-specific logger.
	// middleware.GetLoggerFromContext is used by default.
	LoggerProvider func(ctx context.Context) log.FieldLogger
}

// NewLoggingRoundTripper creates an HTTP transport that log requests.
func NewLoggingRoundTripper(delegate http.RoundTripper) http.RoundTripper {
	return NewLoggingRoundTripperWithOpts(delegate, LoggingRoundTripperOpts{})
}

// NewLoggingRoundTripperWithOpts creates an HTTP transport that log requests with options.
func NewLoggingRoundTripperWithOpts(
	delegate http.RoundTripper, opts LoggingRoundTripperOpts,
) http.RoundTripper {
	return &LoggingRoundTripper{
		Delegate:             delegate,
		ClientType:           opts.ClientType,
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
	ctx := r.Context()
	logger := rt.getLogger(ctx)
	if logger == nil {
		return rt.Delegate.RoundTrip(r)
	}

	start := time.Now()
	resp, err := rt.Delegate.RoundTrip(r)
	elapsed := time.Since(start)
	common := "client http request %s %s "
	args := []interface{}{r.Method, r.URL.String(), elapsed.Seconds(), err}
	message := common + "time taken %.3f, err %+v"
	loggerAtLevel := logger.Infof

	isSlowRequest := elapsed >= rt.SlowRequestThreshold
	var isSuccessful bool

	if resp != nil {
		isSuccessful = resp.StatusCode < http.StatusBadRequest
		if rt.Mode == LoggingModeFailed && isSuccessful && !isSlowRequest {
			return resp, err
		}

		args = []interface{}{r.Method, r.URL.String(), resp.StatusCode, elapsed.Seconds(), err}
		message = common + "status code %d, time taken %.3f, err %+v"
	}

	if err != nil || !isSuccessful {
		loggerAtLevel = logger.Errorf
	} else if isSlowRequest {
		loggerAtLevel = logger.Warnf
	}

	if rt.ClientType != "" {
		message += fmt.Sprintf(" client type %s ", rt.ClientType)
	}

	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		message += fmt.Sprintf("request id %s ", requestID)
	}

	if requestType := GetRequestTypeFromContext(ctx); requestType != "" {
		message += fmt.Sprintf("request type %s ", requestType)
	}

	loggerAtLevel(message, args...)

	loggingParams := middleware.GetLoggingParamsFromContext(ctx)
	if loggingParams != nil {
		loggingParams.AddTimeSlotDurationInMs(fmt.Sprintf("external_request_%s_ms", rt.ClientType), elapsed)
	}

	return resp, err
}

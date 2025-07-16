/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"github.com/acronis/go-appkit/log"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyInternalRequestID
	ctxKeyLogger
	ctxKeyLoggingParams
	ctxKeyTraceID
	ctxKeyCallStartTime
)

// NewContextWithRequestID creates a new context with external request id.
func NewContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, requestID)
}

// GetRequestIDFromContext extracts external request id from the context.
func GetRequestIDFromContext(ctx context.Context) string {
	return getStringFromContext(ctx, ctxKeyRequestID)
}

// NewContextWithInternalRequestID creates a new context with internal request id.
func NewContextWithInternalRequestID(ctx context.Context, internalRequestID string) context.Context {
	return context.WithValue(ctx, ctxKeyInternalRequestID, internalRequestID)
}

// GetInternalRequestIDFromContext extracts internal request id from the context.
func GetInternalRequestIDFromContext(ctx context.Context) string {
	return getStringFromContext(ctx, ctxKeyInternalRequestID)
}

// NewContextWithCallStartTime creates a new context with request start time.
func NewContextWithCallStartTime(ctx context.Context, startTime time.Time) context.Context {
	return context.WithValue(ctx, ctxKeyCallStartTime, startTime)
}

// GetCallStartTimeFromContext extracts request start time from the context.
func GetCallStartTimeFromContext(ctx context.Context) time.Time {
	startTime, _ := ctx.Value(ctxKeyCallStartTime).(time.Time)
	return startTime
}

// NewContextWithLogger creates a new context with logger.
func NewContextWithLogger(ctx context.Context, logger log.FieldLogger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger, logger)
}

// GetLoggerFromContext extracts logger from the context.
func GetLoggerFromContext(ctx context.Context) log.FieldLogger {
	value := ctx.Value(ctxKeyLogger)
	if value == nil {
		return nil
	}
	return value.(log.FieldLogger)
}

// NewContextWithLoggingParams creates a new context with logging params.
func NewContextWithLoggingParams(ctx context.Context, loggingParams *LoggingParams) context.Context {
	return context.WithValue(ctx, ctxKeyLoggingParams, loggingParams)
}

// GetLoggingParamsFromContext extracts logging params from the context.
func GetLoggingParamsFromContext(ctx context.Context) *LoggingParams {
	value := ctx.Value(ctxKeyLoggingParams)
	if value == nil {
		return nil
	}
	return value.(*LoggingParams)
}

// NewContextWithTraceID creates a new context with trace id.
func NewContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxKeyTraceID, traceID)
}

// GetTraceIDFromContext extracts trace id from the context.
func GetTraceIDFromContext(ctx context.Context) string {
	return getStringFromContext(ctx, ctxKeyTraceID)
}

func getStringFromContext(ctx context.Context, key ctxKey) string {
	value := ctx.Value(key)
	if value == nil {
		return ""
	}
	return value.(string)
}

// WrappedServerStream wraps grpc.ServerStream to provide a custom context for the stream.
type WrappedServerStream struct {
	grpc.ServerStream
	Ctx context.Context
}

// Context returns the custom context for the wrapped server stream.
func (ss *WrappedServerStream) Context() context.Context {
	return ss.Ctx
}

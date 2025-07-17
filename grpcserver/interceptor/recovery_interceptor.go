/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"fmt"
	"runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/log"
)

const (
	// RecoveryDefaultStackSize defines the default size of stack part which will be logged.
	RecoveryDefaultStackSize = 8192
)

// InternalError is the default error returned when a panic is recovered.
var InternalError = status.Error(codes.Internal, "Internal error")

// recoveryOptions represents options for RecoveryUnaryInterceptor.
type recoveryOptions struct {
	StackSize int
}

// RecoveryOption is a function type for configuring recoveryOptions.
type RecoveryOption func(*recoveryOptions)

// WithRecoveryStackSize sets the stack size for logging stack traces.
func WithRecoveryStackSize(size int) RecoveryOption {
	return func(opts *recoveryOptions) {
		opts.StackSize = size
	}
}

// RecoveryUnaryInterceptor is a gRPC unary interceptor that recovers from panics and returns Internal error.
func RecoveryUnaryInterceptor(options ...RecoveryOption) func(
	ctx context.Context,
	req interface{},
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	opts := recoveryOptions{
		StackSize: RecoveryDefaultStackSize,
	}
	for _, option := range options {
		option(&opts)
	}
	return func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if p := recover(); p != nil {
				if logger := GetLoggerFromContext(ctx); logger != nil {
					var logFields []log.Field
					if opts.StackSize > 0 {
						stack := make([]byte, opts.StackSize)
						stack = stack[:runtime.Stack(stack, false)]
						logFields = append(logFields, log.Bytes("stack", stack))
					}
					logger.Error(fmt.Sprintf("Panic: %+v", p), logFields...)
				}
				err = InternalError
			}
		}()
		return handler(ctx, req)
	}
}

// RecoveryStreamInterceptor is a gRPC stream interceptor that recovers from panics and returns Internal error.
func RecoveryStreamInterceptor(options ...RecoveryOption) func(
	srv interface{},
	ss grpc.ServerStream,
	_ *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	opts := recoveryOptions{
		StackSize: RecoveryDefaultStackSize,
	}
	for _, option := range options {
		option(&opts)
	}
	return func(
		srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if p := recover(); p != nil {
				if logger := GetLoggerFromContext(ss.Context()); logger != nil {
					var logFields []log.Field
					if opts.StackSize > 0 {
						stack := make([]byte, opts.StackSize)
						stack = stack[:runtime.Stack(stack, false)]
						logFields = append(logFields, log.Bytes("stack", stack))
					}
					logger.Error(fmt.Sprintf("Panic: %+v", p), logFields...)
				}
				err = InternalError
			}
		}()
		return handler(srv, ss)
	}
}

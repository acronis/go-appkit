/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"

	"github.com/rs/xid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	headerRequestIDKey         = "x-request-id"
	headerRequestInternalIDKey = "x-int-request-id"
)

// requestIDOptions represents options for RequestIDUnaryInterceptor.
type requestIDOptions struct {
	GenerateID         func() string
	GenerateInternalID func() string
}

// RequestIDOption is a function type for configuring requestIDOptions.
type RequestIDOption func(*requestIDOptions)

func newID() string {
	return xid.New().String()
}

// WithRequestIDGenerator sets the function for generating request IDs.
func WithRequestIDGenerator(generator func() string) RequestIDOption {
	return func(opts *requestIDOptions) {
		opts.GenerateID = generator
	}
}

// WithInternalRequestIDGenerator sets the function for generating internal request IDs.
func WithInternalRequestIDGenerator(generator func() string) RequestIDOption {
	return func(opts *requestIDOptions) {
		opts.GenerateInternalID = generator
	}
}

// RequestIDUnaryInterceptor is a gRPC unary interceptor that extracts the request ID from the incoming context metadata
// and attaches it to the context. If the request ID is missing, a new one is generated.
func RequestIDUnaryInterceptor(options ...RequestIDOption) func(
	ctx context.Context,
	req interface{},
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	opts := requestIDOptions{
		GenerateID:         newID,
		GenerateInternalID: newID,
	}
	for _, option := range options {
		option(&opts)
	}

	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var requestID string
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			requestIDList := md.Get(headerRequestIDKey)
			if len(requestIDList) > 0 {
				requestID = requestIDList[0]
			}
		}
		if requestID == "" {
			requestID = opts.GenerateID()
		}
		internalRequestID := opts.GenerateInternalID()

		headerMD := metadata.Pairs(
			headerRequestIDKey, requestID,
			headerRequestInternalIDKey, internalRequestID,
		)
		if err := grpc.SetHeader(ctx, headerMD); err != nil {
			return nil, err
		}

		ctx = NewContextWithRequestID(ctx, requestID)
		ctx = NewContextWithInternalRequestID(ctx, internalRequestID)

		return handler(ctx, req)
	}
}

// RequestIDStreamInterceptor is a gRPC stream interceptor that extracts the request ID from the incoming context metadata
// and attaches it to the context. If the request ID is missing, a new one is generated.
func RequestIDStreamInterceptor(options ...RequestIDOption) func(
	srv interface{},
	ss grpc.ServerStream,
	_ *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	opts := requestIDOptions{
		GenerateID:         newID,
		GenerateInternalID: newID,
	}
	for _, option := range options {
		option(&opts)
	}

	return func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		var requestID string
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			requestIDList := md.Get(headerRequestIDKey)
			if len(requestIDList) > 0 {
				requestID = requestIDList[0]
			}
		}
		if requestID == "" {
			requestID = opts.GenerateID()
		}

		internalRequestID := opts.GenerateInternalID()

		headerMD := metadata.Pairs(
			headerRequestIDKey, requestID,
			headerRequestInternalIDKey, internalRequestID,
		)
		if err := ss.SetHeader(headerMD); err != nil {
			return err
		}

		ctx := NewContextWithRequestID(ss.Context(), requestID)
		ctx = NewContextWithInternalRequestID(ctx, internalRequestID)

		wrappedStream := &WrappedServerStream{ServerStream: ss, Ctx: ctx}
		return handler(srv, wrappedStream)
	}
}

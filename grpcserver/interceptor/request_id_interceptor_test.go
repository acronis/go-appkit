/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
)

// RequestIDInterceptorTestSuite is a test suite for RequestID interceptors
type RequestIDInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func TestRequestIDServerUnaryInterceptor(t *testing.T) {
	suite.Run(t, &RequestIDInterceptorTestSuite{IsUnary: true})
}

func TestRequestIDServerStreamInterceptor(t *testing.T) {
	suite.Run(t, &RequestIDInterceptorTestSuite{IsUnary: false})
}

// TestRequestIDServerUnaryInterceptor tests the unary interceptor
func (s *RequestIDInterceptorTestSuite) TestRequestIDServerInterceptor() {
	tests := []struct {
		name    string
		options []RequestIDOption
		md      metadata.MD
		checkFn func(t *testing.T, svc *testService, header metadata.MD)
	}{
		{
			name:    "Default generators - Request ID not present in request header metadata",
			options: nil,
			md:      metadata.Pairs(),
			checkFn: func(t *testing.T, svc *testService, respHeader metadata.MD) {
				reqID := s.getStringFromMd(respHeader, headerRequestIDKey)
				require.NotEmpty(t, reqID)
				require.Equal(t, reqID, GetRequestIDFromContext(svc.LastContext()))

				intReqID := s.getStringFromMd(respHeader, headerRequestInternalIDKey)
				require.NotEmpty(t, intReqID)
				require.Equal(t, intReqID, GetInternalRequestIDFromContext(svc.LastContext()))

				require.NotEqual(t, reqID, intReqID)
			},
		},
		{
			name:    "Default generators - Request ID present in metadata",
			options: nil,
			md:      metadata.Pairs(headerRequestIDKey, "existing-request-id"),
			checkFn: func(t *testing.T, svc *testService, respHeader metadata.MD) {
				reqID := s.getStringFromMd(respHeader, headerRequestIDKey)
				require.Equal(t, "existing-request-id", reqID)
				require.Equal(t, reqID, GetRequestIDFromContext(svc.LastContext()))

				intReqID := s.getStringFromMd(respHeader, headerRequestInternalIDKey)
				require.NotEmpty(t, intReqID)
				require.Equal(t, intReqID, GetInternalRequestIDFromContext(svc.LastContext()))

				require.NotEqual(t, reqID, intReqID)
			},
		},
		{
			name: "Custom generators - Request ID not present",
			options: []RequestIDOption{
				WithRequestIDGenerator(func() string { return "custom-request-id" }),
				WithInternalRequestIDGenerator(func() string { return "custom-internal-id" }),
			},
			md: metadata.Pairs(),
			checkFn: func(t *testing.T, svc *testService, respHeader metadata.MD) {
				reqID := s.getStringFromMd(respHeader, headerRequestIDKey)
				require.Equal(t, "custom-request-id", reqID)
				require.Equal(t, reqID, GetRequestIDFromContext(svc.LastContext()))

				intReqID := s.getStringFromMd(respHeader, headerRequestInternalIDKey)
				require.Equal(t, "custom-internal-id", intReqID)
				require.Equal(t, intReqID, GetInternalRequestIDFromContext(svc.LastContext()))
			},
		},
		{
			name: "Custom generators - Existing request ID preserved, custom internal ID generator used",
			options: []RequestIDOption{
				WithRequestIDGenerator(func() string { return "custom-request-id" }),
				WithInternalRequestIDGenerator(func() string { return "custom-internal-id" }),
			},
			md: metadata.Pairs(headerRequestIDKey, "existing-request-id"),
			checkFn: func(t *testing.T, svc *testService, respHeader metadata.MD) {
				reqID := s.getStringFromMd(respHeader, headerRequestIDKey)
				require.Equal(t, "existing-request-id", reqID)
				require.Equal(t, reqID, GetRequestIDFromContext(svc.LastContext()))

				intReqID := s.getStringFromMd(respHeader, headerRequestInternalIDKey)
				require.Equal(t, "custom-internal-id", intReqID)
				require.Equal(t, intReqID, GetInternalRequestIDFromContext(svc.LastContext()))
			},
		},
		{
			name: "Custom request ID generator only",
			options: []RequestIDOption{
				WithRequestIDGenerator(func() string { return "custom-request-id" }),
			},
			md: metadata.Pairs(),
			checkFn: func(t *testing.T, svc *testService, respHeader metadata.MD) {
				reqIDVals := respHeader.Get(headerRequestIDKey)
				require.Len(t, reqIDVals, 1)
				require.Equal(t, "custom-request-id", reqIDVals[0])

				intReqIDVals := respHeader.Get(headerRequestInternalIDKey)
				require.Len(t, intReqIDVals, 1)
				require.NotEmpty(t, intReqIDVals[0])
				require.NotEqual(t, "custom-request-id", intReqIDVals[0]) // Should use default generator
			},
		},
		{
			name: "Custom internal request ID generator only",
			options: []RequestIDOption{
				WithInternalRequestIDGenerator(func() string { return "custom-internal-id" }),
			},
			md: metadata.Pairs(),
			checkFn: func(t *testing.T, svc *testService, respHeader metadata.MD) {
				reqIDVals := respHeader.Get(headerRequestIDKey)
				require.Len(t, reqIDVals, 1)
				require.NotEmpty(t, reqIDVals[0])
				require.NotEqual(t, "custom-internal-id", reqIDVals[0]) // Should use default generator

				intReqIDVals := respHeader.Get(headerRequestInternalIDKey)
				require.Len(t, intReqIDVals, 1)
				require.Equal(t, "custom-internal-id", intReqIDVals[0])
			},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			svc, client, closeSvc, err := startTestService([]grpc.ServerOption{
				grpc.UnaryInterceptor(RequestIDUnaryInterceptor(tt.options...)),
				grpc.StreamInterceptor(RequestIDStreamInterceptor(tt.options...)),
			}, nil)
			s.Require().NoError(err)
			defer func() { s.Require().NoError(closeSvc()) }()

			var header metadata.MD
			if s.IsUnary {
				reqCtx := metadata.NewOutgoingContext(context.Background(), tt.md)
				resp, respErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{}, grpc.Header(&header))
				s.Require().NoError(respErr)
				s.Require().Equal("test", string(resp.Payload.GetBody()))
			} else {
				reqCtx := metadata.NewOutgoingContext(context.Background(), tt.md)
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(streamErr)

				// Receive one message to trigger the interceptor
				resp, recvErr := stream.Recv()
				s.Require().NoError(recvErr)
				s.Require().Equal("test-stream", string(resp.Payload.GetBody()))

				// Get headers from stream
				var headerErr error
				header, headerErr = stream.Header()
				s.Require().NoError(headerErr)
			}

			tt.checkFn(s.T(), svc, header)
		})
	}
}

// getStringFromMd is a helper function to extract a single string value from metadata
func (s *RequestIDInterceptorTestSuite) getStringFromMd(md metadata.MD, key string) string {
	s.T().Helper()
	vals := md.Get(key)
	s.Require().Len(vals, 1)
	return vals[0]
}

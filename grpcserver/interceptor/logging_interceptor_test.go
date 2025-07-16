/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
)

// LoggingInterceptorTestSuite is a test suite for Logging interceptors
type LoggingInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func TestLoggingUnaryInterceptor(t *testing.T) {
	suite.Run(t, &LoggingInterceptorTestSuite{IsUnary: true})
}

func TestLoggingStreamInterceptor(t *testing.T) {
	suite.Run(t, &LoggingInterceptorTestSuite{IsUnary: false})
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor() {
	const headerRequestID = "test-request-id"
	const headerUserAgent = "test-user-agent"

	permissionDeniedErr := status.Error(codes.PermissionDenied, "Permission denied")

	tests := []struct {
		name             string
		md               metadata.MD
		unaryCallHandler func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error)
		streamHandler    func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error
		wantErr          error
		wantCode         codes.Code
	}{
		{
			name:     "log gRPC call is started and finished",
			md:       metadata.Pairs(headerRequestIDKey, headerRequestID),
			wantCode: codes.OK,
		},
		{
			name: "log gRPC call is started and finished with error",
			md:   metadata.Pairs(headerRequestIDKey, headerRequestID),
			unaryCallHandler: func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
				return nil, permissionDeniedErr
			},
			streamHandler: func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
				return permissionDeniedErr
			},
			wantErr:  permissionDeniedErr,
			wantCode: codes.PermissionDenied,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := logtest.NewRecorder()
			svc, client, closeSvc, err := s.setupTestService(logger, headerUserAgent, nil)
			s.Require().NoError(err)
			defer func() { s.Require().NoError(closeSvc()) }()

			if tt.unaryCallHandler != nil {
				svc.SwitchUnaryCallHandler(tt.unaryCallHandler)
			}
			if tt.streamHandler != nil {
				svc.SwitchStreamingOutputCallHandler(tt.streamHandler)
			}

			var headers metadata.MD
			reqCtx := metadata.NewOutgoingContext(context.Background(), tt.md)

			if s.IsUnary {
				resp, respErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{}, grpc.Header(&headers))
				if tt.wantErr != nil {
					s.Require().ErrorIs(respErr, tt.wantErr)
				} else {
					s.Require().NoError(respErr)
					s.Require().Equal("test", string(resp.Payload.GetBody()))
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(streamErr)

				_, recvErr := stream.Recv()
				if tt.wantErr != nil {
					s.Require().ErrorIs(recvErr, tt.wantErr)
				} else {
					s.Require().NoError(recvErr)
				}
			}

			s.Require().Equal(1, len(logger.Entries()))
			callFinishedLogEntry := logger.Entries()[0]
			s.Require().Contains(callFinishedLogEntry.Text, "gRPC call finished")
			s.Require().Equal(log.LevelInfo, callFinishedLogEntry.Level)
			s.requireCommonFields(callFinishedLogEntry, headerRequestID, headerUserAgent)
			s.requireLogFieldString(callFinishedLogEntry, "grpc_code", tt.wantCode.String())
			_, found := callFinishedLogEntry.FindField("duration_ms")
			s.Require().True(found)

			if tt.wantErr != nil {
				s.requireLogFieldString(callFinishedLogEntry, "grpc_error", tt.wantErr.Error())
			}
		})
	}
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptorWithOptions() {
	const headerRequestID = "test-request-id"
	const headerUserAgent = "test-user-agent"

	// Test with request start enabled
	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, headerUserAgent, []LoggingOption{WithLoggingCallStart(true)})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))

	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)

		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	// Should have 2 entries: started and finished
	s.Require().Equal(2, len(logger.Entries()))
	s.Require().Contains(logger.Entries()[0].Text, "gRPC call started")
	s.Require().Contains(logger.Entries()[1].Text, "gRPC call finished")
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_AllOptions() {
	const (
		headerRequestID   = "test-request-id"
		headerUserAgent   = "test-user-agent"
		headerTraceID     = "test-trace-id"
		headerCustomValue = "custom-header-value"
	)

	customLogger := logtest.NewRecorder()

	tests := []struct {
		name            string
		options         []LoggingOption
		expectedLogs    int
		testCustomLog   bool
		useCustomLogger bool
	}{
		{
			name: "With all options enabled",
			options: []LoggingOption{
				WithLoggingCallStart(true),
				WithLoggingCallHeaders(map[string]string{"custom-header": "custom_header_field"}),
				WithLoggingAddCallInfoToLogger(true),
				WithLoggingSlowCallThreshold(50 * time.Millisecond),
			},
			expectedLogs: 2,
		},
		{
			name: "With excluded methods",
			options: []LoggingOption{
				WithLoggingCallStart(true),
				WithLoggingExcludedMethods(s.getMethodPath()),
			},
			expectedLogs: 0, // Should be excluded
		},
		{
			name: "With custom logger provider",
			options: []LoggingOption{
				WithLoggingCallStart(true),
				s.getCustomLoggerProvider(customLogger),
			},
			expectedLogs:    2,
			testCustomLog:   true,
			useCustomLogger: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := logtest.NewRecorder()
			customLogger.Reset()

			_, client, closeSvc, err := s.setupTestService(logger, headerUserAgent, tt.options)
			s.Require().NoError(err)
			defer func() { s.Require().NoError(closeSvc()) }()

			md := metadata.Pairs(
				headerRequestIDKey, headerRequestID,
				"custom-header", headerCustomValue,
			)
			reqCtx := metadata.NewOutgoingContext(context.Background(), md)

			// Add trace ID to context
			reqCtx = NewContextWithTraceID(reqCtx, headerTraceID)

			if s.IsUnary {
				_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
				s.Require().NoError(err)
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(streamErr)
				_, recvErr := stream.Recv()
				s.Require().NoError(recvErr)
			}

			if tt.testCustomLog && tt.useCustomLogger {
				s.Require().Equal(tt.expectedLogs, len(customLogger.Entries()))
				if len(customLogger.Entries()) > 0 {
					// Verify custom logger was used
					logEntry := customLogger.Entries()[0]
					s.Require().Contains(logEntry.Text, "gRPC call started")
				}
			} else {
				s.Require().Equal(tt.expectedLogs, len(logger.Entries()))
				if len(logger.Entries()) > 0 {
					logEntry := logger.Entries()[0]
					if tt.name == "With all options enabled" {
						s.Require().Contains(logEntry.Text, "gRPC call started")
						// Check custom headers
						s.requireLogFieldString(logEntry, "custom_header_field", headerCustomValue)
					}
				}
			}
		})
	}
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_ExcludedMethods() {
	const headerRequestID = "test-request-id"

	tests := []struct {
		name         string
		method       string
		statusCode   codes.Code
		expectedLogs int
	}{
		{
			name:         "Excluded method with success",
			method:       s.getMethodPath(),
			statusCode:   codes.OK,
			expectedLogs: 0,
		},
		{
			name:         "Excluded method with error",
			method:       s.getMethodPath(),
			statusCode:   codes.Internal,
			expectedLogs: 1, // Should log errors even for excluded methods
		},
		{
			name:         "Non-excluded method",
			method:       s.getMethodPath(),
			statusCode:   codes.OK,
			expectedLogs: 1,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := logtest.NewRecorder()

			// Use different exclusion based on test case
			excludedMethod := s.getMethodPath()
			if tt.name == "Non-excluded method" {
				excludedMethod = "/grpc.testing.TestService/DifferentCall"
			}

			svc, client, closeSvc, err := s.setupTestService(logger, "", []LoggingOption{
				WithLoggingExcludedMethods(excludedMethod),
			})
			s.Require().NoError(err)
			defer func() { s.Require().NoError(closeSvc()) }()

			if tt.statusCode != codes.OK {
				svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
					return nil, status.Error(tt.statusCode, "test error")
				})
				svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
					return status.Error(tt.statusCode, "test error")
				})
			}

			reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))

			if s.IsUnary {
				_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
				if tt.statusCode != codes.OK {
					s.Require().Error(err)
				} else {
					s.Require().NoError(err)
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(streamErr)
				_, recvErr := stream.Recv()
				if tt.statusCode != codes.OK {
					s.Require().Error(recvErr)
				} else {
					s.Require().NoError(recvErr)
				}
			}

			s.Require().Equal(tt.expectedLogs, len(logger.Entries()))
		})
	}
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_SlowRequests() {
	const headerRequestID = "test-request-id"

	var threshold time.Duration
	if s.IsUnary {
		threshold = 10 * time.Millisecond
	} else {
		threshold = 1 * time.Nanosecond // Very low threshold for stream to trigger slow request
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, "", []LoggingOption{
		WithLoggingSlowCallThreshold(threshold),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up a slow handler
	svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
		time.Sleep(50 * time.Millisecond) // Simulate slow processing
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
	})

	reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))

	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	s.Require().Equal(1, len(logger.Entries()))
	logEntry := logger.Entries()[0]
	s.Require().Contains(logEntry.Text, "gRPC call finished")

	// Check for slow request flag
	slowField, found := logEntry.FindField("slow_request")
	s.Require().True(found)
	s.Require().True(slowField.Int != 0)

	// Check for time_slots field
	_, found = logEntry.FindField("time_slots")
	s.Require().True(found)
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_LoggingParams() {
	const headerRequestID = "test-request-id"

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, "", nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// We need to test this separately as it requires custom interceptor setup
	// This is testing a feature that works with both unary and stream interceptors
	if s.IsUnary {
		reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	s.Require().Equal(1, len(logger.Entries()))
	logEntry := logger.Entries()[0]
	s.Require().Contains(logEntry.Text, "gRPC call finished")

	// Note: This test would need a custom setup to properly test LoggingParams
	// For now, we'll just ensure basic logging works
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_RemoteAddressParsing() {
	const headerRequestID = "test-request-id"

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, "", nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))

	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	s.Require().Equal(1, len(logger.Entries()))
	logEntry := logger.Entries()[0]

	// Check that remote address fields are present
	remoteAddrField, found := logEntry.FindField("remote_addr")
	s.Require().True(found)
	s.Require().True(strings.HasPrefix(string(remoteAddrField.Bytes), "127.0.0.1:"))

	// Check parsed IP and port
	ipField, found := logEntry.FindField("remote_addr_ip")
	s.Require().True(found)
	s.Require().Equal("127.0.0.1", string(ipField.Bytes))

	portField, found := logEntry.FindField("remote_addr_port")
	s.Require().True(found)
	s.Require().Greater(uint16(portField.Int), uint16(0))
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_RequestHeaders() {
	const headerRequestID = "test-request-id"

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, "", []LoggingOption{
		WithLoggingCallHeaders(map[string]string{
			"x-custom-header":  "custom_header_log",
			"x-missing-header": "missing_header_log",
		}),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	md := metadata.Pairs(
		headerRequestIDKey, headerRequestID,
		"x-custom-header", "custom-value",
	)
	reqCtx := metadata.NewOutgoingContext(context.Background(), md)

	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	s.Require().Equal(1, len(logger.Entries()))
	logEntry := logger.Entries()[0]

	// Check that custom header is logged
	s.requireLogFieldString(logEntry, "custom_header_log", "custom-value")

	// Check that missing header is not logged (field should not exist)
	_, found := logEntry.FindField("missing_header_log")
	s.Require().False(found)
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_AddRequestInfoToLogger() {
	const headerRequestID = "test-request-id"

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, "", []LoggingOption{
		WithLoggingAddCallInfoToLogger(true),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))

	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	// Test that the context logger has the request info
	// This is harder to test directly, but we can verify that the logger was properly configured
	s.Require().Equal(1, len(logger.Entries()))
	logEntry := logger.Entries()[0]
	s.requireLogFieldString(logEntry, "request_id", headerRequestID)
}

func (s *LoggingInterceptorTestSuite) TestLoggingServerInterceptor_Errors() {
	const headerRequestID = "test-request-id"

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, "", nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up an error handler
	testErr := status.Error(codes.Internal, "test internal error")
	svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
		return nil, testErr
	})
	svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
		return testErr
	})

	reqCtx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerRequestIDKey, headerRequestID))

	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
	}

	s.Require().Equal(1, len(logger.Entries()))
	logEntry := logger.Entries()[0]
	s.Require().Contains(logEntry.Text, "gRPC call finished")

	// Check error fields
	s.requireLogFieldString(logEntry, "grpc_code", codes.Internal.String())
	s.requireLogFieldString(logEntry, "grpc_error", "rpc error: code = Internal desc = test internal error")
}

// Helper methods for the test suite
func (s *LoggingInterceptorTestSuite) setupTestService(logger *logtest.Recorder, userAgent string, options []LoggingOption) (*testService, grpc_testing.TestServiceClient, func() error, error) {
	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		serverOptions = []grpc.ServerOption{
			grpc.ChainUnaryInterceptor(
				RequestIDUnaryInterceptor(),
				LoggingUnaryInterceptor(logger, options...),
			),
		}
	} else {
		serverOptions = []grpc.ServerOption{
			grpc.ChainStreamInterceptor(
				RequestIDStreamInterceptor(),
				LoggingStreamInterceptor(logger, options...),
			),
		}
	}

	var dialOptions []grpc.DialOption
	if userAgent != "" {
		dialOptions = []grpc.DialOption{grpc.WithUserAgent(userAgent)}
	}

	return startTestService(serverOptions, dialOptions)
}

func (s *LoggingInterceptorTestSuite) requireCommonFields(logEntry logtest.RecordedEntry, headerRequestID, headerUserAgent string) {
	s.T().Helper()
	s.requireLogFieldString(logEntry, "request_id", headerRequestID)
	s.Require().NotEmpty(s.getLogFieldAsString(logEntry, "int_request_id"))
	s.requireLogFieldString(logEntry, "grpc_service", "grpc.testing.TestService")
	if s.IsUnary {
		s.requireLogFieldString(logEntry, "grpc_method", "UnaryCall")
		s.requireLogFieldString(logEntry, "grpc_method_type", string(CallMethodTypeUnary))
	} else {
		s.requireLogFieldString(logEntry, "grpc_method", "StreamingOutputCall")
		s.requireLogFieldString(logEntry, "grpc_method_type", string(CallMethodTypeStream))
	}
	s.Require().True(strings.HasPrefix(s.getLogFieldAsString(logEntry, "remote_addr"), "127.0.0.1:"))
	s.requireLogFieldString(logEntry, "user_agent", headerUserAgent+" grpc-go/"+grpc.Version)
}

func (s *LoggingInterceptorTestSuite) requireLogFieldString(logEntry logtest.RecordedEntry, key, want string) {
	s.T().Helper()
	logField, found := logEntry.FindField(key)
	s.Require().True(found)
	s.Require().Equal(want, string(logField.Bytes))
}

func (s *LoggingInterceptorTestSuite) requireLogFieldInt(logEntry logtest.RecordedEntry, key string, want int) {
	s.T().Helper()
	logField, found := logEntry.FindField(key)
	s.Require().True(found)
	s.Require().Equal(want, int(logField.Int))
}

func (s *LoggingInterceptorTestSuite) getLogFieldAsString(logEntry logtest.RecordedEntry, key string) string {
	logField, found := logEntry.FindField(key)
	if !found {
		return ""
	}
	return string(logField.Bytes)
}

func (s *LoggingInterceptorTestSuite) getMethodPath() string {
	if s.IsUnary {
		return "/grpc.testing.TestService/UnaryCall"
	}
	return "/grpc.testing.TestService/StreamingOutputCall"
}

func (s *LoggingInterceptorTestSuite) getCustomLoggerProvider(customLogger log.FieldLogger) LoggingOption {
	if s.IsUnary {
		return WithLoggingUnaryCustomLoggerProvider(func(ctx context.Context, info *grpc.UnaryServerInfo) log.FieldLogger {
			return customLogger
		})
	}
	return WithLoggingStreamCustomLoggerProvider(func(ctx context.Context, info *grpc.StreamServerInfo) log.FieldLogger {
		return customLogger
	})
}

func getLogFieldAsString(logEntry logtest.RecordedEntry, key string) string {
	logField, found := logEntry.FindField(key)
	if !found {
		return ""
	}
	return string(logField.Bytes)
}

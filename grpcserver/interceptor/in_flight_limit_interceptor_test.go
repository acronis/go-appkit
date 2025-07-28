/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
	"github.com/acronis/go-appkit/testutil"
)

// InFlightLimitInterceptorTestSuite is a test suite for InFlightLimit interceptors
type InFlightLimitInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func TestInFlightLimitUnaryInterceptor(t *testing.T) {
	suite.Run(t, &InFlightLimitInterceptorTestSuite{IsUnary: true})
}

func TestInFlightLimitStreamInterceptor(t *testing.T) {
	suite.Run(t, &InFlightLimitInterceptorTestSuite{IsUnary: false})
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_BasicFunctionality() {
	limit := 1
	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	acquired := make(chan struct{})
	release := make(chan struct{})

	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			acquired <- struct{}{}
			<-release
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			acquired <- struct{}{}
			<-release
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		})
	}

	// Start first request
	firstCallErr := make(chan error, 1)
	firstCallDone := make(chan struct{})
	go func() {
		defer close(firstCallDone)
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			if unaryErr != nil {
				firstCallErr <- unaryErr
			}
		} else {
			stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallErr <- streamErr
				return
			}
			if _, recvErr := stream.Recv(); recvErr != nil {
				firstCallErr <- recvErr
			}
		}
	}()

	// Wait for first request to be acquired
	select {
	case <-acquired:
	case <-time.After(5 * time.Second):
		s.Fail("First request did not acquire in-flight limit slot in time")
	}

	// Second request should be rejected
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(unaryErr))
	} else {
		stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr))
	}

	// Release first request
	close(release)

	// Check that the first call completed successfully
	select {
	case <-firstCallDone:
	case <-time.After(5 * time.Second):
		s.Fail("First request did not complete in time")
	}
	testutil.RequireNoErrorInChannel(s.T(), firstCallErr, 5*time.Second)
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_ConcurrentRequests() {
	limit := 3
	concurrentReqs := 10

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	s.setupSlowHandlers(svc, 100*time.Millisecond)

	var okCount, rejectedCount, otherErrsCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < concurrentReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.IsUnary {
				_, unaryErr := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
				if unaryErr != nil {
					if status.Code(unaryErr) == codes.ResourceExhausted {
						rejectedCount.Inc()
					} else {
						otherErrsCount.Inc()
					}
				} else {
					okCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
				if streamErr != nil {
					otherErrsCount.Inc()
					return
				}
				_, recvErr := stream.Recv()
				if recvErr != nil {
					if status.Code(recvErr) == codes.ResourceExhausted {
						rejectedCount.Inc()
					} else {
						otherErrsCount.Inc()
					}
				} else {
					okCount.Inc()
				}
			}
		}()
	}
	wg.Wait()

	// Should allow exactly `limit` requests
	s.Require().Equal(limit, int(okCount.Load()))
	s.Require().Equal(concurrentReqs-limit, int(rejectedCount.Load()))
	s.Require().Equal(0, int(otherErrsCount.Load()))
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_WithGetKey() {
	limit := 2

	getUnaryKeyByClientID := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
				return clientIDs[0], false, nil
			}
		}
		return "", true, nil // bypass if no client-id
	}

	getStreamKeyByClientID := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
				return clientIDs[0], false, nil
			}
		}
		return "", true, nil // bypass if no client-id
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitUnaryGetKey(getUnaryKeyByClientID),
		WithInFlightLimitStreamGetKey(getStreamKeyByClientID),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	s.setupSlowHandlers(svc, 100*time.Millisecond)

	// Test with client-1 - should be limited by key
	client1Ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("client-id", "client-1"))

	var okCount, rejectedCount, otherErrsCount atomic.Int32
	var wg sync.WaitGroup

	// Launch 5 concurrent requests for client-1 - only 2 should succeed (limit=2)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.IsUnary {
				_, unaryErr := client.UnaryCall(client1Ctx, &grpc_testing.SimpleRequest{})
				if unaryErr != nil {
					if status.Code(unaryErr) == codes.ResourceExhausted {
						rejectedCount.Inc()
					} else {
						otherErrsCount.Inc()
					}
				} else {
					okCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(client1Ctx, &grpc_testing.StreamingOutputCallRequest{})
				if streamErr != nil {
					otherErrsCount.Inc()
				}
				_, recvErr := stream.Recv()
				if recvErr != nil {
					if status.Code(recvErr) == codes.ResourceExhausted {
						rejectedCount.Inc()
					} else {
						otherErrsCount.Inc()
					}
				} else {
					okCount.Inc()
				}
			}
		}()
	}
	wg.Wait()

	// Should have exactly limit successful requests and the rest rejected
	s.Require().Equal(limit, int(okCount.Load()))
	s.Require().Equal(5-limit, int(rejectedCount.Load()))
	s.Require().Equal(0, int(otherErrsCount.Load()))

	// Client-2 should have its own separate in-flight limit
	client2Ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("client-id", "client-2"))
	if s.IsUnary {
		_, err = client.UnaryCall(client2Ctx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(client2Ctx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	// Request without client-id should bypass in-flight limiting
	if s.IsUnary {
		_, err = client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
		_, err = client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().NoError(recvErr2)
	}
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_DryRun() {
	limit := 1

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitDryRun(true),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	s.setupSlowHandlers(svc, 100*time.Millisecond)

	reqCtx := context.Background()

	// All requests should succeed in dry run mode
	concurrentReqs := 5
	var okCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < concurrentReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.IsUnary {
				_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
				if unaryErr == nil {
					okCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				if streamErr == nil {
					_, recvErr := stream.Recv()
					if recvErr == nil {
						okCount.Inc()
					}
				}
			}
		}()
	}
	wg.Wait()

	// All requests should succeed
	s.Require().Equal(concurrentReqs, int(okCount.Load()))

	// Should have warning logs about in-flight limit being exceeded
	s.Require().Greater(len(logger.Entries()), 0)
	foundWarning := false
	for _, entry := range logger.Entries() {
		if entry.Level == log.LevelWarn && entry.Text == "in-flight limit exceeded, continuing in dry run mode" {
			foundWarning = true
			break
		}
	}
	s.Require().True(foundWarning)
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_Backlog() {
	limit := 1
	backlogLimit := 1

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitBacklogLimit(backlogLimit),
		WithInFlightLimitBacklogTimeout(time.Second),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	s.setupSlowHandlers(svc, 200*time.Millisecond)

	reqCtx := context.Background()

	// Start first request - it will take 200ms to complete
	firstCallErr := make(chan error, 1)
	firstCallDone := make(chan struct{})
	go func() {
		defer close(firstCallDone)
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
			if unaryErr != nil {
				firstCallErr <- unaryErr
			}
		} else {
			stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallErr <- streamErr
				return
			}
			if _, recvErr := stream.Recv(); recvErr != nil {
				firstCallErr <- recvErr
			}
		}
	}()

	// Give the first request time to start
	time.Sleep(50 * time.Millisecond)

	// Second request should be backlogged and eventually succeed
	startTime := time.Now()
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}
	duration := time.Since(startTime)

	// Should have waited at least 150ms (200ms - 50ms already elapsed)
	s.Require().GreaterOrEqual(duration, 100*time.Millisecond)

	// Check that the first call completed successfully
	select {
	case <-firstCallDone:
	case <-time.After(5 * time.Second):
		s.Fail("First request did not complete in time")
	}
	testutil.RequireNoErrorInChannel(s.T(), firstCallErr, 5*time.Second)
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_BacklogTimeout() {
	limit := 1
	backlogTimeout := 100 * time.Millisecond

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitBacklogLimit(1),
		WithInFlightLimitBacklogTimeout(backlogTimeout),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up very slow handlers to create in-flight requests
	acquired := make(chan struct{})
	release := make(chan struct{})

	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			acquired <- struct{}{}
			<-release
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			acquired <- struct{}{}
			<-release
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		})
	}

	reqCtx := context.Background()

	// Start first request
	firstCallErr := make(chan error, 1)
	firstCallDone := make(chan struct{})
	go func() {
		defer close(firstCallDone)
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
			if unaryErr != nil {
				firstCallErr <- unaryErr
			}
		} else {
			stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallErr <- streamErr
				return
			}
			if _, recvErr := stream.Recv(); recvErr != nil {
				firstCallErr <- recvErr
			}
		}
	}()

	// Wait for first request to be acquired
	<-acquired

	// Second request should timeout in backlog
	startTime := time.Now()
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(unaryErr))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr))
	}
	duration := time.Since(startTime)

	// Should have timed out after the backlog timeout
	s.Require().GreaterOrEqual(duration, backlogTimeout)
	s.Require().LessOrEqual(duration, backlogTimeout+100*time.Millisecond) // Some tolerance

	// Release first request
	close(release)

	// Check that the first call completed successfully
	select {
	case <-firstCallDone:
	case <-time.After(5 * time.Second):
		s.Fail("First request did not complete in time")
	}
	testutil.RequireNoErrorInChannel(s.T(), firstCallErr, 5*time.Second)
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_CustomCallbacks() {
	limit := 1

	var rejectedCalled bool
	customUnaryOnReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams) (interface{}, error) {
		rejectedCalled = true
		return nil, status.Error(codes.Unavailable, "custom rejection message")
	}
	customStreamOnReject := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams) error {
		rejectedCalled = true
		return status.Error(codes.Unavailable, "custom rejection message")
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitUnaryOnReject(customUnaryOnReject),
		WithInFlightLimitStreamOnReject(customStreamOnReject),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	acquired := make(chan struct{})
	release := make(chan struct{})

	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			acquired <- struct{}{}
			<-release
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			acquired <- struct{}{}
			<-release
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		})
	}

	reqCtx := context.Background()

	// Start first request
	firstCallErr := make(chan error, 1)
	firstCallDone := make(chan struct{})
	go func() {
		defer close(firstCallDone)
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
			if unaryErr != nil {
				firstCallErr <- unaryErr
			}
		} else {
			stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallErr <- streamErr
				return
			}
			if _, recvErr := stream.Recv(); recvErr != nil {
				firstCallErr <- recvErr
			}
		}
	}()

	// Wait for first request to be acquired
	<-acquired

	// Second request should be rejected with custom callback
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.Unavailable, status.Code(unaryErr))
		s.Require().Contains(unaryErr.Error(), "custom rejection message")
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.Unavailable, status.Code(recvErr))
		s.Require().Contains(recvErr.Error(), "custom rejection message")
	}

	s.Require().True(rejectedCalled)

	// Release first request
	close(release)

	// Check that the first call completed successfully
	select {
	case <-firstCallDone:
	case <-time.After(5 * time.Second):
		s.Fail("First request did not complete in time")
	}
	testutil.RequireNoErrorInChannel(s.T(), firstCallErr, 5*time.Second)
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_RetryAfterHeader() {
	limit := 1
	retryAfter := 3 * time.Second

	customUnaryGetRetryAfter := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) time.Duration {
		return retryAfter
	}
	customStreamGetRetryAfter := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) time.Duration {
		return retryAfter
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitUnaryGetRetryAfter(customUnaryGetRetryAfter),
		WithInFlightLimitStreamGetRetryAfter(customStreamGetRetryAfter),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	acquired := make(chan struct{})
	release := make(chan struct{})

	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			acquired <- struct{}{}
			<-release
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			acquired <- struct{}{}
			<-release
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		})
	}

	reqCtx := context.Background()

	// Start first request
	firstCallErr := make(chan error, 1)
	firstCallDone := make(chan struct{})
	go func() {
		defer close(firstCallDone)
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
			if unaryErr != nil {
				firstCallErr <- unaryErr
			}
		} else {
			stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallErr <- streamErr
				return
			}
			if _, recvErr := stream.Recv(); recvErr != nil {
				firstCallErr <- recvErr
			}
		}
	}()

	// Wait for first request to be acquired
	<-acquired

	// Second request should be rejected with retry-after header
	var headers metadata.MD
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{}, grpc.Header(&headers))
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(unaryErr))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{}, grpc.Header(&headers))
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr))
	}

	// Check retry-after header
	retryAfterHeaders := headers.Get("retry-after")
	s.Require().Len(retryAfterHeaders, 1)
	retryAfterSecs, parseErr := strconv.Atoi(retryAfterHeaders[0])
	s.Require().NoError(parseErr)
	s.Require().Equal(int(retryAfter.Seconds()), retryAfterSecs)

	// Release first request
	close(release)

	// Check that the first call completed successfully
	select {
	case <-firstCallDone:
	case <-time.After(5 * time.Second):
		s.Fail("First request did not complete in time")
	}
	testutil.RequireNoErrorInChannel(s.T(), firstCallErr, 5*time.Second)
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_InvalidOptions() {
	tests := []struct {
		name        string
		limit       int
		options     []InFlightLimitOption
		expectError string
	}{
		{
			name:        "zero limit",
			limit:       0,
			expectError: "limit should be positive",
		},
		{
			name:        "negative limit",
			limit:       -1,
			expectError: "limit should be positive",
		},
		{
			name:        "negative backlog limit",
			limit:       1,
			options:     []InFlightLimitOption{WithInFlightLimitBacklogLimit(-1)},
			expectError: "backlog limit should not be negative",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var err error
			if s.IsUnary {
				_, err = InFlightLimitUnaryInterceptor(tt.limit, tt.options...)
			} else {
				_, err = InFlightLimitStreamInterceptor(tt.limit, tt.options...)
			}
			s.Require().Error(err)
			s.Require().Contains(err.Error(), tt.expectError)
		})
	}
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_DefaultHandlers() {
	// Test default reject handler with mock context and parameters
	ctx := context.Background()
	logger := logtest.NewRecorder()
	ctx = NewContextWithLogger(ctx, logger)

	params := InFlightLimitParams{
		Key:               "test-key",
		RequestBacklogged: false,
	}

	// Test DefaultInFlightLimitUnaryOnReject
	if s.IsUnary {
		result, err := DefaultInFlightLimitUnaryOnReject(ctx, &grpc_testing.SimpleRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, nil, params)
		s.Require().Error(err)
		s.Nil(result)
		s.Equal(codes.ResourceExhausted, status.Code(err))
		s.Contains(err.Error(), "Too many in-flight requests")
	} else {
		// Test DefaultInFlightLimitStreamOnReject
		mockStream := &mockServerStream{ctx: ctx}
		err := DefaultInFlightLimitStreamOnReject(nil, mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/Method"}, nil, params)
		s.Require().Error(err)
		s.Equal(codes.ResourceExhausted, status.Code(err))
		s.Contains(err.Error(), "Too many in-flight requests")
	}

	// Verify warning log was created
	entries := logger.Entries()
	s.Require().Greater(len(entries), 0)
	found := false
	for _, entry := range entries {
		if entry.Level == log.LevelWarn && entry.Text == "in-flight limit exceeded" {
			found = true
			break
		}
	}
	s.True(found, "Should log in-flight limit exceeded warning")
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_DefaultErrorHandlers() {
	ctx := context.Background()
	logger := logtest.NewRecorder()
	ctx = NewContextWithLogger(ctx, logger)

	params := InFlightLimitParams{
		Key:               "test-key",
		RequestBacklogged: false,
	}

	testErr := status.Error(codes.Internal, "test error")

	// Test DefaultInFlightLimitUnaryOnError
	if s.IsUnary {
		result, err := DefaultInFlightLimitUnaryOnError(ctx, &grpc_testing.SimpleRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, nil, params, testErr)
		s.Require().Error(err)
		s.Nil(result)
		s.Equal(codes.Internal, status.Code(err))
		s.Contains(err.Error(), "Internal server error")
	} else {
		// Test DefaultInFlightLimitStreamOnError
		mockStream := &mockServerStream{ctx: ctx}
		err := DefaultInFlightLimitStreamOnError(nil, mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/Method"}, nil, params, testErr)
		s.Require().Error(err)
		s.Equal(codes.Internal, status.Code(err))
		s.Contains(err.Error(), "Internal server error")
	}

	// Verify error log was created
	entries := logger.Entries()
	s.Require().Greater(len(entries), 0)
	found := false
	for _, entry := range entries {
		if entry.Level == log.LevelError && entry.Text == "in-flight limiting error" {
			found = true
			break
		}
	}
	s.True(found, "Should log in-flight limiting error")
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_DryRunHandlers() {
	ctx := context.Background()
	logger := logtest.NewRecorder()
	ctx = NewContextWithLogger(ctx, logger)

	params := InFlightLimitParams{
		Key:               "test-key",
		RequestBacklogged: false,
	}

	handlerCalled := false
	mockHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		handlerCalled = true
		return "success", nil
	}
	mockStreamHandler := func(srv interface{}, ss grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	// Test DefaultInFlightLimitUnaryOnRejectInDryRun
	if s.IsUnary {
		result, err := DefaultInFlightLimitUnaryOnRejectInDryRun(ctx, &grpc_testing.SimpleRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, mockHandler, params)
		s.Require().NoError(err)
		s.Equal("success", result)
		s.True(handlerCalled)
	} else {
		// Test DefaultInFlightLimitStreamOnRejectInDryRun
		mockStream := &mockServerStream{ctx: ctx}
		err := DefaultInFlightLimitStreamOnRejectInDryRun(nil, mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/Method"}, mockStreamHandler, params)
		s.Require().NoError(err)
		s.True(handlerCalled)
	}

	// Verify dry run warning log was created
	entries := logger.Entries()
	s.Require().Greater(len(entries), 0)
	found := false
	for _, entry := range entries {
		if entry.Level == log.LevelWarn && entry.Text == "in-flight limit exceeded, continuing in dry run mode" {
			found = true
			break
		}
	}
	s.True(found, "Should log dry run warning")
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_GetKeyError() {
	limit := 1

	// Create get key functions that return an error
	errorUnaryGetKey := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
		return "", false, status.Error(codes.InvalidArgument, "key extraction failed")
	}
	errorStreamGetKey := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
		return "", false, status.Error(codes.InvalidArgument, "key extraction failed")
	}

	var options []InFlightLimitOption
	if s.IsUnary {
		options = []InFlightLimitOption{WithInFlightLimitUnaryGetKey(errorUnaryGetKey)}
	} else {
		options = []InFlightLimitOption{WithInFlightLimitStreamGetKey(errorStreamGetKey)}
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, limit, options)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// Request should fail due to key extraction error
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Equal(codes.Internal, status.Code(unaryErr))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Equal(codes.Internal, status.Code(recvErr))
	}
}

func (s *InFlightLimitInterceptorTestSuite) TestInFlightLimitInterceptor_MaxKeys() {
	limit := 1
	maxKeys := 1

	getUnaryKeyByIndex := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if indexes := md.Get("index"); len(indexes) > 0 {
				return indexes[0], false, nil
			}
		}
		return "default", false, nil
	}

	getStreamKeyByIndex := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			if indexes := md.Get("index"); len(indexes) > 0 {
				return indexes[0], false, nil
			}
		}
		return "default", false, nil
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitUnaryGetKey(getUnaryKeyByIndex),
		WithInFlightLimitStreamGetKey(getStreamKeyByIndex),
		WithInFlightLimitMaxKeys(maxKeys),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// With maxKeys=1, only one key can be tracked at a time
	// When a new key is used, the old one should be evicted from the LRU cache

	ctx1 := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("index", "1"))
	ctx2 := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("index", "2"))

	// Make request with key "1" - should succeed
	if s.IsUnary {
		_, err = client.UnaryCall(ctx1, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(ctx1, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	// Make request with key "2" - should succeed (evicts key "1" from cache)
	if s.IsUnary {
		_, err = client.UnaryCall(ctx2, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(ctx2, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	// Make another request with key "1" - should succeed because key "1" was evicted and recreated
	if s.IsUnary {
		_, err = client.UnaryCall(ctx1, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(ctx1, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}
}

// Helper methods

func (s *InFlightLimitInterceptorTestSuite) setupSlowHandlers(svc *testService, delay time.Duration) {
	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			time.Sleep(delay)
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			time.Sleep(delay)
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		})
	}
}

func (s *InFlightLimitInterceptorTestSuite) setupTestService(
	logger *logtest.Recorder, limit int, options []InFlightLimitOption,
) (*testService, grpc_testing.TestServiceClient, func() error, error) {
	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := InFlightLimitUnaryInterceptor(limit, options...)
		if err != nil {
			return nil, nil, nil, err
		}
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := InFlightLimitStreamInterceptor(limit, options...)
		if err != nil {
			return nil, nil, nil, err
		}
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &WrappedServerStream{ServerStream: ss, Ctx: NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	return startTestService(serverOptions, nil)
}

func (s *InFlightLimitInterceptorTestSuite) TestWithInFlightLimitOnRejectInDryRun() {
	limit := 1

	var customHandlerCalls atomic.Int32

	customUnaryRejectInDryRun := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams) (interface{}, error) {
		customHandlerCalls.Inc()
		return handler(ctx, req)
	}

	customStreamRejectInDryRun := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams) error {
		customHandlerCalls.Inc()
		return handler(srv, ss)
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitDryRun(true),
		WithInFlightLimitUnaryOnRejectInDryRun(customUnaryRejectInDryRun),
		WithInFlightLimitStreamOnRejectInDryRun(customStreamRejectInDryRun),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	s.setupSlowHandlers(svc, 100*time.Millisecond)

	reqCtx := context.Background()

	// Launch multiple concurrent requests to trigger dry run rejection
	var wg sync.WaitGroup
	concurrentReqs := 3
	var successCount atomic.Int32

	for i := 0; i < concurrentReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var headers metadata.MD
			if s.IsUnary {
				_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{}, grpc.Header(&headers))
				if unaryErr == nil {
					successCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{}, grpc.Header(&headers))
				if streamErr == nil {
					if _, recvErr := stream.Recv(); recvErr == nil {
						successCount.Inc()
					}
				}
			}
		}()
	}
	wg.Wait()

	// All requests should succeed in dry run mode
	s.Require().Equal(concurrentReqs, int(successCount.Load()), "All requests should succeed in dry run mode")
	s.Require().Equal(concurrentReqs-1, int(customHandlerCalls.Load()), "Custom dry run reject handler should have been called")
}

func (s *InFlightLimitInterceptorTestSuite) TestWithInFlightLimitOnError() {
	limit := 1

	customUnaryOnError := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams, err error) (interface{}, error) {
		// Custom error handling: log the error with custom context and return a different error
		logger := GetLoggerFromContext(ctx)
		if logger != nil {
			logger.Error("custom in-flight limit error handler",
				log.String("method", info.FullMethod),
				log.String("key", params.Key),
				log.Error(err),
			)
		}
		return nil, status.Error(codes.FailedPrecondition, "custom error response")
	}

	customStreamOnError := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams, err error) error {
		// Custom error handling: log the error with custom context and return a different error
		logger := GetLoggerFromContext(ss.Context())
		if logger != nil {
			logger.Error("custom in-flight limit error handler",
				log.String("method", info.FullMethod),
				log.String("key", params.Key),
				log.Error(err),
			)
		}
		return status.Error(codes.FailedPrecondition, "custom error response")
	}

	// Create get key functions that will cause an error
	errorUnaryGetKey := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
		return "", false, status.Error(codes.InvalidArgument, "key extraction failed")
	}

	errorStreamGetKey := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
		return "", false, status.Error(codes.InvalidArgument, "key extraction failed")
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, limit, []InFlightLimitOption{
		WithInFlightLimitUnaryGetKey(errorUnaryGetKey),
		WithInFlightLimitStreamGetKey(errorStreamGetKey),
		WithInFlightLimitUnaryOnError(customUnaryOnError),
		WithInFlightLimitStreamOnError(customStreamOnError),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// Request should fail with custom error response
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.FailedPrecondition, status.Code(unaryErr))
		s.Require().Contains(unaryErr.Error(), "custom error response")
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.FailedPrecondition, status.Code(recvErr))
		s.Require().Contains(recvErr.Error(), "custom error response")
	}

	// Check that custom error handler was called by verifying log entry
	entries := logger.Entries()
	s.Require().Greater(len(entries), 0)
	found := false
	for _, entry := range entries {
		if entry.Level == log.LevelError && entry.Text == "custom in-flight limit error handler" {
			found = true
			break
		}
	}
	s.Require().True(found, "Custom error handler should have been called")
}

func (s *InFlightLimitInterceptorTestSuite) TestWithInFlightLimitDryRunCallbackPriority() {
	limit := 1

	var regularRejectCalled atomic.Bool
	var dryRunRejectCalled atomic.Bool

	regularUnaryOnReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams) (interface{}, error) {
		regularRejectCalled.Store(true)
		return nil, status.Error(codes.ResourceExhausted, "regular rejection")
	}

	dryRunUnaryOnReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams) (interface{}, error) {
		dryRunRejectCalled.Store(true)
		return handler(ctx, req)
	}

	regularStreamOnReject := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams) error {
		regularRejectCalled.Store(true)
		return status.Error(codes.ResourceExhausted, "regular rejection")
	}

	dryRunStreamOnReject := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams) error {
		dryRunRejectCalled.Store(true)
		return handler(srv, ss)
	}

	logger := logtest.NewRecorder()
	options := []InFlightLimitOption{
		WithInFlightLimitDryRun(true),
		WithInFlightLimitUnaryOnReject(regularUnaryOnReject),
		WithInFlightLimitStreamOnReject(regularStreamOnReject),
		WithInFlightLimitUnaryOnRejectInDryRun(dryRunUnaryOnReject),
		WithInFlightLimitStreamOnRejectInDryRun(dryRunStreamOnReject),
	}

	svc, client, closeSvc, err := s.setupTestService(logger, limit, options)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up slow handlers to create in-flight requests
	s.setupSlowHandlers(svc, 100*time.Millisecond)

	reqCtx := context.Background()

	// Launch multiple concurrent requests to trigger dry run rejection
	var wg sync.WaitGroup
	concurrentReqs := 3
	var successCount atomic.Int32

	for i := 0; i < concurrentReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.IsUnary {
				_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
				if unaryErr == nil {
					successCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				if streamErr == nil {
					_, recvErr := stream.Recv()
					if recvErr == nil {
						successCount.Inc()
					}
				}
			}
		}()
	}
	wg.Wait()

	// All requests should succeed in dry run mode
	s.Require().Equal(concurrentReqs, int(successCount.Load()), "All requests should succeed in dry run mode")

	// Verify that dry run callback was called, not regular callback
	s.Require().True(dryRunRejectCalled.Load(), "Dry run callback should have been called")
	s.Require().False(regularRejectCalled.Load(), "Regular callback should NOT have been called in dry run mode")
}

func (s *InFlightLimitInterceptorTestSuite) TestWithInFlightLimitErrorCallbackWithParams() {
	limit := 1

	var receivedParams InFlightLimitParams
	var receivedError error

	customUnaryOnError := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams, err error) (interface{}, error) {
		receivedParams = params
		receivedError = err
		return nil, status.Error(codes.Aborted, "test completed")
	}

	customStreamOnError := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams, err error) error {
		receivedParams = params
		receivedError = err
		return status.Error(codes.Aborted, "test completed")
	}

	// Create a get key function that will cause a specific error
	testKey := "test-key-123"
	errorGetKey := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
		return testKey, false, status.Error(codes.PermissionDenied, "permission denied for key extraction")
	}
	errorStreamGetKey := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
		return testKey, false, status.Error(codes.PermissionDenied, "permission denied for key extraction")
	}

	logger := logtest.NewRecorder()
	options := []InFlightLimitOption{
		WithInFlightLimitUnaryOnError(customUnaryOnError),
		WithInFlightLimitStreamOnError(customStreamOnError),
	}

	if s.IsUnary {
		options = append(options, WithInFlightLimitUnaryGetKey(errorGetKey))
	} else {
		options = append(options, WithInFlightLimitStreamGetKey(errorStreamGetKey))
	}

	_, client, closeSvc, err := s.setupTestService(logger, limit, options)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// Request should trigger error callback
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.Aborted, status.Code(unaryErr))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.Aborted, status.Code(recvErr))
	}

	// Verify that error callback received correct parameters
	s.Require().Equal(testKey, receivedParams.Key, "Error callback should receive correct key")
	s.Require().NotNil(receivedError, "Error callback should receive the original error")
	s.Require().Equal(codes.PermissionDenied, status.Code(receivedError), "Error callback should receive original error code")
	s.Require().Contains(receivedError.Error(), "permission denied for key extraction", "Error callback should receive original error message")
}

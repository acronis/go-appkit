/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"math"
	"net"
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
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/log/logtest"
)

// RateLimitInterceptorTestSuite is a test suite for RateLimit interceptors
type RateLimitInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func TestRateLimitUnaryInterceptor(t *testing.T) {
	suite.Run(t, &RateLimitInterceptorTestSuite{IsUnary: true})
}

func TestRateLimitStreamInterceptor(t *testing.T) {
	suite.Run(t, &RateLimitInterceptorTestSuite{IsUnary: false})
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_BasicFunctionality() {
	tests := []struct {
		name    string
		rate    Rate
		options []RateLimitOption
		alg     RateLimitAlg
	}{
		{
			name:    "leaky bucket algorithm",
			rate:    Rate{1, time.Second},
			options: []RateLimitOption{WithRateLimitAlg(RateLimitAlgLeakyBucket)},
		},
		{
			name:    "sliding window algorithm",
			rate:    Rate{1, time.Second},
			options: []RateLimitOption{WithRateLimitAlg(RateLimitAlgSlidingWindow)},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			logger := logtest.NewRecorder()
			_, client, closeSvc, err := s.setupTestService(logger, tt.rate, tt.options)
			s.Require().NoError(err)
			defer func() { s.Require().NoError(closeSvc()) }()

			reqCtx := context.Background()

			if s.IsUnary {
				_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
				s.Require().NoError(unaryErr)
				// Second request should be rejected
				_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
				s.Require().Error(err)
				s.Require().Equal(codes.ResourceExhausted, status.Code(err))
			} else {
				stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(streamErr)
				_, recvErr := stream.Recv()
				s.Require().NoError(recvErr)
				// Second request should be rejected
				stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
				s.Require().NoError(streamErr2)
				_, recvErr2 := stream2.Recv()
				s.Require().Error(recvErr2)
				s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
			}
		})
	}
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_LeakyBucket() {
	rate := Rate{5, time.Second} // 5 requests per second
	maxBurst := 5

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitAlg(RateLimitAlgLeakyBucket),
		WithRateLimitMaxBurst(maxBurst),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Send concurrent requests up to burst limit
	concurrentReqs := 10
	var okCount, rejectedCount atomic.Int32
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
					}
				} else {
					okCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
				if streamErr != nil {
					rejectedCount.Inc()
					return
				}
				_, recvErr := stream.Recv()
				if recvErr != nil {
					if status.Code(recvErr) == codes.ResourceExhausted {
						rejectedCount.Inc()
					}
				} else {
					okCount.Inc()
				}
			}
		}()
	}
	wg.Wait()

	// Should allow maxBurst + 1 requests (initial bucket capacity + 1 emission)
	s.Require().Equal(maxBurst+1, int(okCount.Load()))
	s.Require().Equal(concurrentReqs-maxBurst-1, int(rejectedCount.Load()))
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_SlidingWindow() {
	rate := Rate{2, time.Second} // 2 requests per second

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitAlg(RateLimitAlgSlidingWindow),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Send concurrent requests
	concurrentReqs := 5
	var okCount, rejectedCount atomic.Int32
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
					}
				} else {
					okCount.Inc()
				}
			} else {
				stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
				if streamErr != nil {
					rejectedCount.Inc()
					return
				}
				_, recvErr := stream.Recv()
				if recvErr != nil {
					if status.Code(recvErr) == codes.ResourceExhausted {
						rejectedCount.Inc()
					}
				} else {
					okCount.Inc()
				}
			}
		}()
	}
	wg.Wait()

	// Should allow exactly 'rate.Count' requests in the sliding window
	s.Require().Equal(rate.Count, int(okCount.Load()))
	s.Require().Equal(concurrentReqs-rate.Count, int(rejectedCount.Load()))
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_WithGetKey() {
	rate := Rate{1, time.Second}

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
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitUnaryGetKey(getUnaryKeyByClientID),
		WithRateLimitStreamGetKey(getStreamKeyByClientID),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Client 1 requests
	client1Ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("client-id", "client-1"))
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(client1Ctx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		_, unaryErr2 := client.UnaryCall(client1Ctx, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(unaryErr2))
	} else {
		stream, streamErr := client.StreamingOutputCall(client1Ctx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client.StreamingOutputCall(client1Ctx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}

	// Client 2 should have its own rate limit
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

	// Request without client-id should bypass rate limiting
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

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_DryRun() {
	rate := Rate{1, time.Second}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitDryRun(true),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// All requests should succeed in dry run mode
	for i := 0; i < 5; i++ {
		if s.IsUnary {
			_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
			s.Require().NoError(err)
		} else {
			stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			s.Require().NoError(streamErr)
			_, recvErr := stream.Recv()
			s.Require().NoError(recvErr)
		}
	}

	// Should have warning logs about rate limit being exceeded
	s.Require().Greater(len(logger.Entries()), 0)
	foundWarning := false
	for _, entry := range logger.Entries() {
		if entry.Level == log.LevelWarn && entry.Text == "rate limit exceeded, continuing in dry run mode" {
			foundWarning = true
			break
		}
	}
	s.Require().True(foundWarning)
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_Backlog() {
	rate := Rate{1, time.Second}
	backlogLimit := 1

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitBacklogLimit(backlogLimit),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request should succeed immediately
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

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

	// Should have waited approximately the rate limit duration
	s.Require().GreaterOrEqual(duration, time.Millisecond*800) // Allow some tolerance
	s.Require().LessOrEqual(duration, time.Millisecond*1200)   // Allow some tolerance
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_BacklogTimeout() {
	rate := Rate{1, time.Minute} // Very slow rate
	backlogTimeout := 100 * time.Millisecond

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitBacklogLimit(1),
		WithRateLimitBacklogTimeout(backlogTimeout),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request should succeed
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

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
	s.Require().LessOrEqual(duration, backlogTimeout+50*time.Millisecond) // Some tolerance
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_CustomCallbacks() {
	rate := Rate{1, time.Second}

	var rejectedCalled bool
	customUnaryOnReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams) (interface{}, error) {
		rejectedCalled = true
		return nil, status.Error(codes.Unavailable, "custom rejection message")
	}
	customStreamOnReject := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams) error {
		rejectedCalled = true
		return status.Error(codes.Unavailable, "custom rejection message")
	}
	customUnaryOnError := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams, err error) (interface{}, error) {
		return nil, status.Error(codes.Aborted, "custom error message")
	}
	customStreamOnError := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams, err error) error {
		return status.Error(codes.Aborted, "custom error message")
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, []RateLimitOption{
		WithRateLimitUnaryOnReject(customUnaryOnReject),
		WithRateLimitStreamOnReject(customStreamOnReject),
		WithRateLimitUnaryOnError(customUnaryOnError),
		WithRateLimitStreamOnError(customStreamOnError),
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request should succeed
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

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
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_RetryAfterHeader() {
	rate := Rate{1, time.Second}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request should succeed
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

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
	s.Require().Greater(retryAfterSecs, 0)
	s.Require().LessOrEqual(retryAfterSecs, int(math.Ceil(rate.Duration.Seconds())))
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_InvalidOptions() {
	rate := Rate{1, time.Second}

	// Test negative backlog limit
	var err error
	if s.IsUnary {
		_, err = RateLimitUnaryInterceptor(rate, WithRateLimitBacklogLimit(-1))
	} else {
		_, err = RateLimitStreamInterceptor(rate, WithRateLimitBacklogLimit(-1))
	}
	s.Require().Error(err)
	s.Require().Contains(err.Error(), "backlog limit should not be negative")
}

// Helper methods

func (s *RateLimitInterceptorTestSuite) setupTestService(
	logger *logtest.Recorder, rate Rate, options []RateLimitOption,
) (*testService, grpc_testing.TestServiceClient, func() error, error) {
	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := RateLimitUnaryInterceptor(rate, options...)
		if err != nil {
			return nil, nil, nil, err
		}
		loggingInceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := RateLimitStreamInterceptor(rate, options...)
		if err != nil {
			return nil, nil, nil, err
		}
		loggingInceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &WrappedServerStream{ServerStream: ss, Ctx: NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInceptor, streamInterceptor))
	}

	return startTestService(serverOptions, nil)
}

func (s *RateLimitInterceptorTestSuite) createRateLimitInterceptor(
	rate Rate, options []RateLimitOption,
) (func(context.Context, interface{}, *grpc.UnaryServerInfo, grpc.UnaryHandler) (interface{}, error), error) {
	if s.IsUnary {
		return RateLimitUnaryInterceptor(rate, options...)
	}
	// For stream tests, we create a dummy unary interceptor just for error testing
	return RateLimitUnaryInterceptor(rate, options...)
}

// rateLimitGetKeyByIP contains the shared logic for extracting client IP
func rateLimitGetKeyByIP(ctx context.Context) (string, bool, error) {
	if p, ok := peer.FromContext(ctx); ok {
		if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
			return host, false, nil
		}
		return p.Addr.String(), false, nil
	}
	return "", true, nil // Bypass if no peer info available
}

// rateLimitUnaryGetKeyByIP extracts client IP from unary gRPC requests for rate limiting.
func rateLimitUnaryGetKeyByIP(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
	return rateLimitGetKeyByIP(ctx)
}

// rateLimitStreamGetKeyByIP extracts client IP from stream gRPC requests for rate limiting.
func rateLimitStreamGetKeyByIP(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
	return rateLimitGetKeyByIP(ss.Context())
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_GetRetryAfter() {
	// Custom GetRetryAfter functions that double the estimated time
	customUnaryGetRetryAfter := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, estimatedTime time.Duration) time.Duration {
		return estimatedTime * 2
	}
	customStreamGetRetryAfter := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, estimatedTime time.Duration) time.Duration {
		return estimatedTime * 2
	}

	var options []RateLimitOption
	if s.IsUnary {
		options = []RateLimitOption{
			WithRateLimitUnaryGetKey(rateLimitUnaryGetKeyByIP),
			WithRateLimitUnaryGetRetryAfter(customUnaryGetRetryAfter),
		}
	} else {
		options = []RateLimitOption{
			WithRateLimitStreamGetKey(rateLimitStreamGetKeyByIP),
			WithRateLimitStreamGetRetryAfter(customStreamGetRetryAfter),
		}
	}

	// Very low rate to trigger rate limiting quickly
	rate := Rate{1, time.Second}

	logger := logtest.NewRecorder()
	_, testServiceClient, tearDown, err := s.setupTestService(logger, rate, options)
	s.Require().NoError(err)
	defer tearDown()

	reqCtx := context.Background()
	// Make first request - should succeed
	if s.IsUnary {
		_, err := testServiceClient.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err, "First request should succeed")
	} else {
		stream, err := testServiceClient.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(err, "Should create stream")
		_, err = stream.Recv()
		s.Require().NoError(err, "Should receive from stream")
	}

	// Make second request immediately - should be rate limited
	var md metadata.MD
	if s.IsUnary {
		_, err := testServiceClient.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{}, grpc.Header(&md))
		s.Require().Error(err, "Second request should be rate limited")
		s.Assert().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		stream, err := testServiceClient.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{}, grpc.Header(&md))
		s.Require().NoError(err, "Should create stream")
		_, err = stream.Recv()
		s.Require().Error(err, "Second stream should be rate limited")
		s.Assert().Equal(codes.ResourceExhausted, status.Code(err))
	}

	// Check that retry-after header is set with custom value (doubled)
	retryAfterHeaders := md.Get("retry-after")
	s.Require().Len(retryAfterHeaders, 1, "Should have retry-after header")

	retryAfterSeconds, err := strconv.Atoi(retryAfterHeaders[0])
	s.Require().NoError(err, "Should parse retry-after header")

	// Since we doubled the retry time, and the rate is 1 request per second,
	// the retry-after should be at least 2 seconds (the custom function doubles the estimated time)
	s.Assert().GreaterOrEqual(retryAfterSeconds, 2, "Custom GetRetryAfter should have doubled the retry time")
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_ErrorHandling() {
	tests := []struct {
		name        string
		rate        Rate
		options     []RateLimitOption
		expectError string
	}{
		{
			name:        "invalid rate - zero duration",
			rate:        Rate{1, 0},
			expectError: "MaxRate must be greater than zero",
		},
		{
			name:        "negative backlog limit",
			rate:        Rate{1, time.Second},
			options:     []RateLimitOption{WithRateLimitBacklogLimit(-1)},
			expectError: "backlog limit should not be negative",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var err error
			if s.IsUnary {
				_, err = RateLimitUnaryInterceptor(tt.rate, tt.options...)
			} else {
				_, err = RateLimitStreamInterceptor(tt.rate, tt.options...)
			}
			s.Require().Error(err)
			s.Contains(err.Error(), tt.expectError)
		})
	}
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_DefaultHandlers() {
	// Test default reject handler with mock context and parameters
	ctx := context.Background()
	logger := logtest.NewRecorder()
	ctx = NewContextWithLogger(ctx, logger)

	params := RateLimitParams{
		Key:                 "test-key",
		RequestBacklogged:   false,
		EstimatedRetryAfter: time.Second,
	}

	// Test DefaultRateLimitUnaryOnReject
	if s.IsUnary {
		result, err := DefaultRateLimitUnaryOnReject(ctx, &grpc_testing.SimpleRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, nil, params)
		s.Require().Error(err)
		s.Nil(result)
		s.Equal(codes.ResourceExhausted, status.Code(err))
		s.Contains(err.Error(), "Too many requests")
	} else {
		// Test DefaultRateLimitStreamOnReject
		mockStream := &mockServerStream{ctx: ctx}
		err := DefaultRateLimitStreamOnReject(nil, mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/Method"}, nil, params)
		s.Require().Error(err)
		s.Equal(codes.ResourceExhausted, status.Code(err))
		s.Contains(err.Error(), "Too many requests")
	}

	// Verify warning log was created
	entries := logger.Entries()
	s.Require().Greater(len(entries), 0)
	found := false
	for _, entry := range entries {
		if entry.Level == log.LevelWarn && entry.Text == "rate limit exceeded" {
			found = true
			break
		}
	}
	s.True(found, "Should log rate limit exceeded warning")
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_DefaultErrorHandlers() {
	ctx := context.Background()
	logger := logtest.NewRecorder()
	ctx = NewContextWithLogger(ctx, logger)

	params := RateLimitParams{
		Key:                 "test-key",
		RequestBacklogged:   false,
		EstimatedRetryAfter: time.Second,
	}

	testErr := status.Error(codes.Internal, "test error")

	// Test DefaultRateLimitUnaryOnError
	if s.IsUnary {
		result, err := DefaultRateLimitUnaryOnError(ctx, &grpc_testing.SimpleRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, nil, params, testErr)
		s.Require().Error(err)
		s.Nil(result)
		s.Equal(codes.Internal, status.Code(err))
		s.Contains(err.Error(), "Internal server error")
	} else {
		// Test DefaultRateLimitStreamOnError
		mockStream := &mockServerStream{ctx: ctx}
		err := DefaultRateLimitStreamOnError(nil, mockStream,
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
		if entry.Level == log.LevelError && entry.Text == "rate limiting error" {
			found = true
			break
		}
	}
	s.True(found, "Should log rate limiting error")
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_DryRunHandlers() {
	ctx := context.Background()
	logger := logtest.NewRecorder()
	ctx = NewContextWithLogger(ctx, logger)

	params := RateLimitParams{
		Key:                 "test-key",
		RequestBacklogged:   false,
		EstimatedRetryAfter: time.Second,
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

	// Test DefaultRateLimitUnaryOnRejectInDryRun
	if s.IsUnary {
		result, err := DefaultRateLimitUnaryOnRejectInDryRun(ctx, &grpc_testing.SimpleRequest{},
			&grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, mockHandler, params)
		s.Require().NoError(err)
		s.Equal("success", result)
		s.True(handlerCalled)
	} else {
		// Test DefaultRateLimitStreamOnRejectInDryRun
		mockStream := &mockServerStream{ctx: ctx}
		err := DefaultRateLimitStreamOnRejectInDryRun(nil, mockStream,
			&grpc.StreamServerInfo{FullMethod: "/test.Service/Method"}, mockStreamHandler, params)
		s.Require().NoError(err)
		s.True(handlerCalled)
	}

	// Verify dry run warning log was created
	entries := logger.Entries()
	s.Require().Greater(len(entries), 0)
	found := false
	for _, entry := range entries {
		if entry.Level == log.LevelWarn && entry.Text == "rate limit exceeded, continuing in dry run mode" {
			found = true
			break
		}
	}
	s.True(found, "Should log dry run warning")
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_GetKeyError() {
	rate := Rate{1, time.Second}

	// Create get key functions that return an error
	errorUnaryGetKey := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
		return "", false, status.Error(codes.InvalidArgument, "key extraction failed")
	}
	errorStreamGetKey := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
		return "", false, status.Error(codes.InvalidArgument, "key extraction failed")
	}

	var options []RateLimitOption
	if s.IsUnary {
		options = []RateLimitOption{WithRateLimitUnaryGetKey(errorUnaryGetKey)}
	} else {
		options = []RateLimitOption{WithRateLimitStreamGetKey(errorStreamGetKey)}
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, options)
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

// mockServerStream is a mock implementation of grpc.ServerStream for testing
type mockServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}

func (m *mockServerStream) SetHeader(md metadata.MD) error {
	return nil
}

func (s *RateLimitInterceptorTestSuite) TestRateLimitInterceptor_CustomDryRunCallbacks() {
	rate := Rate{1, time.Second}

	// Custom dry run callbacks that set flags to verify they were called
	var customUnaryDryRunCalled, customStreamDryRunCalled bool

	customUnaryOnRejectInDryRun := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams) (interface{}, error) {
		customUnaryDryRunCalled = true
		return &grpc_testing.SimpleResponse{
			Payload: &grpc_testing.Payload{
				Body: []byte("custom dry run response"),
			},
		}, nil
	}

	customStreamOnRejectInDryRun := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams) error {
		customStreamDryRunCalled = true
		return handler(srv, ss)
	}

	var options []RateLimitOption
	if s.IsUnary {
		options = []RateLimitOption{
			WithRateLimitDryRun(true),
			WithRateLimitUnaryOnRejectInDryRun(customUnaryOnRejectInDryRun),
		}
	} else {
		options = []RateLimitOption{
			WithRateLimitDryRun(true),
			WithRateLimitStreamOnRejectInDryRun(customStreamOnRejectInDryRun),
		}
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, rate, options)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request should succeed
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
	}

	// Reset flags before second request
	customUnaryDryRunCalled = false
	customStreamDryRunCalled = false

	// Second request should trigger rate limit but use custom dry run callback
	if s.IsUnary {
		result, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		s.Equal([]byte("custom dry run response"), result.Payload.Body)
		s.True(customUnaryDryRunCalled, "Custom unary dry run callback should have been called")
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
		s.True(customStreamDryRunCalled, "Custom stream dry run callback should have been called")
	}
}

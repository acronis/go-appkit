/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/grpcserver/interceptor"
	"github.com/acronis/go-appkit/log/logtest"
)

// mockMetricsCollector implements MetricsCollector for testing
type mockMetricsCollector struct {
	rateLimitRejects       atomic.Int32
	lastRateLimitDryRun    atomic.Bool
	inFlightLimitRejects   atomic.Int32
	lastInFlightDryRun     atomic.Bool
	lastInFlightBacklogged atomic.Bool
}

func (m *mockMetricsCollector) IncRateLimitRejects(ruleName string, dryRun bool) {
	m.rateLimitRejects.Inc()
	m.lastRateLimitDryRun.Store(dryRun)
}

func (m *mockMetricsCollector) IncInFlightLimitRejects(ruleName string, dryRun bool, backlogged bool) {
	m.inFlightLimitRejects.Inc()
	m.lastInFlightDryRun.Store(dryRun)
	m.lastInFlightBacklogged.Store(backlogged)
}

func (m *mockMetricsCollector) Reset() {
	m.rateLimitRejects.Store(0)
	m.lastRateLimitDryRun.Store(false)
	m.inFlightLimitRejects.Store(0)
	m.lastInFlightDryRun.Store(false)
	m.lastInFlightBacklogged.Store(false)
}

// grpcTestService represents the gRPC test service interface
type grpcTestService interface {
	UnaryCall(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error)
	StreamingOutputCall(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error
	EmptyCall(ctx context.Context, req *grpc_testing.Empty) (*grpc_testing.Empty, error)
}

// testService implements grpcTestService
type testService struct {
	grpc_testing.UnimplementedTestServiceServer
	unaryCallHandler           func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error)
	streamingOutputCallHandler func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error
}

func (s *testService) UnaryCall(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
	if s.unaryCallHandler != nil {
		return s.unaryCallHandler(ctx, req)
	}
	return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
}

func (s *testService) StreamingOutputCall(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
	if s.streamingOutputCallHandler != nil {
		return s.streamingOutputCallHandler(req, stream)
	}
	return stream.Send(&grpc_testing.StreamingOutputCallResponse{
		Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
	})
}

func (s *testService) EmptyCall(ctx context.Context, req *grpc_testing.Empty) (*grpc_testing.Empty, error) {
	return &grpc_testing.Empty{}, nil
}

func startTestService(
	serverOpts []grpc.ServerOption,
	dialOpts []grpc.DialOption,
) (svc grpcTestService, client grpc_testing.TestServiceClient, closeFn func() error, err error) {
	testSvc := &testService{}
	var clientConn *grpc.ClientConn
	if _, clientConn, closeFn, err = newTestServerAndClient(serverOpts, dialOpts, func(s *grpc.Server) {
		grpc_testing.RegisterTestServiceServer(s, testSvc)
	}); err != nil {
		return nil, nil, nil, err
	}
	return testSvc, grpc_testing.NewTestServiceClient(clientConn), closeFn, nil
}

func newTestServerAndClient(
	serverOpts []grpc.ServerOption, dialOpts []grpc.DialOption, registerFn func(s *grpc.Server),
) (server *grpc.Server, clientConn *grpc.ClientConn, closeFn func() error, err error) {
	srv := grpc.NewServer(serverOpts...)
	registerFn(srv)
	ln, lnErr := net.Listen("tcp", "localhost:0")
	if lnErr != nil {
		return nil, nil, nil, fmt.Errorf("listen: %w", lnErr)
	}
	serveResult := make(chan error)
	go func() {
		serveResult <- srv.Serve(ln)
	}()
	defer func() {
		if err != nil {
			srv.Stop()
			if srvErr := <-serveResult; srvErr != nil && !isConnClosedError(srvErr) {
				err = fmt.Errorf("serve: %w; %w", srvErr, err)
			}
		}
	}()

	// Create client connection with insecure credentials
	clientConn, dialErr := grpc.NewClient(ln.Addr().String(),
		append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))...,
	)
	if dialErr != nil {
		return nil, nil, nil, fmt.Errorf("dial: %w", dialErr)
	}
	return srv, clientConn, func() error {
		mErr := clientConn.Close()
		srv.GracefulStop()
		return mErr
	}, nil
}

func isConnClosedError(err error) bool {
	return err != nil && err.Error() == "use of closed network connection"
}

// ThrottleInterceptorTestSuite is a test suite for Throttle interceptors
type ThrottleInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitBasicFunctionality() {
	mockCollector := &mockMetricsCollector{}
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg, withMetricsCollector(mockCollector))
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

	// Verify rate limit reject metrics
	s.Require().Equal(1, int(mockCollector.rateLimitRejects.Load()))
	s.Require().False(mockCollector.lastRateLimitDryRun.Load())
	s.Require().Equal(0, int(mockCollector.inFlightLimitRejects.Load()))
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_InFlightLimitBasicFunctionality() {
	mockCollector := &mockMetricsCollector{}
	cfg := &Config{
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"test_zone": {
				InFlightLimit: 1,
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				InFlightLimits: []RuleInFlightLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, cfg, withMetricsCollector(mockCollector))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	requestStarted := make(chan struct{}, 1)
	requestCanFinish := make(chan struct{}, 1)
	var requestsCounter atomic.Int32

	if s.IsUnary {
		svc.(*testService).unaryCallHandler = func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			if requestsCounter.Add(1) == 1 {
				close(requestStarted)
				<-requestCanFinish
			}
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		}
	} else {
		svc.(*testService).streamingOutputCallHandler = func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			if requestsCounter.Add(1) == 1 {
				close(requestStarted)
				<-requestCanFinish
			}
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		}
	}

	var wg sync.WaitGroup
	var okCount, rejectedCount atomic.Int32

	concurrentReqs := 5
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

	// Wait for the first request to start
	<-requestStarted

	// Give a small delay to ensure other requests are queued
	time.Sleep(100 * time.Millisecond)

	// Allow the first request to complete
	close(requestCanFinish)

	wg.Wait()

	s.Require().Equal(1, int(requestsCounter.Load()), "only one request should be processed")
	s.Require().Equal(1, int(okCount.Load()), "only one request should succeed")
	s.Require().Equal(concurrentReqs-1, int(rejectedCount.Load()), "all other requests should be rejected")

	// Verify in-flight limit reject metrics
	s.Require().Equal(0, int(mockCollector.rateLimitRejects.Load()))
	s.Require().Equal(concurrentReqs-1, int(mockCollector.inFlightLimitRejects.Load()))
	s.Require().False(mockCollector.lastInFlightDryRun.Load())
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitLeakyBucket() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				Alg:        RateLimitAlgLeakyBucket,
				RateLimit:  RateLimitValue{Count: 5, Duration: time.Second},
				BurstLimit: 5,
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

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

	// Should allow burst + 1 request (initial bucket capacity + 1 emission)
	s.Require().Equal(6, int(okCount.Load()))
	s.Require().Equal(concurrentReqs-6, int(rejectedCount.Load()))
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitSlidingWindow() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				Alg:       RateLimitAlgSlidingWindow,
				RateLimit: RateLimitValue{Count: 2, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

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
	s.Require().Equal(2, int(okCount.Load()))
	s.Require().Equal(concurrentReqs-2, int(rejectedCount.Load()))
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_WithIdentityKey() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	getKeyIdentityUnary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
				return clientIDs[0], false, nil
			}
		}
		return "", true, nil // bypass if no client-id
	}

	getKeyIdentityStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
				return clientIDs[0], false, nil
			}
		}
		return "", true, nil // bypass if no client-id
	}

	logger := logtest.NewRecorder()
	var serverOptions []grpc.ServerOption

	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryGetKeyIdentity(getKeyIdentityUnary))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamGetKeyIdentity(getKeyIdentityStream))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
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

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitDryRun() {
	mockCollector := &mockMetricsCollector{}
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key:    ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
					DryRun: true,
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg, withMetricsCollector(mockCollector))
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

	// Verify dry run rate limit metrics (first request passes, remaining 4 would be rejected in non-dry-run)
	s.Require().Equal(4, int(mockCollector.rateLimitRejects.Load()))
	s.Require().True(mockCollector.lastRateLimitDryRun.Load())
	s.Require().Equal(0, int(mockCollector.inFlightLimitRejects.Load()))
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_InFlightLimitDryRun() {
	mockCollector := &mockMetricsCollector{}
	cfg := &Config{
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"test_zone": {
				InFlightLimit: 1,
				ZoneConfig: ZoneConfig{
					Key:    ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
					DryRun: true,
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				InFlightLimits: []RuleInFlightLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, cfg, withMetricsCollector(mockCollector))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	// Set up a blocking handler to test in-flight limits in dry run
	requestStarted := make(chan struct{}, 1)
	requestCanFinish := make(chan struct{}, 1)
	var requestsCounter atomic.Int32

	if s.IsUnary {
		svc.(*testService).unaryCallHandler = func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			if requestsCounter.Add(1) == 1 {
				close(requestStarted)
				<-requestCanFinish
			}
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		}
	} else {
		svc.(*testService).streamingOutputCallHandler = func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			if requestsCounter.Add(1) == 1 {
				close(requestStarted)
				<-requestCanFinish
			}
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		}
	}

	// Start first request that will block
	firstCallResult := make(chan error, 1)
	go func() {
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			firstCallResult <- unaryErr
		} else {
			stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallResult <- streamErr
				return
			}
			_, recvErr := stream.Recv()
			firstCallResult <- recvErr
		}
	}()

	// Wait for the first request to start
	<-requestStarted

	// Make additional requests that should succeed in dry run but trigger metrics
	const additionalReqs = 3
	for i := 0; i < additionalReqs; i++ {
		if s.IsUnary {
			_, unaryErr2 := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			s.Require().NoError(unaryErr2)
		} else {
			stream2, streamErr2 := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			s.Require().NoError(streamErr2)
			_, recvErr2 := stream2.Recv()
			s.Require().NoError(recvErr2)
		}
	}

	// Allow the first request to complete
	close(requestCanFinish)

	// Wait for the first call to finish and check result
	s.Require().NoError(<-firstCallResult)

	// Verify dry run in-flight limit metrics (first request passes, remaining should trigger metrics)
	s.Require().Equal(0, int(mockCollector.rateLimitRejects.Load()))
	s.Require().Equal(additionalReqs, int(mockCollector.inFlightLimitRejects.Load()))
	s.Require().True(mockCollector.lastInFlightDryRun.Load())
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_ServiceMethodMatching() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/UnaryCall"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	if s.IsUnary {
		// First UnaryCall should succeed
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		// Second UnaryCall should be rejected
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err)
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))

		// EmptyCall should not be throttled (not in ServiceMethods)
		_, err = client.EmptyCall(reqCtx, &grpc_testing.Empty{})
		s.Require().NoError(err)
		_, err = client.EmptyCall(reqCtx, &grpc_testing.Empty{})
		s.Require().NoError(err)
	} else {
		// StreamingOutputCall should not be throttled (not in ServiceMethods)
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().NoError(recvErr2)
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_ExcludedServiceMethods() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods:         []string{"/grpc.testing.TestService/*"},
				ExcludedServiceMethods: []string{"/grpc.testing.TestService/EmptyCall"},
				RateLimits:             []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// EmptyCall should not be throttled (excluded)
	_, err = client.EmptyCall(reqCtx, &grpc_testing.Empty{})
	s.Require().NoError(err)
	_, err = client.EmptyCall(reqCtx, &grpc_testing.Empty{})
	s.Require().NoError(err)

	if s.IsUnary {
		// UnaryCall should be throttled
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err)
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		// StreamingOutputCall should be throttled
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_MultipleZones() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"zone1": {
				RateLimit: RateLimitValue{Count: 2, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
			"zone2": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"inflight_zone": {
				InFlightLimit: 3,
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "zone1"}, {Zone: "zone2"}},
				InFlightLimits: []RuleInFlightLimit{{Zone: "inflight_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request should succeed (allowed by both zones)
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		// Second request should be rejected by zone2 (limit: 1)
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err)
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_Tags() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
				Tags:           TagsList{"tag1", "tag2"},
			},
		},
	}

	logger := logtest.NewRecorder()
	var serverOptions []grpc.ServerOption

	// Use tag filtering - should not apply any rules
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryTags([]string{"tag3"}))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamTags([]string{"tag3"}))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// Both requests should succeed (no matching tags)
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().NoError(recvErr2)
	}

	// Now test with matching tag
	var serverOptions2 []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor2, err := UnaryInterceptor(cfg, WithUnaryTags([]string{"tag1"}))
		s.Require().NoError(err)
		loggingInterceptor2 := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions2 = append(serverOptions2, grpc.ChainUnaryInterceptor(loggingInterceptor2, unaryInterceptor2))
	} else {
		streamInterceptor2, err := StreamInterceptor(cfg, WithStreamTags([]string{"tag1"}))
		s.Require().NoError(err)
		loggingInterceptor2 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions2 = append(serverOptions2, grpc.ChainStreamInterceptor(loggingInterceptor2, streamInterceptor2))
	}

	_, client2, closeSvc2, err := startTestService(serverOptions2, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc2()) }()

	// First request should succeed, second should be rejected
	if s.IsUnary {
		_, unaryErr := client2.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		_, err = client2.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err)
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		stream, streamErr := client2.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)

		stream2, streamErr2 := client2.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_ZoneLevelTags() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"zone_a": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits: []RuleRateLimit{
					{Zone: "zone_a", Tags: TagsList{"tag_a"}},
				},
			},
		},
	}

	logger := logtest.NewRecorder()

	// Test 1: tag_a matches zone_a rate limit
	_, client, closeSvc, err := s.setupTestService(logger, cfg, withTags([]string{"tag_a"}))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	if s.IsUnary {
		_, err = client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().NoError(err, "first request should succeed")
		_, err = client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().Error(err, "second request should fail")
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr, "first request should succeed")

		stream2, streamErr2 := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2, "second request should fail")
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}

	// Test 2: tag_b (no match) - should not apply any limits
	_, client2, closeSvc2, err := s.setupTestService(logger, cfg, withTags([]string{"tag_b"}))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc2()) }()

	if s.IsUnary {
		for i := 0; i < 5; i++ {
			_, unaryErr := client2.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			s.Require().NoError(unaryErr, "request %d should succeed", i+1)
		}
	} else {
		for i := 0; i < 5; i++ {
			stream, streamErr := client2.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			s.Require().NoError(streamErr, "stream %d should be created", i+1)
			_, recvErr := stream.Recv()
			s.Require().NoError(recvErr, "stream %d should receive", i+1)
		}
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RuleLevelTagsPrecedence() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				Tags:           TagsList{"tag_rule"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone", Tags: TagsList{"tag_zone"}}},
			},
		},
	}

	logger := logtest.NewRecorder()

	// Test with tag_rule - should apply zone even though zone has tag_zone
	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryTags([]string{"tag_rule"}))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamTags([]string{"tag_rule"}))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// First request succeeds, second fails (rate limiting 1/s)
	if s.IsUnary {
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err, "first request should succeed")
		_, err = client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err, "second request should fail")
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr, "first request should succeed")

		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2, "second request should fail")
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}

	// Test with tag_zone - should also apply zone (zone tag matches)
	var serverOptions2 []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor2, err := UnaryInterceptor(cfg, WithUnaryTags([]string{"tag_zone"}))
		s.Require().NoError(err)
		loggingInterceptor2 := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions2 = append(serverOptions2, grpc.ChainUnaryInterceptor(loggingInterceptor2, unaryInterceptor2))
	} else {
		streamInterceptor2, err := StreamInterceptor(cfg, WithStreamTags([]string{"tag_zone"}))
		s.Require().NoError(err)
		loggingInterceptor2 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions2 = append(serverOptions2, grpc.ChainStreamInterceptor(loggingInterceptor2, streamInterceptor2))
	}

	_, client2, closeSvc2, err := startTestService(serverOptions2, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc2()) }()

	// First request succeeds, second fails (rate limiting 1/s)
	if s.IsUnary {
		_, err = client2.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err, "first request should succeed")
		_, err = client2.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().Error(err, "second request should fail")
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
	} else {
		stream, streamErr := client2.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr, "first request should succeed")

		stream2, streamErr2 := client2.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2, "second request should fail")
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}

	// Test with tag_none - should not apply any zones
	var serverOptions3 []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor3, err := UnaryInterceptor(cfg, WithUnaryTags([]string{"tag_none"}))
		s.Require().NoError(err)
		loggingInterceptor3 := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions3 = append(serverOptions3, grpc.ChainUnaryInterceptor(loggingInterceptor3, unaryInterceptor3))
	} else {
		streamInterceptor3, err := StreamInterceptor(cfg, WithStreamTags([]string{"tag_none"}))
		s.Require().NoError(err)
		loggingInterceptor3 := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions3 = append(serverOptions3, grpc.ChainStreamInterceptor(loggingInterceptor3, streamInterceptor3))
	}

	_, client3, closeSvc3, err := startTestService(serverOptions3, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc3()) }()

	// All requests should succeed (no matching tags)
	if s.IsUnary {
		for i := 0; i < 5; i++ {
			_, unaryErr := client3.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
			s.Require().NoError(unaryErr, "request %d should succeed", i+1)
		}
	} else {
		for i := 0; i < 5; i++ {
			stream, streamErr := client3.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			s.Require().NoError(streamErr, "stream %d should be created", i+1)
			_, recvErr := stream.Recv()
			s.Require().NoError(recvErr, "stream %d should receive", i+1)
		}
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RetryAfterHeader() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit:          RateLimitValue{Count: 1, Duration: time.Second},
				ResponseRetryAfter: RateLimitRetryAfterValue{Duration: 5 * time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
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
		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{}, grpc.Header(&headers))
		s.Require().NoError(streamErr2)
		_, recvErr2 := stream2.Recv()
		s.Require().Error(recvErr2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr2))
	}

	// Check retry-after header
	retryAfterVals := headers.Get("retry-after")
	s.Require().Len(retryAfterVals, 1)
	s.Require().Equal("5", retryAfterVals[0])
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitOnReject() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	var onRejectCalled atomic.Bool

	// Custom reject handler that returns a custom response
	onReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.RateLimitParams) (interface{}, error) {
		onRejectCalled.Store(true)
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("custom-reject-response")}}, nil
	}

	onRejectStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.RateLimitParams) error {
		onRejectCalled.Store(true)
		return ss.SendMsg(&grpc_testing.StreamingOutputCallResponse{
			Payload: &grpc_testing.Payload{Body: []byte("custom-reject-response")},
		})
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryRateLimitOnReject(onReject))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamRateLimitOnReject(onRejectStream))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	if s.IsUnary {
		// First request should succeed
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		s.Require().False(onRejectCalled.Load())

		// Second request should trigger custom reject handler
		resp, err := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
		s.Require().True(onRejectCalled.Load())
		s.Require().Equal("custom-reject-response", string(resp.Payload.Body))
	} else {
		// First request should succeed
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
		s.Require().False(onRejectCalled.Load())

		// Second request should trigger custom reject handler
		stream2, streamErr2 := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		resp, recvErr2 := stream2.Recv()
		s.Require().NoError(recvErr2)
		s.Require().True(onRejectCalled.Load())
		s.Require().Equal("custom-reject-response", string(resp.Payload.Body))
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitOnRejectInDryRun() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key:    ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
					DryRun: true,
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	var rejectionCount atomic.Int32

	onRejectDryRun := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.RateLimitParams) (interface{}, error) {
		rejectionCount.Inc()
		return handler(ctx, req)
	}

	onRejectStreamDryRun := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.RateLimitParams) error {
		rejectionCount.Inc()
		return handler(srv, ss)
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryRateLimitOnRejectInDryRun(onRejectDryRun))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamRateLimitOnRejectInDryRun(onRejectStreamDryRun))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	// All requests should succeed in dry run mode, but callback should be called
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

	s.Require().Equal(4, int(rejectionCount.Load())) // First request allowed, next 4 would be rejected
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitOnError() {
	// Create a config that will trigger rate limiting errors by using a very small max keys
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 10, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	var onErrorCalled atomic.Bool

	onError := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.RateLimitParams, err error) (interface{}, error) {
		onErrorCalled.Store(true)
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("error-handled")}}, nil
	}
	onErrorStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.RateLimitParams, err error) error {
		onErrorCalled.Store(true)
		return ss.SendMsg(&grpc_testing.StreamingOutputCallResponse{
			Payload: &grpc_testing.Payload{Body: []byte("error-handled")},
		})
	}

	getKeyIdentityUnary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
		return "", false, fmt.Errorf("internal error")
	}

	getKeyIdentityStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
		return "", false, fmt.Errorf("internal error")
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryRateLimitOnError(onError), WithUnaryGetKeyIdentity(getKeyIdentityUnary))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamRateLimitOnError(onErrorStream), WithStreamGetKeyIdentity(getKeyIdentityStream))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	if s.IsUnary {
		resp, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		s.Require().True(onErrorCalled.Load())
		s.Require().Equal("error-handled", string(resp.Payload.Body))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		resp, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
		s.Require().True(onErrorCalled.Load())
		s.Require().Equal("error-handled", string(resp.Payload.Body))
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_InFlightLimitOnReject() {
	cfg := &Config{
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"test_zone": {
				InFlightLimit: 1,
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				InFlightLimits: []RuleInFlightLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	svc := &testService{}

	requestStarted := make(chan struct{}, 1)
	requestCanFinish := make(chan struct{}, 1)

	// Set up a handler that blocks to create in-flight requests
	if s.IsUnary {
		svc.unaryCallHandler = func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			close(requestStarted)
			<-requestCanFinish
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		}
	} else {
		svc.streamingOutputCallHandler = func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			close(requestStarted)
			<-requestCanFinish
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		}
	}

	var onRejectCalled atomic.Bool

	// Custom reject handler that returns a custom response
	onReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.InFlightLimitParams) (interface{}, error) {
		onRejectCalled.Store(true)
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("custom-inflight-reject")}}, nil
	}
	onRejectStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.InFlightLimitParams) error {
		onRejectCalled.Store(true)
		return ss.SendMsg(&grpc_testing.StreamingOutputCallResponse{
			Payload: &grpc_testing.Payload{Body: []byte("custom-inflight-reject")},
		})
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryInFlightLimitOnReject(onReject))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamInFlightLimitOnReject(onRejectStream))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, clientConn, closeFn, err := newTestServerAndClient(serverOptions, nil, func(s *grpc.Server) {
		grpc_testing.RegisterTestServiceServer(s, svc)
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeFn()) }()

	client := grpc_testing.NewTestServiceClient(clientConn)

	firstCallResult := make(chan error, 1)

	// Start first request that will block
	go func() {
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			firstCallResult <- unaryErr
		} else {
			stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallResult <- streamErr
				return
			}
			_, recvErr := stream.Recv()
			firstCallResult <- recvErr
		}
	}()

	// Wait for the first request to start
	<-requestStarted

	// Second request should trigger custom reject handler
	if s.IsUnary {
		resp, unaryErr2 := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr2)
		s.Require().True(onRejectCalled.Load())
		s.Require().Equal("custom-inflight-reject", string(resp.Payload.Body))
	} else {
		stream2, streamErr2 := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr2)
		resp, recvErr2 := stream2.Recv()
		s.Require().NoError(recvErr2)
		s.Require().True(onRejectCalled.Load())
		s.Require().Equal("custom-inflight-reject", string(resp.Payload.Body))
	}

	// Allow the first request to complete
	close(requestCanFinish)

	// Wait for the first call to finish and check result
	s.Require().NoError(<-firstCallResult)
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_InFlightLimitOnRejectInDryRun() {
	cfg := &Config{
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"test_zone": {
				InFlightLimit: 1,
				ZoneConfig: ZoneConfig{
					Key:    ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
					DryRun: true,
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				InFlightLimits: []RuleInFlightLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	svc := &testService{}

	requestStarted := make(chan struct{}, 1)
	requestCanFinish := make(chan struct{}, 1)

	// Set up a handler that blocks to create in-flight requests
	if s.IsUnary {
		svc.unaryCallHandler = func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			close(requestStarted)
			<-requestCanFinish
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("test")}}, nil
		}
	} else {
		svc.streamingOutputCallHandler = func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			close(requestStarted)
			<-requestCanFinish
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		}
	}

	var rejectionCount atomic.Int32
	onRejectDryRun := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.InFlightLimitParams) (interface{}, error) {
		rejectionCount.Inc()
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("dry-run-reject")}}, nil
	}
	onRejectStreamDryRun := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.InFlightLimitParams) error {
		rejectionCount.Inc()
		return ss.SendMsg(&grpc_testing.StreamingOutputCallResponse{
			Payload: &grpc_testing.Payload{Body: []byte("dry-run-reject-stream")},
		})
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryInFlightLimitOnRejectInDryRun(onRejectDryRun))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamInFlightLimitOnRejectInDryRun(onRejectStreamDryRun))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, clientConn, closeFn, err := newTestServerAndClient(serverOptions, nil, func(s *grpc.Server) {
		grpc_testing.RegisterTestServiceServer(s, svc)
	})
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeFn()) }()

	client := grpc_testing.NewTestServiceClient(clientConn)

	firstCallResult := make(chan error, 1)

	// Start first request that will block
	go func() {
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			firstCallResult <- unaryErr
		} else {
			stream, streamErr := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallResult <- streamErr
				return
			}
			_, recvErr := stream.Recv()
			firstCallResult <- recvErr
		}
	}()

	// Wait for the first request to start
	<-requestStarted

	// Multiple additional requests should trigger the dry run callback
	for i := 0; i < 3; i++ {
		if s.IsUnary {
			resp, unaryErr2 := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
			s.Require().NoError(unaryErr2)
			s.Require().Equal("dry-run-reject", string(resp.Payload.Body))
		} else {
			stream2, streamErr2 := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
			s.Require().NoError(streamErr2)
			resp, recvErr2 := stream2.Recv()
			s.Require().NoError(recvErr2)
			s.Require().Equal("dry-run-reject-stream", string(resp.Payload.Body))
		}
	}

	s.Require().Equal(3, int(rejectionCount.Load()))

	// Allow the first request to complete
	close(requestCanFinish)

	// Wait for the first call to finish and check result
	s.Require().NoError(<-firstCallResult)
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_InFlightLimitOnError() {
	// Create a config that will trigger in-flight limiting errors by using a very small max keys
	cfg := &Config{
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"test_zone": {
				InFlightLimit: 10,
				ZoneConfig: ZoneConfig{
					Key:     ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
					MaxKeys: 1, // Very small to trigger key eviction and potential errors
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				InFlightLimits: []RuleInFlightLimit{{Zone: "test_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	var onErrorCalled atomic.Bool

	// Custom error handler
	onError := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.InFlightLimitParams, err error) (interface{}, error) {
		onErrorCalled.Store(true)
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("inflight-error-handled")}}, nil
	}

	onErrorStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.InFlightLimitParams, err error) error {
		onErrorCalled.Store(true)
		return ss.SendMsg(&grpc_testing.StreamingOutputCallResponse{
			Payload: &grpc_testing.Payload{Body: []byte("inflight-error-handled")},
		})
	}

	getKeyIdentityUnary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
		return "", false, fmt.Errorf("internal error")
	}

	getKeyIdentityStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
		return "", false, fmt.Errorf("internal error")
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg, WithUnaryInFlightLimitOnError(onError), WithUnaryGetKeyIdentity(getKeyIdentityUnary))
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg, WithStreamInFlightLimitOnError(onErrorStream), WithStreamGetKeyIdentity(getKeyIdentityStream))
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	if s.IsUnary {
		resp, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
		s.Require().True(onErrorCalled.Load())
		s.Require().Equal("inflight-error-handled", string(resp.Payload.Body))
	} else {
		stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		resp, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
		s.Require().True(onErrorCalled.Load())
		s.Require().Equal("inflight-error-handled", string(resp.Payload.Body))
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_VariadicOptions() {
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"test_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "test_zone"}},
			},
		},
	}

	getKeyIdentityUnary := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
				return clientIDs[0], false, nil
			}
		}
		return "", true, nil // bypass if no client-id
	}

	getKeyIdentityStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
		if md, ok := metadata.FromIncomingContext(ss.Context()); ok {
			if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
				return clientIDs[0], false, nil
			}
		}
		return "", true, nil // bypass if no client-id
	}

	logger := logtest.NewRecorder()
	var serverOptions []grpc.ServerOption

	if s.IsUnary {
		// Test variadic options for unary interceptor
		unaryInterceptor, err := UnaryInterceptor(cfg,
			WithUnaryGetKeyIdentity(getKeyIdentityUnary),
		)
		s.Require().NoError(err)

		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		// Test variadic options for stream interceptor
		streamInterceptor, err := StreamInterceptor(cfg,
			WithStreamGetKeyIdentity(getKeyIdentityStream),
		)
		s.Require().NoError(err)

		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
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
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_CallbackPriorityAndCombination() {
	// Test that demonstrates how multiple callbacks work together
	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"rate_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
			"rate_zone_dry": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key:    ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
					DryRun: true,
				},
			},
		},
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"inflight_zone": {
				InFlightLimit: 2,
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/UnaryCall"},
				RateLimits:     []RuleRateLimit{{Zone: "rate_zone"}},
				InFlightLimits: []RuleInFlightLimit{{Zone: "inflight_zone"}},
			},
			{
				ServiceMethods: []string{"/grpc.testing.TestService/StreamingOutputCall"},
				RateLimits:     []RuleRateLimit{{Zone: "rate_zone_dry"}},
				InFlightLimits: []RuleInFlightLimit{{Zone: "inflight_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	var rateLimitRejectCalled, inFlightRejectCalled, dryRunRejectCalled atomic.Bool

	// Rate limit callbacks
	rateLimitOnReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.RateLimitParams) (interface{}, error) {
		rateLimitRejectCalled.Store(true)
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("rate-limit-reject")}}, nil
	}

	rateLimitOnRejectDryRun := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.RateLimitParams) (interface{}, error) {
		dryRunRejectCalled.Store(true)
		return handler(ctx, req) // Continue with normal processing in dry run
	}

	// In-flight limit callbacks
	inFlightOnReject := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.InFlightLimitParams) (interface{}, error) {
		inFlightRejectCalled.Store(true)
		return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("inflight-reject")}}, nil
	}

	// Stream equivalents
	rateLimitOnRejectStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.RateLimitParams) error {
		dryRunRejectCalled.Store(true)
		return handler(srv, ss) // Continue with normal processing in dry run
	}

	inFlightOnRejectStream := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params interceptor.InFlightLimitParams) error {
		inFlightRejectCalled.Store(true)
		return ss.SendMsg(&grpc_testing.StreamingOutputCallResponse{
			Payload: &grpc_testing.Payload{Body: []byte("inflight-reject-stream")},
		})
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		unaryInterceptor, err := UnaryInterceptor(cfg,
			WithUnaryRateLimitOnReject(rateLimitOnReject),
			WithUnaryRateLimitOnRejectInDryRun(rateLimitOnRejectDryRun),
			WithUnaryInFlightLimitOnReject(inFlightOnReject),
		)
		s.Require().NoError(err)
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		streamInterceptor, err := StreamInterceptor(cfg,
			WithStreamRateLimitOnRejectInDryRun(rateLimitOnRejectStream),
			WithStreamInFlightLimitOnReject(inFlightOnRejectStream),
		)
		s.Require().NoError(err)
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}

	_, client, closeSvc, err := startTestService(serverOptions, nil)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	reqCtx := context.Background()

	if s.IsUnary {
		// First request should succeed
		_, unaryErr := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)

		// Second request should trigger rate limit reject (not dry run for UnaryCall)
		resp, err := client.UnaryCall(reqCtx, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
		s.Require().True(rateLimitRejectCalled.Load())
		s.Require().Equal("rate-limit-reject", string(resp.Payload.Body))
	} else {
		// For streaming, the dry run rate limit should be triggered
		for i := 0; i < 3; i++ {
			stream, streamErr := client.StreamingOutputCall(reqCtx, &grpc_testing.StreamingOutputCallRequest{})
			s.Require().NoError(streamErr)
			_, recvErr := stream.Recv()
			s.Require().NoError(recvErr)
		}
		s.Require().True(dryRunRejectCalled.Load())
	}
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_InFlightLimitByHeader() {
	const headerName = "x-client-id"
	const clientA = "client-a"
	const clientB = "client-b"

	cfg := &Config{
		InFlightLimitZones: map[string]InFlightLimitZoneConfig{
			"header_zone": {
				InFlightLimit: 1,
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeHeader, HeaderName: headerName},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				InFlightLimits: []RuleInFlightLimit{{Zone: "header_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	svc, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	var requestStartedOnce sync.Once
	requestStarted := make(chan struct{}, 1)
	requestCanFinish := make(chan struct{}, 1)

	if s.IsUnary {
		svc.(*testService).unaryCallHandler = func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			if md, ok := metadata.FromIncomingContext(ctx); ok {
				if clientID := md.Get(headerName); len(clientID) > 0 && clientID[0] == clientA {
					// Simulate a long-running request for client A
					requestStartedOnce.Do(func() { close(requestStarted) })
					<-requestCanFinish
					return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("ok")}}, nil
				}
			}
			return &grpc_testing.SimpleResponse{Payload: &grpc_testing.Payload{Body: []byte("ok")}}, nil
		}
	} else {
		svc.(*testService).streamingOutputCallHandler = func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
				if clientID := md.Get(headerName); len(clientID) > 0 && clientID[0] == clientA {
					// Simulate a long-running stream for client A
					requestStartedOnce.Do(func() { close(requestStarted) })
					<-requestCanFinish
				}
			}
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{Payload: &grpc_testing.Payload{Body: []byte("ok-stream")}})
		}
	}

	ctxKeyA := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerName, clientA))
	ctxKeyB := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerName, clientB))

	// Launch first request with key A to occupy the in-flight slot
	firstCallResult := make(chan error, 1)
	go func() {
		if s.IsUnary {
			_, unaryErr := client.UnaryCall(ctxKeyA, &grpc_testing.SimpleRequest{})
			firstCallResult <- unaryErr
		} else {
			resp, streamErr := client.StreamingOutputCall(ctxKeyA, &grpc_testing.StreamingOutputCallRequest{})
			if streamErr != nil {
				firstCallResult <- streamErr
				return
			}
			_, recvErr := resp.Recv()
			firstCallResult <- recvErr
		}
	}()

	<-requestStarted // Wait for the first request to start

	// Second concurrent request with the SAME header value should be rejected
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(ctxKeyA, &grpc_testing.SimpleRequest{})
		s.Require().Error(unaryErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(unaryErr))
	} else {
		resp, stream := client.StreamingOutputCall(ctxKeyA, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(stream)
		_, recvErr := resp.Recv()
		s.Require().Error(recvErr)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recvErr))
	}

	// Concurrent request with DIFFERENT header value should be allowed (independent key)
	if s.IsUnary {
		_, unaryErr := client.UnaryCall(ctxKeyB, &grpc_testing.SimpleRequest{})
		s.Require().NoError(unaryErr)
	} else {
		resp, streamErr := client.StreamingOutputCall(ctxKeyB, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(streamErr)
		_, recvErr := resp.Recv()
		s.Require().NoError(recvErr)
	}

	// Allow the first request to complete
	close(requestCanFinish)

	// Wait for the first call to finish and check result
	s.Require().NoError(<-firstCallResult)
}

func (s *ThrottleInterceptorTestSuite) TestThrottleInterceptor_RateLimitByHeader() {
	const headerName = "x-client-id"
	const clientA = "client-a"
	const clientB = "client-b"

	cfg := &Config{
		RateLimitZones: map[string]RateLimitZoneConfig{
			"header_zone": {
				RateLimit: RateLimitValue{Count: 1, Duration: time.Second},
				ZoneConfig: ZoneConfig{
					Key: ZoneKeyConfig{Type: ZoneKeyTypeHeader, HeaderName: headerName},
				},
			},
		},
		Rules: []RuleConfig{
			{
				ServiceMethods: []string{"/grpc.testing.TestService/*"},
				RateLimits:     []RuleRateLimit{{Zone: "header_zone"}},
			},
		},
	}

	logger := logtest.NewRecorder()
	_, client, closeSvc, err := s.setupTestService(logger, cfg)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	ctxKeyA := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerName, clientA))
	ctxKeyB := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(headerName, clientB))

	// First request with key A should pass
	if s.IsUnary {
		_, err = client.UnaryCall(ctxKeyA, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
		// Second with same key should be rate-limited
		_, err = client.UnaryCall(ctxKeyA, &grpc_testing.SimpleRequest{})
		s.Require().Error(err)
		s.Require().Equal(codes.ResourceExhausted, status.Code(err))
		// Using different key should succeed
		_, err = client.UnaryCall(ctxKeyB, &grpc_testing.SimpleRequest{})
		s.Require().NoError(err)
	} else {
		st1, err1 := client.StreamingOutputCall(ctxKeyA, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(err1)
		_, recv1 := st1.Recv()
		s.Require().NoError(recv1)
		// Second stream with same key should be rate-limited
		st2, err2 := client.StreamingOutputCall(ctxKeyA, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(err2)
		_, recv2 := st2.Recv()
		s.Require().Error(recv2)
		s.Require().Equal(codes.ResourceExhausted, status.Code(recv2))
		// Different key should succeed
		st3, err3 := client.StreamingOutputCall(ctxKeyB, &grpc_testing.StreamingOutputCallRequest{})
		s.Require().NoError(err3)
		_, recv3 := st3.Recv()
		s.Require().NoError(recv3)
	}
}

type testServiceOption func(o *testServiceOptions)

func withTags(tags []string) testServiceOption {
	return func(o *testServiceOptions) {
		o.tags = tags
	}
}

func withMetricsCollector(mc MetricsCollector) testServiceOption {
	return func(o *testServiceOptions) {
		o.mc = mc
	}
}

type testServiceOptions struct {
	mc   MetricsCollector
	tags []string
}

func (s *ThrottleInterceptorTestSuite) setupTestService(
	logger *logtest.Recorder, cfg *Config, options ...testServiceOption,
) (grpcTestService, grpc_testing.TestServiceClient, func() error, error) {
	var opts testServiceOptions
	for _, o := range options {
		o(&opts)
	}

	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		var interceptorOpts []UnaryInterceptorOption
		if opts.mc != nil {
			interceptorOpts = append(interceptorOpts, WithUnaryMetricsCollector(opts.mc))
		}
		if len(opts.tags) != 0 {
			interceptorOpts = append(interceptorOpts, WithUnaryTags(opts.tags))
		}
		unaryInterceptor, err := UnaryInterceptor(cfg, interceptorOpts...)
		if err != nil {
			return nil, nil, nil, err
		}
		loggingInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
			return handler(interceptor.NewContextWithLogger(ctx, logger), req)
		}
		serverOptions = append(serverOptions, grpc.ChainUnaryInterceptor(loggingInterceptor, unaryInterceptor))
	} else {
		var interceptorOpts []StreamInterceptorOption
		if opts.mc != nil {
			interceptorOpts = append(interceptorOpts, WithStreamMetricsCollector(opts.mc))
		}
		if len(opts.tags) != 0 {
			interceptorOpts = append(interceptorOpts, WithStreamTags(opts.tags))
		}
		streamInterceptor, err := StreamInterceptor(cfg, interceptorOpts...)
		if err != nil {
			return nil, nil, nil, err
		}
		loggingInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: interceptor.NewContextWithLogger(ss.Context(), logger)}
			return handler(srv, wrappedStream)
		}
		serverOptions = append(serverOptions, grpc.ChainStreamInterceptor(loggingInterceptor, streamInterceptor))
	}
	return startTestService(serverOptions, nil)
}

/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/testutil"
)

// MetricsInterceptorTestSuite is a test suite for Metrics interceptors
type MetricsInterceptorTestSuite struct {
	suite.Suite
	IsUnary bool
}

func TestMetricsServerUnaryInterceptor(t *testing.T) {
	suite.Run(t, &MetricsInterceptorTestSuite{IsUnary: true})
}

func TestMetricsServerStreamInterceptor(t *testing.T) {
	suite.Run(t, &MetricsInterceptorTestSuite{IsUnary: false})
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorHistogram() {
	const okCalls = 10
	const permissionDeniedCalls = 5

	promMetrics := NewPrometheusMetrics()

	svc, client, closeSvc, err := s.createTestService(promMetrics)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	getHist := func(code codes.Code) prometheus.Histogram {
		methodName, methodType := s.getMethodNameAndType()
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.PermissionDenied), 0)

	for i := 0; i < okCalls; i++ {
		err = s.makeCall(client)
		s.Require().NoError(err)
	}
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), okCalls)
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.PermissionDenied), 0)

	permissionDeniedErr := status.Error(codes.PermissionDenied, "Permission denied")
	s.switchHandler(svc, permissionDeniedErr)
	for i := 0; i < permissionDeniedCalls; i++ {
		err = s.makeCall(client)
		s.Require().ErrorIs(err, permissionDeniedErr)
	}
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), okCalls)
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.PermissionDenied), permissionDeniedCalls)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorInFlight() {
	promMetrics := NewPrometheusMetrics()

	svc, client, closeSvc, err := s.createTestService(promMetrics)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	methodName, methodType := s.getMethodNameAndType()
	gauge := promMetrics.InFlight.WithLabelValues(
		"grpc.testing.TestService", methodName, string(methodType), "")

	requireSamplesCountInGauge(s.T(), gauge, 0)

	called, done := make(chan struct{}), make(chan struct{})
	s.switchHandlerWithChannels(svc, called, done)

	callErr := make(chan error)
	go func() {
		callErr <- s.makeCall(client)
	}()

	<-called
	requireSamplesCountInGauge(s.T(), gauge, 1)
	close(done)
	s.Require().NoError(<-callErr)
	requireSamplesCountInGauge(s.T(), gauge, 0)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorExcludedMethods() {
	promMetrics := NewPrometheusMetrics()

	methodName, methodType := s.getMethodNameAndType()
	fullMethodName := "/grpc.testing.TestService/" + methodName

	_, client, closeSvc, err := s.createTestServiceWithOptions(promMetrics, WithMetricsExcludedMethods(fullMethodName))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	err = s.makeCall(client)
	s.Require().NoError(err)

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorMultipleExcludedMethods() {
	promMetrics := NewPrometheusMetrics()

	methodName, methodType := s.getMethodNameAndType()
	fullMethodName := "/grpc.testing.TestService/" + methodName

	_, client, closeSvc, err := s.createTestServiceWithOptions(promMetrics, WithMetricsExcludedMethods(fullMethodName, "/grpc.testing.TestService/OtherCall"))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	err = s.makeCall(client)
	s.Require().NoError(err)

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorVariadicOptions() {
	promMetrics := NewPrometheusMetrics()

	_, client, closeSvc, err := s.createTestServiceWithOptions(promMetrics,
		WithMetricsExcludedMethods("/grpc.testing.TestService/ExcludedMethod"),
		WithMetricsExcludedMethods("/grpc.testing.TestService/AnotherExcludedMethod"))
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	methodName, methodType := s.getMethodNameAndType()
	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)

	err = s.makeCall(client)
	s.Require().NoError(err)

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 1)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorWithUserAgentTypeProvider() {
	promMetrics := NewPrometheusMetrics()

	userAgentType := "test-client"
	var opts []MetricsOption
	if s.IsUnary {
		opts = []MetricsOption{WithMetricsUnaryUserAgentTypeProvider(func(ctx context.Context, info *grpc.UnaryServerInfo) string {
			return userAgentType
		})}
	} else {
		opts = []MetricsOption{WithMetricsStreamUserAgentTypeProvider(func(ctx context.Context, info *grpc.StreamServerInfo) string {
			return userAgentType
		})}
	}

	_, client, closeSvc, err := s.createTestServiceWithOptions(promMetrics, opts...)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	methodName, methodType := s.getMethodNameAndType()
	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), userAgentType, code.String()).(prometheus.Histogram)
	}

	gauge := promMetrics.InFlight.WithLabelValues(
		"grpc.testing.TestService", methodName, string(methodType), userAgentType)

	// Check initial state
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)
	requireSamplesCountInGauge(s.T(), gauge, 0)

	err = s.makeCall(client)
	s.Require().NoError(err)

	// Check after call
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 1)
	requireSamplesCountInGauge(s.T(), gauge, 0)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorWithEmptyUserAgentTypeProvider() {
	promMetrics := NewPrometheusMetrics()

	var opts []MetricsOption
	if s.IsUnary {
		opts = []MetricsOption{WithMetricsUnaryUserAgentTypeProvider(func(ctx context.Context, info *grpc.UnaryServerInfo) string {
			return ""
		})}
	} else {
		opts = []MetricsOption{WithMetricsStreamUserAgentTypeProvider(func(ctx context.Context, info *grpc.StreamServerInfo) string {
			return ""
		})}
	}

	_, client, closeSvc, err := s.createTestServiceWithOptions(promMetrics, opts...)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	methodName, methodType := s.getMethodNameAndType()
	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)

	err = s.makeCall(client)
	s.Require().NoError(err)

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 1)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorWithoutUserAgentTypeProvider() {
	promMetrics := NewPrometheusMetrics()

	_, client, closeSvc, err := s.createTestServiceWithOptions(promMetrics)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	methodName, methodType := s.getMethodNameAndType()
	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 0)

	err = s.makeCall(client)
	s.Require().NoError(err)

	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.OK), 1)
}

func (s *MetricsInterceptorTestSuite) TestMetricsServerInterceptorPanicHandling() {
	promMetrics := NewPrometheusMetrics()

	svc, client, closeSvc, err := s.createTestServiceWithRecovery(promMetrics)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(closeSvc()) }()

	methodName, methodType := s.getMethodNameAndType()
	getHist := func(code codes.Code) prometheus.Histogram {
		return promMetrics.Durations.WithLabelValues(
			"grpc.testing.TestService", methodName, string(methodType), "", code.String()).(prometheus.Histogram)
	}

	gauge := promMetrics.InFlight.WithLabelValues(
		"grpc.testing.TestService", methodName, string(methodType), "")

	// Verify initial state
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.Internal), 0)
	requireSamplesCountInGauge(s.T(), gauge, 0)

	// Set up handler that panics
	panicMessage := "test panic"
	s.switchHandlerThatPanics(svc, panicMessage)

	// Make call that will panic - recovery interceptor will convert panic to Internal error
	err = s.makeCall(client)
	s.Require().Error(err)
	s.Require().ErrorIs(err, InternalError)

	// Verify metrics were recorded with Internal status
	testutil.RequireSamplesCountInHistogram(s.T(), getHist(codes.Internal), 1)
	// Verify in-flight counter was decremented
	requireSamplesCountInGauge(s.T(), gauge, 0)
}

func (s *MetricsInterceptorTestSuite) createTestService(promMetrics *PrometheusMetrics) (*testService, grpc_testing.TestServiceClient, func() error, error) {
	return s.createTestServiceWithOptions(promMetrics)
}

func (s *MetricsInterceptorTestSuite) createTestServiceWithOptions(promMetrics *PrometheusMetrics, options ...MetricsOption) (*testService, grpc_testing.TestServiceClient, func() error, error) {
	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		serverOptions = []grpc.ServerOption{grpc.UnaryInterceptor(MetricsUnaryInterceptor(promMetrics, options...))}
	} else {
		serverOptions = []grpc.ServerOption{grpc.StreamInterceptor(MetricsStreamInterceptor(promMetrics, options...))}
	}
	return startTestService(serverOptions, nil)
}

func (s *MetricsInterceptorTestSuite) createTestServiceWithRecovery(promMetrics *PrometheusMetrics, options ...MetricsOption) (*testService, grpc_testing.TestServiceClient, func() error, error) {
	var serverOptions []grpc.ServerOption
	if s.IsUnary {
		serverOptions = []grpc.ServerOption{grpc.ChainUnaryInterceptor(
			MetricsUnaryInterceptor(promMetrics, options...),
			RecoveryUnaryInterceptor(),
		)}
	} else {
		serverOptions = []grpc.ServerOption{grpc.ChainStreamInterceptor(
			MetricsStreamInterceptor(promMetrics, options...),
			RecoveryStreamInterceptor(),
		)}
	}
	return startTestService(serverOptions, nil)
}

func (s *MetricsInterceptorTestSuite) getMethodNameAndType() (string, CallMethodType) {
	if s.IsUnary {
		return "UnaryCall", CallMethodTypeUnary
	}
	return "StreamingOutputCall", CallMethodTypeStream
}

func (s *MetricsInterceptorTestSuite) makeCall(client grpc_testing.TestServiceClient) error {
	if s.IsUnary {
		_, err := client.UnaryCall(context.Background(), &grpc_testing.SimpleRequest{})
		return err
	}

	stream, err := client.StreamingOutputCall(context.Background(), &grpc_testing.StreamingOutputCallRequest{})
	if err != nil {
		return err
	}
	_, err = stream.Recv()
	return err
}

func (s *MetricsInterceptorTestSuite) switchHandler(svc *testService, returnErr error) {
	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			return nil, returnErr
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			return returnErr
		})
	}
}

func (s *MetricsInterceptorTestSuite) switchHandlerWithChannels(svc *testService, called, done chan struct{}) {
	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			close(called)
			<-done
			return &grpc_testing.SimpleResponse{}, nil
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			close(called)
			<-done
			return stream.Send(&grpc_testing.StreamingOutputCallResponse{
				Payload: &grpc_testing.Payload{Body: []byte("test-stream")},
			})
		})
	}
}

func (s *MetricsInterceptorTestSuite) switchHandlerThatPanics(svc *testService, panicMessage string) {
	if s.IsUnary {
		svc.SwitchUnaryCallHandler(func(ctx context.Context, req *grpc_testing.SimpleRequest) (*grpc_testing.SimpleResponse, error) {
			panic(panicMessage)
		})
	} else {
		svc.SwitchStreamingOutputCallHandler(func(req *grpc_testing.StreamingOutputCallRequest, stream grpc_testing.TestService_StreamingOutputCallServer) error {
			panic(panicMessage)
		})
	}
}

func TestNewPrometheusMetrics(t *testing.T) {
	t.Run("test default constructor", func(t *testing.T) {
		promMetrics := NewPrometheusMetrics()
		require.NotNil(t, promMetrics)
		require.NotNil(t, promMetrics.Durations)
		require.NotNil(t, promMetrics.InFlight)
	})

	t.Run("test with namespace option", func(t *testing.T) {
		promMetrics := NewPrometheusMetrics(
			WithPrometheusNamespace("test_namespace"),
		)
		require.NotNil(t, promMetrics)
		require.NotNil(t, promMetrics.Durations)
		require.NotNil(t, promMetrics.InFlight)
	})

	t.Run("test with custom duration buckets", func(t *testing.T) {
		customBuckets := []float64{0.1, 0.5, 1.0, 5.0}
		promMetrics := NewPrometheusMetrics(
			WithPrometheusDurationBuckets(customBuckets),
		)
		require.NotNil(t, promMetrics)
		require.NotNil(t, promMetrics.Durations)
		require.NotNil(t, promMetrics.InFlight)
	})

	t.Run("test with const labels", func(t *testing.T) {
		constLabels := prometheus.Labels{"service": "test", "version": "1.0"}
		promMetrics := NewPrometheusMetrics(
			WithPrometheusConstLabels(constLabels),
		)
		require.NotNil(t, promMetrics)
		require.NotNil(t, promMetrics.Durations)
		require.NotNil(t, promMetrics.InFlight)
	})

	t.Run("test with curried label names", func(t *testing.T) {
		curriedLabels := []string{"instance", "region"}
		promMetrics := NewPrometheusMetrics(
			WithPrometheusCurriedLabelNames(curriedLabels),
		)
		require.NotNil(t, promMetrics)
		require.NotNil(t, promMetrics.Durations)
		require.NotNil(t, promMetrics.InFlight)
	})

	t.Run("test with multiple options", func(t *testing.T) {
		customBuckets := []float64{0.1, 0.5, 1.0}
		constLabels := prometheus.Labels{"service": "test"}
		promMetrics := NewPrometheusMetrics(
			WithPrometheusNamespace("multi_test"),
			WithPrometheusDurationBuckets(customBuckets),
			WithPrometheusConstLabels(constLabels),
		)
		require.NotNil(t, promMetrics)
		require.NotNil(t, promMetrics.Durations)
		require.NotNil(t, promMetrics.InFlight)
	})
}

type tHelper interface {
	Helper()
}

func assertSamplesCountInGauge(t assert.TestingT, gauge prometheus.Gauge, wantCount int) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	reg := prometheus.NewPedanticRegistry()
	if !assert.NoError(t, reg.Register(gauge)) {
		return false
	}
	gotMetrics, err := reg.Gather()
	if !assert.NoError(t, err) {
		return false
	}
	if !assert.Equal(t, 1, len(gotMetrics)) {
		return false
	}
	return assert.Equal(t, wantCount, int(gotMetrics[0].GetMetric()[0].GetGauge().GetValue()))
}

func requireSamplesCountInGauge(t require.TestingT, gauge prometheus.Gauge, wantCount int) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if assertSamplesCountInGauge(t, gauge, wantCount) {
		return
	}
	t.FailNow()
}

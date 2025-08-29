/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/internal/libinfo"
)

const (
	callMetricsLabelService       = "grpc_service"
	callMetricsLabelMethod        = "grpc_method"
	callMetricsLabelMethodType    = "grpc_method_type"
	callMetricsLabelUserAgentType = "user_agent_type"
	callMetricsLabelCode          = "grpc_code"
)

// CallInfoMetrics represents a call info for collecting metrics.
type CallInfoMetrics struct {
	Service       string
	Method        string
	UserAgentType string
}

// CallMethodType represents the type of gRPC method call.
type CallMethodType string

const (
	// CallMethodTypeUnary represents a unary gRPC method call.
	CallMethodTypeUnary CallMethodType = "unary"
	// CallMethodTypeStream represents a streaming gRPC method call.
	CallMethodTypeStream CallMethodType = "stream"
)

// UnaryUserAgentTypeProvider returns a user agent type or empty string based on the gRPC context and method info.
type UnaryUserAgentTypeProvider func(ctx context.Context, info *grpc.UnaryServerInfo) string

// StreamUserAgentTypeProvider returns a user agent type or empty string based on the gRPC context and stream method info.
type StreamUserAgentTypeProvider func(ctx context.Context, info *grpc.StreamServerInfo) string

// MetricsCollector is an interface for collecting metrics for incoming gRPC calls.
type MetricsCollector interface {
	// IncInFlightCalls increments the counter of in-flight calls.
	IncInFlightCalls(callInfo CallInfoMetrics, methodType CallMethodType)

	// DecInFlightCalls decrements the counter of in-flight calls.
	DecInFlightCalls(callInfo CallInfoMetrics, methodType CallMethodType)

	// ObserveCallFinish observes the duration of the call and the status code.
	ObserveCallFinish(callInfo CallInfoMetrics, methodType CallMethodType, code codes.Code, startTime time.Time)
}

// DefaultPrometheusDurationBuckets is default buckets into which observations of serving gRPC calls are counted.
var DefaultPrometheusDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 150, 300, 600}

// PrometheusOption is a function type for configuring the metrics collector.
type PrometheusOption func(*prometheusOptions)

// prometheusOptions hold options for configuring Prometheus metrics collector.
type prometheusOptions struct {
	namespace         string
	durationBuckets   []float64
	constLabels       prometheus.Labels
	curriedLabelNames []string
}

// WithPrometheusNamespace sets the namespace for metrics.
func WithPrometheusNamespace(namespace string) PrometheusOption {
	return func(c *prometheusOptions) {
		c.namespace = namespace
	}
}

// WithPrometheusDurationBuckets sets the duration buckets for histogram metrics.
func WithPrometheusDurationBuckets(buckets []float64) PrometheusOption {
	return func(c *prometheusOptions) {
		c.durationBuckets = buckets
	}
}

// WithPrometheusConstLabels sets constant labels that will be applied to all metrics.
func WithPrometheusConstLabels(labels prometheus.Labels) PrometheusOption {
	return func(c *prometheusOptions) {
		c.constLabels = labels
	}
}

// WithPrometheusCurriedLabelNames sets label names that will be curried.
func WithPrometheusCurriedLabelNames(labelNames []string) PrometheusOption {
	return func(c *prometheusOptions) {
		c.curriedLabelNames = labelNames
	}
}

// PrometheusMetrics represents collector of metrics for incoming gRPC calls.
type PrometheusMetrics struct {
	Durations *prometheus.HistogramVec
	InFlight  *prometheus.GaugeVec
}

// NewPrometheusMetrics creates a new instance of PrometheusMetrics with the provided options.
func NewPrometheusMetrics(opts ...PrometheusOption) *PrometheusMetrics {
	config := &prometheusOptions{
		durationBuckets: DefaultPrometheusDurationBuckets,
	}
	for _, opt := range opts {
		opt(config)
	}

	makeLabelNames := func(names ...string) []string {
		l := append(make([]string, 0, len(config.curriedLabelNames)+len(names)), config.curriedLabelNames...)
		return append(l, names...)
	}

	durations := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   config.namespace,
			Name:        "grpc_call_duration_seconds",
			Help:        "A histogram of the gRPC call durations.",
			Buckets:     config.durationBuckets,
			ConstLabels: libinfo.AddPrometheusLibVersionLabel(config.constLabels),
		},
		makeLabelNames(
			callMetricsLabelService,
			callMetricsLabelMethod,
			callMetricsLabelMethodType,
			callMetricsLabelUserAgentType,
			callMetricsLabelCode,
		),
	)

	inFlight := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   config.namespace,
			Name:        "grpc_calls_in_flight",
			Help:        "Current number of gRPC calls being served.",
			ConstLabels: libinfo.AddPrometheusLibVersionLabel(config.constLabels),
		},
		makeLabelNames(
			callMetricsLabelService,
			callMetricsLabelMethod,
			callMetricsLabelMethodType,
			callMetricsLabelUserAgentType,
		),
	)

	return &PrometheusMetrics{
		Durations: durations,
		InFlight:  inFlight,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (pm *PrometheusMetrics) MustCurryWith(labels prometheus.Labels) *PrometheusMetrics {
	return &PrometheusMetrics{
		Durations: pm.Durations.MustCurryWith(labels).(*prometheus.HistogramVec),
		InFlight:  pm.InFlight.MustCurryWith(labels),
	}
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (pm *PrometheusMetrics) MustRegister() {
	prometheus.MustRegister(
		pm.Durations,
		pm.InFlight,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (pm *PrometheusMetrics) Unregister() {
	prometheus.Unregister(pm.InFlight)
	prometheus.Unregister(pm.Durations)
}

// IncInFlightCalls increments the counter of in-flight calls.
func (pm *PrometheusMetrics) IncInFlightCalls(callInfo CallInfoMetrics, methodType CallMethodType) {
	pm.InFlight.With(prometheus.Labels{
		callMetricsLabelService:       callInfo.Service,
		callMetricsLabelMethod:        callInfo.Method,
		callMetricsLabelMethodType:    string(methodType),
		callMetricsLabelUserAgentType: callInfo.UserAgentType,
	}).Inc()
}

// DecInFlightCalls decrements the counter of in-flight calls.
func (pm *PrometheusMetrics) DecInFlightCalls(callInfo CallInfoMetrics, methodType CallMethodType) {
	pm.InFlight.With(prometheus.Labels{
		callMetricsLabelService:       callInfo.Service,
		callMetricsLabelMethod:        callInfo.Method,
		callMetricsLabelMethodType:    string(methodType),
		callMetricsLabelUserAgentType: callInfo.UserAgentType,
	}).Dec()
}

// ObserveCallFinish observes the duration of the call and the status code.
func (pm *PrometheusMetrics) ObserveCallFinish(
	callInfo CallInfoMetrics, methodType CallMethodType, code codes.Code, startTime time.Time,
) {
	pm.Durations.With(prometheus.Labels{
		callMetricsLabelService:       callInfo.Service,
		callMetricsLabelMethod:        callInfo.Method,
		callMetricsLabelCode:          code.String(),
		callMetricsLabelMethodType:    string(methodType),
		callMetricsLabelUserAgentType: callInfo.UserAgentType,
	}).Observe(time.Since(startTime).Seconds())
}

// MetricsOption is a function type for configuring the metrics interceptor.
type MetricsOption func(*metricsOptions)

// metricsOptions holds options for configuring the metrics interceptor.
type metricsOptions struct {
	excludedMethods             []string
	unaryUserAgentTypeProvider  UnaryUserAgentTypeProvider
	streamUserAgentTypeProvider StreamUserAgentTypeProvider
}

// WithMetricsExcludedMethods returns an option that excludes the specified methods from metrics collection.
func WithMetricsExcludedMethods(methods ...string) MetricsOption {
	return func(c *metricsOptions) {
		c.excludedMethods = append(c.excludedMethods, methods...)
	}
}

// WithMetricsUnaryUserAgentTypeProvider sets a user agent type provider for unary interceptors.
func WithMetricsUnaryUserAgentTypeProvider(provider UnaryUserAgentTypeProvider) MetricsOption {
	return func(c *metricsOptions) {
		c.unaryUserAgentTypeProvider = provider
	}
}

// WithMetricsStreamUserAgentTypeProvider sets a user agent type provider for stream interceptors.
func WithMetricsStreamUserAgentTypeProvider(provider StreamUserAgentTypeProvider) MetricsOption {
	return func(c *metricsOptions) {
		c.streamUserAgentTypeProvider = provider
	}
}

// isMethodExcluded checks if the given method is in the excluded methods list.
func isMethodExcluded(fullMethod string, excludedMethods []string) bool {
	for _, excludedMethod := range excludedMethods {
		if fullMethod == excludedMethod {
			return true
		}
	}
	return false
}

// prepareCallMetrics prepares the call metrics information and manages start time.
func prepareCallMetrics(ctx context.Context, fullMethod string, userAgentType string) (
	newCtx context.Context, callInfo CallInfoMetrics, startTime time.Time, startTimeGenerated bool,
) {
	startTime = GetCallStartTimeFromContext(ctx)
	if startTime.IsZero() {
		startTime = time.Now()
		ctx = NewContextWithCallStartTime(ctx, startTime)
		startTimeGenerated = true
	}

	service, method := splitFullMethodName(fullMethod)
	callInfo = CallInfoMetrics{
		Service:       service,
		Method:        method,
		UserAgentType: userAgentType,
	}

	return ctx, callInfo, startTime, startTimeGenerated
}

// MetricsUnaryInterceptor is an interceptor that collects metrics for incoming gRPC calls.
func MetricsUnaryInterceptor(
	collector MetricsCollector,
	opts ...MetricsOption,
) grpc.UnaryServerInterceptor {
	config := &metricsOptions{}
	for _, opt := range opts {
		opt(config)
	}
	return func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (interface{}, error) {
		if isMethodExcluded(info.FullMethod, config.excludedMethods) {
			return handler(ctx, req)
		}

		userAgentType := ""
		if config.unaryUserAgentTypeProvider != nil {
			userAgentType = config.unaryUserAgentTypeProvider(ctx, info)
		}

		ctx, callInfo, startTime, _ := prepareCallMetrics(ctx, info.FullMethod, userAgentType)

		collector.IncInFlightCalls(callInfo, CallMethodTypeUnary)
		defer collector.DecInFlightCalls(callInfo, CallMethodTypeUnary)

		var resp interface{}
		var err error
		defer func() {
			if p := recover(); p != nil {
				collector.ObserveCallFinish(callInfo, CallMethodTypeUnary, codes.Internal, startTime)
				panic(p)
			}
			collector.ObserveCallFinish(callInfo, CallMethodTypeUnary, getCodeFromError(err), startTime)
		}()

		resp, err = handler(ctx, req)
		return resp, err
	}
}

// MetricsStreamInterceptor is an interceptor that collects metrics for incoming gRPC stream calls.
func MetricsStreamInterceptor(
	collector MetricsCollector,
	opts ...MetricsOption,
) grpc.StreamServerInterceptor {
	config := &metricsOptions{}
	for _, opt := range opts {
		opt(config)
	}
	return func(
		srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
	) error {
		if isMethodExcluded(info.FullMethod, config.excludedMethods) {
			return handler(srv, ss)
		}

		userAgentType := ""
		if config.streamUserAgentTypeProvider != nil {
			userAgentType = config.streamUserAgentTypeProvider(ss.Context(), info)
		}

		ctx, callInfo, startTime, startTimeGenerated := prepareCallMetrics(ss.Context(), info.FullMethod, userAgentType)

		collector.IncInFlightCalls(callInfo, CallMethodTypeStream)
		defer collector.DecInFlightCalls(callInfo, CallMethodTypeStream)

		nextSrvStream := ss
		if startTimeGenerated {
			nextSrvStream = &WrappedServerStream{
				ServerStream: ss,
				Ctx:          ctx,
			}
		}

		var err error
		defer func() {
			if p := recover(); p != nil {
				collector.ObserveCallFinish(callInfo, CallMethodTypeStream, codes.Internal, startTime)
				panic(p)
			}
			collector.ObserveCallFinish(callInfo, CallMethodTypeStream, getCodeFromError(err), startTime)
		}()

		err = handler(srv, nextSrvStream)
		return err
	}
}

func getCodeFromError(err error) codes.Code {
	s, ok := status.FromError(err)
	if !ok {
		s = status.FromContextError(err)
	}
	return s.Code()
}

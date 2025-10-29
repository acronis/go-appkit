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
	CustomValues  map[string]string
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
	customLabelNames  []string
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

// WithPrometheusCustomLabelNames sets custom label names that will be added to the metrics.
// Values for these labels provided via CallInfoMetrics.CustomValues when recording metrics.
// Values in CallInfoMetrics.CustomValues can be set dynamically during request processing using MetricsParams.SetValue() method.
// MetricsParams can be obtained from the context via GetMetricsParamsFromContext().
// If a custom value is not set for a label name provided here, it will be recorded as an empty string in the metric.
// If a custom value is set with a name not listed here, it will be ignored.
func WithPrometheusCustomLabelNames(labelNames []string) PrometheusOption {
	return func(c *prometheusOptions) {
		c.customLabelNames = labelNames
	}
}

// PrometheusMetrics represents collector of metrics for incoming gRPC calls.
type PrometheusMetrics struct {
	Durations        *prometheus.HistogramVec
	InFlight         *prometheus.GaugeVec
	customLabelNames []string
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
		l := make([]string, 0, len(config.curriedLabelNames)+len(names)+len(config.customLabelNames))
		l = append(l, config.curriedLabelNames...)
		l = append(l, names...)
		l = append(l, config.customLabelNames...)
		return l
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
		Durations:        durations,
		InFlight:         inFlight,
		customLabelNames: config.customLabelNames,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (pm *PrometheusMetrics) MustCurryWith(labels prometheus.Labels) *PrometheusMetrics {
	return &PrometheusMetrics{
		Durations:        pm.Durations.MustCurryWith(labels).(*prometheus.HistogramVec),
		InFlight:         pm.InFlight.MustCurryWith(labels),
		customLabelNames: pm.customLabelNames,
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

// makeLabels creates prometheus.Labels from base labels and custom labels.
// It ensures all custom label names from pm.customLabelNames are present (with empty values if not provided).
func (pm *PrometheusMetrics) makeLabels(baseLabels prometheus.Labels, customLabels map[string]string) prometheus.Labels {
	labels := make(prometheus.Labels, len(baseLabels)+len(pm.customLabelNames))
	for k, v := range baseLabels {
		labels[k] = v
	}
	for _, labelName := range pm.customLabelNames {
		if v, ok := customLabels[labelName]; ok {
			labels[labelName] = v
		} else {
			labels[labelName] = ""
		}
	}
	return labels
}

// IncInFlightCalls increments the counter of in-flight calls.
func (pm *PrometheusMetrics) IncInFlightCalls(callInfo CallInfoMetrics, methodType CallMethodType) {
	baseLabels := prometheus.Labels{
		callMetricsLabelService:       callInfo.Service,
		callMetricsLabelMethod:        callInfo.Method,
		callMetricsLabelMethodType:    string(methodType),
		callMetricsLabelUserAgentType: callInfo.UserAgentType,
	}
	pm.InFlight.With(pm.makeLabels(baseLabels, callInfo.CustomValues)).Inc()
}

// DecInFlightCalls decrements the counter of in-flight calls.
func (pm *PrometheusMetrics) DecInFlightCalls(callInfo CallInfoMetrics, methodType CallMethodType) {
	baseLabels := prometheus.Labels{
		callMetricsLabelService:       callInfo.Service,
		callMetricsLabelMethod:        callInfo.Method,
		callMetricsLabelMethodType:    string(methodType),
		callMetricsLabelUserAgentType: callInfo.UserAgentType,
	}
	pm.InFlight.With(pm.makeLabels(baseLabels, callInfo.CustomValues)).Dec()
}

// ObserveCallFinish observes the duration of the call and the status code.
func (pm *PrometheusMetrics) ObserveCallFinish(
	callInfo CallInfoMetrics, methodType CallMethodType, code codes.Code, startTime time.Time,
) {
	baseLabels := prometheus.Labels{
		callMetricsLabelService:       callInfo.Service,
		callMetricsLabelMethod:        callInfo.Method,
		callMetricsLabelCode:          code.String(),
		callMetricsLabelMethodType:    string(methodType),
		callMetricsLabelUserAgentType: callInfo.UserAgentType,
	}
	pm.Durations.With(pm.makeLabels(baseLabels, callInfo.CustomValues)).Observe(time.Since(startTime).Seconds())
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

// copyValues creates a copy of the map.
func copyValues(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// prepareCallMetrics prepares the call metrics information and manages start time.
func prepareCallMetrics(ctx context.Context, fullMethod string, userAgentType string) (
	newCtx context.Context, callInfo CallInfoMetrics, startTime time.Time, startTimeGenerated bool, mp *MetricsParams,
) {
	startTime = GetCallStartTimeFromContext(ctx)
	if startTime.IsZero() {
		startTime = time.Now()
		ctx = NewContextWithCallStartTime(ctx, startTime)
		startTimeGenerated = true
	}

	mp = GetMetricsParamsFromContext(ctx)
	if mp == nil {
		mp = &MetricsParams{}
		ctx = NewContextWithMetricsParams(ctx, mp)
	}

	service, method := splitFullMethodName(fullMethod)
	callInfo = CallInfoMetrics{
		Service:       service,
		Method:        method,
		UserAgentType: userAgentType,
		CustomValues:  copyValues(mp.values), // we copy values here to avoid mutation during the InFlight metrics processing
	}

	return ctx, callInfo, startTime, startTimeGenerated, mp
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

		ctx, callInfo, startTime, _, mp := prepareCallMetrics(ctx, info.FullMethod, userAgentType)

		collector.IncInFlightCalls(callInfo, CallMethodTypeUnary)
		defer collector.DecInFlightCalls(callInfo, CallMethodTypeUnary)

		var resp interface{}
		var err error
		defer func() {
			// re-extract labels from MetricsParams
			callInfo.CustomValues = mp.values

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

		ctx, callInfo, startTime, startTimeGenerated, mp := prepareCallMetrics(ss.Context(), info.FullMethod, userAgentType)

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
			// re-extract labels from MetricsParams
			callInfo.CustomValues = mp.values

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

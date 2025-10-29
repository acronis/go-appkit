/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/acronis/go-appkit/internal/libinfo"
)

const (
	httpRequestMetricsLabelMethod        = "method"
	httpRequestMetricsLabelRoutePattern  = "route_pattern"
	httpRequestMetricsLabelUserAgentType = "user_agent_type"
	httpRequestMetricsLabelStatusCode    = "status_code"
)

const (
	userAgentTypeBrowser    = "browser"
	userAgentTypeHTTPClient = "http-client"
)

// HTTPRequestInfoMetrics represents a request info for collecting metrics.
type HTTPRequestInfoMetrics struct {
	Method        string
	RoutePattern  string
	UserAgentType string
	CustomValues  map[string]string
}

// HTTPRequestMetricsCollector is an interface for collecting metrics for incoming HTTP requests.
type HTTPRequestMetricsCollector interface {
	// IncInFlightRequests increments the counter of in-flight requests.
	IncInFlightRequests(requestInfo HTTPRequestInfoMetrics)

	// DecInFlightRequests decrements the counter of in-flight requests.
	DecInFlightRequests(requestInfo HTTPRequestInfoMetrics)

	// ObserveRequestFinish observes the duration of the request and the status code.
	ObserveRequestFinish(requestInfo HTTPRequestInfoMetrics, status int, startTime time.Time)
}

// DefaultHTTPRequestDurationBuckets is default buckets into which observations of serving HTTP requests are counted.
var DefaultHTTPRequestDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 150, 300, 600}

// HTTPRequestPrometheusMetricsOpts represents an options for HTTPRequestPrometheusMetrics.
type HTTPRequestPrometheusMetricsOpts struct {
	// Namespace is a namespace for metrics. It will be prepended to all metric names.
	Namespace string

	// DurationBuckets is a list of buckets into which observations of serving HTTP requests are counted.
	DurationBuckets []float64

	// ConstLabels is a set of labels that will be applied to all metrics.
	ConstLabels prometheus.Labels

	// CurriedLabelNames is a list of label names that will be curried with the provided labels.
	// See HTTPRequestPrometheusMetrics.MustCurryWith method for more details.
	// Keep in mind that if this list is not empty,
	// HTTPRequestPrometheusMetrics.MustCurryWith method must be called further with the same labels.
	// Otherwise, the collector will panic.
	CurriedLabelNames []string

	// CustomLabelNames is a list of custom label names that will be added to the metrics.
	// Values for these labels provided via HTTPRequestInfoMetrics.CustomValues when recording metrics.
	// Values in HTTPRequestInfoMetrics.CustomValues can be set dynamically during request processing using MetricsParams.SetValue() method.
	// MetricsParams can be obtained from the context via GetMetricsParamsFromContext().
	// If a custom value is not set for a label name provided here, it will be recorded as an empty string in the metric.
	// If a custom value is set with a name not listed here, it will be ignored.
	CustomLabelNames []string
}

// HTTPRequestPrometheusMetrics represents collector of metrics for incoming HTTP requests.
type HTTPRequestPrometheusMetrics struct {
	Durations        *prometheus.HistogramVec
	InFlight         *prometheus.GaugeVec
	customLabelNames []string
}

// NewHTTPRequestPrometheusMetrics creates a new instance of HTTPRequestPrometheusMetrics with default options.
func NewHTTPRequestPrometheusMetrics() *HTTPRequestPrometheusMetrics {
	return NewHTTPRequestPrometheusMetricsWithOpts(HTTPRequestPrometheusMetricsOpts{})
}

// NewHTTPRequestPrometheusMetricsWithOpts creates a new instance of HTTPRequestPrometheusMetrics with the provided options.
func NewHTTPRequestPrometheusMetricsWithOpts(opts HTTPRequestPrometheusMetricsOpts) *HTTPRequestPrometheusMetrics {
	makeLabelNames := func(names ...string) []string {
		l := make([]string, 0, len(opts.CurriedLabelNames)+len(names)+len(opts.CustomLabelNames))
		l = append(l, opts.CurriedLabelNames...)
		l = append(l, names...)
		l = append(l, opts.CustomLabelNames...)
		return l
	}

	durBuckets := opts.DurationBuckets
	if durBuckets == nil {
		durBuckets = DefaultHTTPRequestDurationBuckets
	}
	durations := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace:   opts.Namespace,
			Name:        "http_request_duration_seconds",
			Help:        "A histogram of the HTTP request durations.",
			Buckets:     durBuckets,
			ConstLabels: libinfo.AddPrometheusLibVersionLabel(opts.ConstLabels),
		},
		makeLabelNames(
			httpRequestMetricsLabelMethod,
			httpRequestMetricsLabelRoutePattern,
			httpRequestMetricsLabelUserAgentType,
			httpRequestMetricsLabelStatusCode,
		),
	)

	inFlight := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   opts.Namespace,
			Name:        "http_requests_in_flight",
			Help:        "Current number of HTTP requests being served.",
			ConstLabels: libinfo.AddPrometheusLibVersionLabel(opts.ConstLabels),
		},
		makeLabelNames(
			httpRequestMetricsLabelMethod,
			httpRequestMetricsLabelRoutePattern,
			httpRequestMetricsLabelUserAgentType,
		),
	)

	return &HTTPRequestPrometheusMetrics{
		Durations:        durations,
		InFlight:         inFlight,
		customLabelNames: opts.CustomLabelNames,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (pm *HTTPRequestPrometheusMetrics) MustCurryWith(labels prometheus.Labels) *HTTPRequestPrometheusMetrics {
	return &HTTPRequestPrometheusMetrics{
		Durations:        pm.Durations.MustCurryWith(labels).(*prometheus.HistogramVec),
		InFlight:         pm.InFlight.MustCurryWith(labels),
		customLabelNames: pm.customLabelNames,
	}
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (pm *HTTPRequestPrometheusMetrics) MustRegister() {
	prometheus.MustRegister(
		pm.Durations,
		pm.InFlight,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (pm *HTTPRequestPrometheusMetrics) Unregister() {
	prometheus.Unregister(pm.InFlight)
	prometheus.Unregister(pm.Durations)
}

// makeLabels creates prometheus.Labels from base labels and custom labels.
// It ensures all custom label names from pm.customLabelNames are present (with empty values if not provided).
func (pm *HTTPRequestPrometheusMetrics) makeLabels(baseLabels prometheus.Labels, customLabels map[string]string) prometheus.Labels {
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

// IncInFlightRequests increments the counter of in-flight requests.
func (pm *HTTPRequestPrometheusMetrics) IncInFlightRequests(requestInfo HTTPRequestInfoMetrics) {
	baseLabels := prometheus.Labels{
		httpRequestMetricsLabelMethod:        requestInfo.Method,
		httpRequestMetricsLabelRoutePattern:  requestInfo.RoutePattern,
		httpRequestMetricsLabelUserAgentType: requestInfo.UserAgentType,
	}
	pm.InFlight.With(pm.makeLabels(baseLabels, requestInfo.CustomValues)).Inc()
}

// DecInFlightRequests decrements the counter of in-flight requests.
func (pm *HTTPRequestPrometheusMetrics) DecInFlightRequests(requestInfo HTTPRequestInfoMetrics) {
	baseLabels := prometheus.Labels{
		httpRequestMetricsLabelMethod:        requestInfo.Method,
		httpRequestMetricsLabelRoutePattern:  requestInfo.RoutePattern,
		httpRequestMetricsLabelUserAgentType: requestInfo.UserAgentType,
	}
	pm.InFlight.With(pm.makeLabels(baseLabels, requestInfo.CustomValues)).Dec()
}

// ObserveRequestFinish observes the duration of the request and the status code.
func (pm *HTTPRequestPrometheusMetrics) ObserveRequestFinish(
	requestInfo HTTPRequestInfoMetrics, status int, startTime time.Time,
) {
	baseLabels := prometheus.Labels{
		httpRequestMetricsLabelMethod:        requestInfo.Method,
		httpRequestMetricsLabelRoutePattern:  requestInfo.RoutePattern,
		httpRequestMetricsLabelUserAgentType: requestInfo.UserAgentType,
		httpRequestMetricsLabelStatusCode:    strconv.Itoa(status),
	}
	pm.Durations.With(pm.makeLabels(baseLabels, requestInfo.CustomValues)).Observe(time.Since(startTime).Seconds())
}

// UserAgentTypeGetterFunc is a function for getting user agent type from the request.
// The set of return values must be finite.
type UserAgentTypeGetterFunc func(r *http.Request) string

// HTTPRequestMetricsOpts represents an options for HTTPRequestMetrics middleware.
type HTTPRequestMetricsOpts struct {
	GetUserAgentType  UserAgentTypeGetterFunc
	ExcludedEndpoints []string
}

type httpRequestMetricsHandler struct {
	next            http.Handler
	collector       HTTPRequestMetricsCollector
	getRoutePattern RoutePatternGetterFunc
	opts            HTTPRequestMetricsOpts
}

// HTTPRequestMetrics is a middleware that collects metrics for incoming HTTP requests using Prometheus data types.
func HTTPRequestMetrics(
	collector HTTPRequestMetricsCollector, getRoutePattern RoutePatternGetterFunc,
) func(next http.Handler) http.Handler {
	return HTTPRequestMetricsWithOpts(collector, getRoutePattern, HTTPRequestMetricsOpts{})
}

// HTTPRequestMetricsWithOpts is a more configurable version of HTTPRequestMetrics middleware.
func HTTPRequestMetricsWithOpts(
	collector HTTPRequestMetricsCollector,
	getRoutePattern RoutePatternGetterFunc,
	opts HTTPRequestMetricsOpts,
) func(next http.Handler) http.Handler {
	if getRoutePattern == nil {
		panic("function for getting route pattern cannot be nil")
	}
	if opts.GetUserAgentType == nil {
		opts.GetUserAgentType = determineUserAgentType
	}
	return func(next http.Handler) http.Handler {
		return &httpRequestMetricsHandler{next: next, collector: collector, getRoutePattern: getRoutePattern, opts: opts}
	}
}

func (h *httpRequestMetricsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	for i := range h.opts.ExcludedEndpoints {
		if r.URL.Path == h.opts.ExcludedEndpoints[i] {
			h.next.ServeHTTP(rw, r)
			return
		}
	}

	startTime := GetRequestStartTimeFromContext(r.Context())
	if startTime.IsZero() {
		startTime = time.Now()
		r = r.WithContext(NewContextWithRequestStartTime(r.Context(), startTime))
	}

	mp := GetMetricsParamsFromContext(r.Context())
	if mp == nil {
		mp = &MetricsParams{}
		r = r.WithContext(NewContextWithMetricsParams(r.Context(), mp))
	}

	reqInfo := HTTPRequestInfoMetrics{
		Method:        r.Method,
		RoutePattern:  h.getRoutePattern(r),
		UserAgentType: h.opts.GetUserAgentType(r),
		CustomValues:  copyValues(mp.values), // we copy values here to avoid mutation during the InFlight metrics processing
	}

	h.collector.IncInFlightRequests(reqInfo)
	defer h.collector.DecInFlightRequests(reqInfo)

	r = r.WithContext(NewContextWithHTTPMetricsEnabled(r.Context()))

	wrw := WrapResponseWriterIfNeeded(rw, r.ProtoMajor)
	defer func() {
		if !IsHTTPMetricsEnabledInContext(r.Context()) {
			return
		}

		if reqInfo.RoutePattern == "" {
			reqInfo.RoutePattern = h.getRoutePattern(r)
		}

		// re-extract labels from MetricsParams
		reqInfo.CustomValues = mp.values

		if p := recover(); p != nil {
			if p != http.ErrAbortHandler {
				h.collector.ObserveRequestFinish(reqInfo, http.StatusInternalServerError, startTime)
			}
			panic(p)
		}
		h.collector.ObserveRequestFinish(reqInfo, wrw.Status(), startTime)
	}()

	h.next.ServeHTTP(wrw, r)
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

func determineUserAgentType(r *http.Request) string {
	if strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		return userAgentTypeBrowser
	}
	return userAgentTypeHTTPClient
}

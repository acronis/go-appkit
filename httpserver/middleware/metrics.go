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
}

// HTTPRequestPrometheusMetrics represents collector of metrics for incoming HTTP requests.
type HTTPRequestPrometheusMetrics struct {
	Durations *prometheus.HistogramVec
	InFlight  *prometheus.GaugeVec
}

// NewHTTPRequestPrometheusMetrics creates a new instance of HTTPRequestPrometheusMetrics with default options.
func NewHTTPRequestPrometheusMetrics() *HTTPRequestPrometheusMetrics {
	return NewHTTPRequestPrometheusMetricsWithOpts(HTTPRequestPrometheusMetricsOpts{})
}

// NewHTTPRequestPrometheusMetricsWithOpts creates a new instance of HTTPRequestPrometheusMetrics with the provided options.
func NewHTTPRequestPrometheusMetricsWithOpts(opts HTTPRequestPrometheusMetricsOpts) *HTTPRequestPrometheusMetrics {
	makeLabelNames := func(names ...string) []string {
		l := append(make([]string, 0, len(opts.CurriedLabelNames)+len(names)), opts.CurriedLabelNames...)
		return append(l, names...)
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
		Durations: durations,
		InFlight:  inFlight,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (pm *HTTPRequestPrometheusMetrics) MustCurryWith(labels prometheus.Labels) *HTTPRequestPrometheusMetrics {
	return &HTTPRequestPrometheusMetrics{
		Durations: pm.Durations.MustCurryWith(labels).(*prometheus.HistogramVec),
		InFlight:  pm.InFlight.MustCurryWith(labels),
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

// IncInFlightRequests increments the counter of in-flight requests.
func (pm *HTTPRequestPrometheusMetrics) IncInFlightRequests(requestInfo HTTPRequestInfoMetrics) {
	pm.InFlight.With(prometheus.Labels{
		httpRequestMetricsLabelMethod:        requestInfo.Method,
		httpRequestMetricsLabelRoutePattern:  requestInfo.RoutePattern,
		httpRequestMetricsLabelUserAgentType: requestInfo.UserAgentType,
	}).Inc()
}

// DecInFlightRequests decrements the counter of in-flight requests.
func (pm *HTTPRequestPrometheusMetrics) DecInFlightRequests(requestInfo HTTPRequestInfoMetrics) {
	pm.InFlight.With(prometheus.Labels{
		httpRequestMetricsLabelMethod:        requestInfo.Method,
		httpRequestMetricsLabelRoutePattern:  requestInfo.RoutePattern,
		httpRequestMetricsLabelUserAgentType: requestInfo.UserAgentType,
	}).Dec()
}

// ObserveRequestFinish observes the duration of the request and the status code.
func (pm *HTTPRequestPrometheusMetrics) ObserveRequestFinish(
	requestInfo HTTPRequestInfoMetrics, status int, startTime time.Time,
) {
	pm.Durations.With(prometheus.Labels{
		httpRequestMetricsLabelMethod:        requestInfo.Method,
		httpRequestMetricsLabelRoutePattern:  requestInfo.RoutePattern,
		httpRequestMetricsLabelUserAgentType: requestInfo.UserAgentType,
		httpRequestMetricsLabelStatusCode:    strconv.Itoa(status),
	}).Observe(time.Since(startTime).Seconds())
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

	reqInfo := HTTPRequestInfoMetrics{
		Method:        r.Method,
		RoutePattern:  h.getRoutePattern(r),
		UserAgentType: h.opts.GetUserAgentType(r),
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

func determineUserAgentType(r *http.Request) string {
	if strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		return userAgentTypeBrowser
	}
	return userAgentTypeHTTPClient
}

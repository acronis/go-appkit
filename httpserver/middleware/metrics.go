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

// DefaultHTTPRequestDurationBuckets is default buckets into which observations of serving HTTP requests are counted.
var DefaultHTTPRequestDurationBuckets = []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 150, 300, 600}

// HTTPRequestMetricsCollectorOpts represents an options for HTTPRequestMetricsCollector.
type HTTPRequestMetricsCollectorOpts struct {
	// Namespace is a namespace for metrics. It will be prepended to all metric names.
	Namespace string

	// DurationBuckets is a list of buckets into which observations of serving HTTP requests are counted.
	DurationBuckets []float64

	// ConstLabels is a set of labels that will be applied to all metrics.
	ConstLabels prometheus.Labels

	// CurriedLabelNames is a list of label names that will be curried with the provided labels.
	// See HTTPRequestMetricsCollector.MustCurryWith method for more details.
	// Keep in mind that if this list is not empty,
	// HTTPRequestMetricsCollector.MustCurryWith method must be called further with the same labels.
	// Otherwise, the collector will panic.
	CurriedLabelNames []string
}

// HTTPRequestMetricsCollector represents collector of metrics for incoming HTTP requests.
type HTTPRequestMetricsCollector struct {
	Durations *prometheus.HistogramVec
	InFlight  *prometheus.GaugeVec
}

// NewHTTPRequestMetricsCollector creates a new metrics collector.
func NewHTTPRequestMetricsCollector() *HTTPRequestMetricsCollector {
	return NewHTTPRequestMetricsCollectorWithOpts(HTTPRequestMetricsCollectorOpts{})
}

// NewHTTPRequestMetricsCollectorWithOpts is a more configurable version of creating HTTPRequestMetricsCollector.
func NewHTTPRequestMetricsCollectorWithOpts(opts HTTPRequestMetricsCollectorOpts) *HTTPRequestMetricsCollector {
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
			ConstLabels: opts.ConstLabels,
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
			ConstLabels: opts.ConstLabels,
		},
		makeLabelNames(
			httpRequestMetricsLabelMethod,
			httpRequestMetricsLabelRoutePattern,
			httpRequestMetricsLabelUserAgentType,
		),
	)

	return &HTTPRequestMetricsCollector{
		Durations: durations,
		InFlight:  inFlight,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (c *HTTPRequestMetricsCollector) MustCurryWith(labels prometheus.Labels) *HTTPRequestMetricsCollector {
	return &HTTPRequestMetricsCollector{
		Durations: c.Durations.MustCurryWith(labels).(*prometheus.HistogramVec),
		InFlight:  c.InFlight.MustCurryWith(labels),
	}
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (c *HTTPRequestMetricsCollector) MustRegister() {
	prometheus.MustRegister(
		c.Durations,
		c.InFlight,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (c *HTTPRequestMetricsCollector) Unregister() {
	prometheus.Unregister(c.InFlight)
	prometheus.Unregister(c.Durations)
}

func (c *HTTPRequestMetricsCollector) trackRequestEnd(reqInfo *httpRequestInfo, status int, startTime time.Time) {
	labels := reqInfo.makeLabels()
	labels[httpRequestMetricsLabelStatusCode] = strconv.Itoa(status)
	c.Durations.With(labels).Observe(time.Since(startTime).Seconds())
}

type httpRequestInfo struct {
	method        string
	routePattern  string
	userAgentType string
}

func (hri *httpRequestInfo) makeLabels() prometheus.Labels {
	return prometheus.Labels{
		httpRequestMetricsLabelMethod:        hri.method,
		httpRequestMetricsLabelRoutePattern:  hri.routePattern,
		httpRequestMetricsLabelUserAgentType: hri.userAgentType,
	}
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
	collector       *HTTPRequestMetricsCollector
	getRoutePattern RoutePatternGetterFunc
	opts            HTTPRequestMetricsOpts
}

// HTTPRequestMetrics is a middleware that collects metrics for incoming HTTP requests using Prometheus data types.
func HTTPRequestMetrics(
	collector *HTTPRequestMetricsCollector, getRoutePattern RoutePatternGetterFunc,
) func(next http.Handler) http.Handler {
	return HTTPRequestMetricsWithOpts(collector, getRoutePattern, HTTPRequestMetricsOpts{})
}

// HTTPRequestMetricsWithOpts is a more configurable version of HTTPRequestMetrics middleware.
func HTTPRequestMetricsWithOpts(
	collector *HTTPRequestMetricsCollector,
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

	reqInfo := &httpRequestInfo{
		method:        r.Method,
		routePattern:  h.getRoutePattern(r),
		userAgentType: h.opts.GetUserAgentType(r),
	}

	inFlightGauge := h.collector.InFlight.With(reqInfo.makeLabels())
	inFlightGauge.Inc()
	defer inFlightGauge.Dec()

	r = r.WithContext(NewContextWithHTTPMetricsEnabled(r.Context()))

	wrw := WrapResponseWriterIfNeeded(rw, r.ProtoMajor)
	defer func() {
		if !IsHTTPMetricsEnabledInContext(r.Context()) {
			return
		}

		if reqInfo.routePattern == "" {
			reqInfo.routePattern = h.getRoutePattern(r)
		}
		if p := recover(); p != nil {
			if p != http.ErrAbortHandler {
				h.collector.trackRequestEnd(reqInfo, http.StatusInternalServerError, startTime)
			}
			panic(p)
		}
		h.collector.trackRequestEnd(reqInfo, wrw.Status(), startTime)
	}()

	h.next.ServeHTTP(wrw, r)
}

func determineUserAgentType(r *http.Request) string {
	if strings.Contains(strings.ToLower(r.UserAgent()), "mozilla") {
		return userAgentTypeBrowser
	}
	return userAgentTypeHTTPClient
}

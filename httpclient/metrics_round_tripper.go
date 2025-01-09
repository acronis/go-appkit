/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"strconv"
	"time"
)

var (
	// ClassifyRequest does request classification, producing non-parameterized summary for given request.
	ClassifyRequest func(r *http.Request, reqType string) string
)

// MetricsCollector is an interface for collecting metrics for client requests.
type MetricsCollector interface {
	// RequestDuration observes the duration of the request and the status code.
	RequestDuration(reqType, remoteAddress, summary, status string, startTime time.Time)
}

// PrometheusMetricsCollector is a Prometheus metrics collector.
type PrometheusMetricsCollector struct {
	// Durations is a histogram of the http client requests durations.
	Durations *prometheus.HistogramVec
}

func NewPrometheusMetricsCollector(namespace string) *PrometheusMetricsCollector {
	return &PrometheusMetricsCollector{
		Durations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_client_request_duration_seconds",
			Help:      "A histogram of the http client requests durations.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 150, 300, 600},
		}, []string{"type", "remote_address", "summary", "status"}),
	}
}

// MustRegister registers the Prometheus metrics.
func (p *PrometheusMetricsCollector) MustRegister() {
	prometheus.MustRegister(p.Durations)
}

// RequestDuration observes the duration of the request and the status code.
func (p *PrometheusMetricsCollector) RequestDuration(reqType, host, summary, status string, start time.Time) {
	p.Durations.WithLabelValues(reqType, host, summary, status).Observe(time.Since(start).Seconds())
}

// Unregister the Prometheus metrics.
func (p *PrometheusMetricsCollector) Unregister() {
	prometheus.Unregister(p.Durations)
}

// MetricsRoundTripper is an HTTP transport that measures requests done.
type MetricsRoundTripper struct {
	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// ReqType is a type of request. e.g. service 'auth-service', an action 'login' or specific information to correlate.
	ReqType string

	// Collector is a metrics collector.
	Collector MetricsCollector
}

// MetricsRoundTripperOpts is an HTTP transport that measures requests done.
type MetricsRoundTripperOpts struct {
	// ReqType is a type of request. e.g. service 'auth-service', an action 'login' or specific information to correlate.
	ReqType string

	// Collector is a metrics collector.
	Collector MetricsCollector
}

// NewMetricsRoundTripper creates an HTTP transport that measures requests done.
func NewMetricsRoundTripper(delegate http.RoundTripper) http.RoundTripper {
	return NewMetricsRoundTripperWithOpts(delegate, MetricsRoundTripperOpts{})
}

// NewMetricsRoundTripperWithOpts creates an HTTP transport that measures requests done.
func NewMetricsRoundTripperWithOpts(delegate http.RoundTripper, opts MetricsRoundTripperOpts) http.RoundTripper {
	reqType := DefaultReqType
	if opts.ReqType == "" {
		reqType = opts.ReqType
	}

	return &MetricsRoundTripper{Delegate: delegate, ReqType: reqType, Collector: opts.Collector}
}

// RoundTrip measures external requests done.
func (rt *MetricsRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.Collector == nil {
		return rt.Delegate.RoundTrip(r)
	}

	status := "0"
	start := time.Now()

	resp, err := rt.Delegate.RoundTrip(r)
	if err == nil && resp != nil {
		status = strconv.Itoa(resp.StatusCode)
	}

	rt.Collector.RequestDuration(rt.ReqType, r.Host, requestSummary(r, rt.ReqType), status, start)
	return resp, err
}

// requestSummary does request classification, producing non-parameterized summary for given request.
func requestSummary(r *http.Request, reqType string) string {
	if ClassifyRequest != nil {
		return ClassifyRequest(r, reqType)
	}

	return fmt.Sprintf("%s %s", r.Method, reqType)
}

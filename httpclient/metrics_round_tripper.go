/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsCollector is an interface for collecting metrics for client requests.
type MetricsCollector interface {
	// RequestDuration observes the duration of the request and the status code.
	RequestDuration(requestType, remoteAddress, summary, status string, duration float64)
}

// PrometheusMetricsCollector is a Prometheus metrics collector.
type PrometheusMetricsCollector struct {
	// Durations is a histogram of the http client requests durations.
	Durations *prometheus.HistogramVec
}

// NewPrometheusMetricsCollector creates a new Prometheus metrics collector.
func NewPrometheusMetricsCollector(namespace string) *PrometheusMetricsCollector {
	return &PrometheusMetricsCollector{
		Durations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_client_request_duration_seconds",
			Help:      "A histogram of the http client requests durations.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 150, 300, 600},
		}, []string{"client_type", "remote_address", "summary", "status"}),
	}
}

// MustRegister registers the Prometheus metrics.
func (p *PrometheusMetricsCollector) MustRegister() {
	prometheus.MustRegister(p.Durations)
}

// RequestDuration observes the duration of the request and the status code.
func (p *PrometheusMetricsCollector) RequestDuration(requestType, host, summary, status string, duration float64) {
	p.Durations.WithLabelValues(requestType, host, summary, status).Observe(duration)
}

// Unregister the Prometheus metrics.
func (p *PrometheusMetricsCollector) Unregister() {
	prometheus.Unregister(p.Durations)
}

// MetricsRoundTripper is an HTTP transport that measures requests done.
type MetricsRoundTripper struct {
	// Delegate is the next RoundTripper in the chain.
	Delegate http.RoundTripper

	// ClientType represents a type of client, it's a service component reference. e.g. 'auth-service'
	ClientType string

	// MetricsCollector is a metrics collector.
	MetricsCollector MetricsCollector

	// ClassifyRequest does request classification, producing non-parameterized summary for given request.
	ClassifyRequest func(r *http.Request, requestType string) string
}

// MetricsRoundTripperOpts is an HTTP transport that measures requests done.
type MetricsRoundTripperOpts struct {
	// ClientType represents a type of client, it's a service component reference. e.g. 'auth-service'
	ClientType string

	// ClassifyRequest does request classification, producing non-parameterized summary for given request.
	ClassifyRequest func(r *http.Request, requestType string) string
}

// NewMetricsRoundTripper creates an HTTP transport that measures requests done.
func NewMetricsRoundTripper(delegate http.RoundTripper, collector MetricsCollector) http.RoundTripper {
	return NewMetricsRoundTripperWithOpts(delegate, collector, MetricsRoundTripperOpts{})
}

// NewMetricsRoundTripperWithOpts creates an HTTP transport that measures requests done.
func NewMetricsRoundTripperWithOpts(
	delegate http.RoundTripper,
	collector MetricsCollector,
	opts MetricsRoundTripperOpts,
) http.RoundTripper {
	return &MetricsRoundTripper{
		Delegate:         delegate,
		ClientType:       opts.ClientType,
		MetricsCollector: collector,
		ClassifyRequest:  opts.ClassifyRequest,
	}
}

// RoundTrip measures external requests done.
func (rt *MetricsRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if rt.MetricsCollector == nil {
		return nil, fmt.Errorf("metrics collector is not provided")
	}

	start := time.Now()
	resp, err := rt.Delegate.RoundTrip(r)
	duration := time.Since(start).Seconds()
	status := "0"

	if err == nil && resp != nil {
		status = strconv.Itoa(resp.StatusCode)
	}

	summary := fmt.Sprintf("%s %s", r.Method, rt.ClientType)
	if rt.ClassifyRequest != nil {
		summary = rt.ClassifyRequest(r, rt.ClientType)
	}

	rt.MetricsCollector.RequestDuration(rt.ClientType, r.Host, summary, status, duration)
	return resp, err
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"fmt"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"strconv"
	"time"
)

var (
	ExternalHTTPRequestDuration *prometheus.HistogramVec
	ClassifyRequest             func(r *http.Request, reqType string, logger log.FieldLogger) string
)

// MustInitAndRegisterMetrics initializes and registers external HTTP request duration metric.
// Panic will be raised in case of error.
func MustInitAndRegisterMetrics(namespace string) {
	ExternalHTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "external_http_request_duration",
			Help:      "external HTTP request duration in seconds",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 150, 300, 600},
		},
		[]string{"type", "remote_address", "summary", "status"},
	)
	prometheus.MustRegister(ExternalHTTPRequestDuration)
}

// UnregisterMetrics unregisters external HTTP request duration metric.
func UnregisterMetrics() {
	if ExternalHTTPRequestDuration != nil {
		prometheus.Unregister(ExternalHTTPRequestDuration)
	}
}

// MetricsRoundTripper is an HTTP transport that measures requests done.
type MetricsRoundTripper struct {
	Delegate http.RoundTripper
	ReqType  string
}

// NewMetricsRoundTripper creates an HTTP transport that measures requests done.
func NewMetricsRoundTripper(delegate http.RoundTripper, reqType string) http.RoundTripper {
	return &MetricsRoundTripper{Delegate: delegate, ReqType: reqType}
}

// RoundTrip measures external requests done.
func (rt *MetricsRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if ExternalHTTPRequestDuration == nil {
		return rt.Delegate.RoundTrip(r)
	}

	logger := middleware.GetLoggerFromContext(r.Context())
	start := time.Now()
	status := "failed"

	resp, err := rt.Delegate.RoundTrip(r)
	if err == nil && resp != nil {
		status = strconv.Itoa(resp.StatusCode)
		if resp.StatusCode >= http.StatusBadRequest && logger != nil {
			logger.Warnf("external request %s %s completed with HTTP status %d", r.Method, r.URL.String(), status)
		}
	} else if err != nil && logger != nil {
		logger.Warnf("external request %s %s: %s", r.Method, r.URL.String(), err.Error())
	}

	ExternalHTTPRequestDuration.WithLabelValues(
		rt.ReqType, r.Host, requestSummary(r, rt.ReqType, logger), status,
	).Observe(time.Since(start).Seconds())

	return resp, err
}

// requestSummary does request classification, producing non-parameterized summary for given request.
func requestSummary(r *http.Request, reqType string, logger log.FieldLogger) string {
	if ClassifyRequest != nil {
		return ClassifyRequest(r, reqType, logger)
	}

	if logger != nil {
		logger.Debugf(
			"request classifier is not initialized, request %s %s will be marked as req type '%s' in metrics",
			r.Method, r.URL.String(), reqType,
		)
	}

	return fmt.Sprintf("%s %s", r.Method, reqType)
}

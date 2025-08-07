/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/acronis/go-appkit/internal/throttleconfig"
)

// MetricsCollector represents a collector of metrics for rate/in-flight limiting rejects.
type MetricsCollector = throttleconfig.MetricsCollector

// PrometheusMetrics represents a collector of Prometheus metrics for rate/in-flight limiting rejects.
type PrometheusMetrics = throttleconfig.PrometheusMetrics

// PrometheusMetricsOption configures Prometheus metrics construction for gRPC throttle interceptor.
type PrometheusMetricsOption func(*throttleconfig.PrometheusMetricsOpts)

// WithPrometheusNamespace sets the Prometheus namespace for metrics.
func WithPrometheusNamespace(ns string) PrometheusMetricsOption {
	return func(o *throttleconfig.PrometheusMetricsOpts) {
		o.Namespace = ns
	}
}

// WithPrometheusConstLabels sets constant labels applied to all metrics.
func WithPrometheusConstLabels(labels prometheus.Labels) PrometheusMetricsOption {
	return func(o *throttleconfig.PrometheusMetricsOpts) {
		if labels == nil {
			o.ConstLabels = nil
			return
		}
		labels = make(prometheus.Labels, len(labels))
		for k, v := range labels {
			labels[k] = v
		}
	}
}

// WithPrometheusCurriedLabelNames sets label names that will be required to curry later.
func WithPrometheusCurriedLabelNames(names ...string) PrometheusMetricsOption {
	return func(o *throttleconfig.PrometheusMetricsOpts) {
		if len(names) == 0 {
			o.CurriedLabelNames = nil
			return
		}
		o.CurriedLabelNames = make([]string, len(names))
		copy(o.CurriedLabelNames, names)
	}
}

// NewPrometheusMetrics creates a new instance of PrometheusMetrics with optional configuration.
// If no options are provided, defaults are used.
func NewPrometheusMetrics(options ...PrometheusMetricsOption) *throttleconfig.PrometheusMetrics {
	var opts throttleconfig.PrometheusMetricsOpts
	for _, apply := range options {
		if apply != nil {
			apply(&opts)
		}
	}
	return throttleconfig.NewPrometheusMetricsWithOpts("grpc", opts)
}

type disabledMetrics = throttleconfig.DisabledMetrics

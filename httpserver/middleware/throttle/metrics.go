/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"github.com/acronis/go-appkit/internal/throttleconfig"
)

// MetricsCollector represents a collector of metrics for rate/in-flight limiting rejects.
type MetricsCollector = throttleconfig.MetricsCollector

// PrometheusMetricsOpts represents options for PrometheusMetrics.
type PrometheusMetricsOpts = throttleconfig.PrometheusMetricsOpts

// PrometheusMetrics represents a collector of Prometheus metrics for rate/in-flight limiting rejects.
type PrometheusMetrics = throttleconfig.PrometheusMetrics

// NewPrometheusMetrics creates a new instance of PrometheusMetrics.
func NewPrometheusMetrics() *throttleconfig.PrometheusMetrics {
	return throttleconfig.NewPrometheusMetrics("")
}

// NewPrometheusMetricsWithOpts creates a new instance of PrometheusMetrics with the provided options.
func NewPrometheusMetricsWithOpts(opts throttleconfig.PrometheusMetricsOpts) *throttleconfig.PrometheusMetrics {
	return throttleconfig.NewPrometheusMetricsWithOpts("", opts)
}

type disabledMetrics = throttleconfig.DisabledMetrics

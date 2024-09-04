/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsLabelDryRun     = "dry_run"
	metricsLabelRule       = "rule"
	metricsLabelBacklogged = "backlogged"
)

const (
	metricsValYes = "yes"
	metricsValNo  = "no"
)

// MetricsCollector represents a collector of metrics for rate/in-flight limiting rejects.
type MetricsCollector interface {
	// IncInFlightLimitRejects increments the counter of rejected requests due to in-flight limit exceeded.
	IncInFlightLimitRejects(ruleName string, dryRun bool, backlogged bool)

	// IncRateLimitRejects increments the counter of rejected requests due to rate limit exceeded.
	IncRateLimitRejects(ruleName string, dryRun bool)
}

// PrometheusMetricsOpts represents options for PrometheusMetrics.
type PrometheusMetricsOpts struct {
	// Namespace is a namespace for metrics. It will be prepended to all metric names.
	Namespace string

	// ConstLabels is a set of labels that will be applied to all metrics.
	ConstLabels prometheus.Labels

	// CurriedLabelNames is a list of label names that will be curried with the provided labels.
	// See PrometheusMetrics.MustCurryWith method for more details.
	// Keep in mind that if this list is not empty,
	// PrometheusMetrics.MustCurryWith method must be called further with the same labels.
	// Otherwise, the collector will panic.
	CurriedLabelNames []string
}

// PrometheusMetrics represents a collector of Prometheus metrics for rate/in-flight limiting rejects.
type PrometheusMetrics struct {
	InFlightLimitRejects *prometheus.CounterVec
	RateLimitRejects     *prometheus.CounterVec
}

// NewPrometheusMetrics creates a new instance of PrometheusMetrics.
func NewPrometheusMetrics() *PrometheusMetrics {
	return NewPrometheusMetricsWithOpts(PrometheusMetricsOpts{})
}

// NewPrometheusMetricsWithOpts creates a new instance of PrometheusMetrics with the provided options.
func NewPrometheusMetricsWithOpts(opts PrometheusMetricsOpts) *PrometheusMetrics {
	makeLabelNames := func(names ...string) []string {
		l := append(make([]string, 0, len(opts.CurriedLabelNames)+len(names)), opts.CurriedLabelNames...)
		return append(l, names...)
	}

	inFlightLimitRejects := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   opts.Namespace,
		Name:        "in_flight_limit_rejects_total",
		Help:        "Number of rejected requests due to in-flight limit exceeded.",
		ConstLabels: opts.ConstLabels,
	}, makeLabelNames(metricsLabelDryRun, metricsLabelRule, metricsLabelBacklogged))

	rateLimitRejects := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   opts.Namespace,
		Name:        "rate_limit_rejects_total",
		Help:        "Number of rejected requests due to rate limit exceeded.",
		ConstLabels: opts.ConstLabels,
	}, makeLabelNames(metricsLabelDryRun, metricsLabelRule))

	return &PrometheusMetrics{
		InFlightLimitRejects: inFlightLimitRejects,
		RateLimitRejects:     rateLimitRejects,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (pm *PrometheusMetrics) MustCurryWith(labels prometheus.Labels) *PrometheusMetrics {
	return &PrometheusMetrics{
		InFlightLimitRejects: pm.InFlightLimitRejects.MustCurryWith(labels),
		RateLimitRejects:     pm.RateLimitRejects.MustCurryWith(labels),
	}
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (pm *PrometheusMetrics) MustRegister() {
	prometheus.MustRegister(
		pm.InFlightLimitRejects,
		pm.RateLimitRejects,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (pm *PrometheusMetrics) Unregister() {
	prometheus.Unregister(pm.InFlightLimitRejects)
	prometheus.Unregister(pm.RateLimitRejects)
}

// IncInFlightLimitRejects increments the counter of rejected requests due to in-flight limit exceeded.
func (pm *PrometheusMetrics) IncInFlightLimitRejects(ruleName string, dryRun bool, backlogged bool) {
	dryRunVal := metricsValNo
	if dryRun {
		dryRunVal = metricsValYes
	}
	backloggedVal := metricsValNo
	if backlogged {
		backloggedVal = metricsValYes
	}
	pm.InFlightLimitRejects.With(prometheus.Labels{
		metricsLabelDryRun:     dryRunVal,
		metricsLabelRule:       ruleName,
		metricsLabelBacklogged: backloggedVal,
	}).Inc()
}

// IncRateLimitRejects increments the counter of rejected requests due to rate limit exceeded.
func (pm *PrometheusMetrics) IncRateLimitRejects(ruleName string, dryRun bool) {
	dryRunVal := metricsValNo
	if dryRun {
		dryRunVal = metricsValYes
	}
	pm.RateLimitRejects.With(prometheus.Labels{
		metricsLabelDryRun: dryRunVal,
		metricsLabelRule:   ruleName,
	}).Inc()
}

type disabledMetrics struct{}

func (disabledMetrics) IncInFlightLimitRejects(string, bool, bool) {}
func (disabledMetrics) IncRateLimitRejects(string, bool)           {}

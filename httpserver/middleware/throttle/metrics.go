/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import "github.com/prometheus/client_golang/prometheus"

const (
	metricsLabelDryRun     = "dry_run"
	metricsLabelRule       = "rule"
	metricsLabelBacklogged = "backlogged"
)

const (
	metricsValYes = "yes"
	metricsValNo  = "no"
)

// MetricsCollector represents collector of metrics for rate/in-flight limiting rejects.
type MetricsCollector struct {
	InFlightLimitRejects *prometheus.CounterVec
	RateLimitRejects     *prometheus.CounterVec
}

// NewMetricsCollector creates a new instance of MetricsCollector.
func NewMetricsCollector(namespace string) *MetricsCollector {
	inFlightLimitRejects := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "in_flight_limit_rejects_total",
		Help:      "Number of rejected requests due to in-flight limit exceeded.",
	}, []string{metricsLabelDryRun, metricsLabelRule, metricsLabelBacklogged})

	rateLimitRejects := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rate_limit_rejects_total",
		Help:      "Number of rejected requests due to rate limit exceeded.",
	}, []string{metricsLabelDryRun, metricsLabelRule})

	return &MetricsCollector{
		InFlightLimitRejects: inFlightLimitRejects,
		RateLimitRejects:     rateLimitRejects,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (mc *MetricsCollector) MustCurryWith(labels prometheus.Labels) *MetricsCollector {
	return &MetricsCollector{
		InFlightLimitRejects: mc.InFlightLimitRejects.MustCurryWith(labels),
		RateLimitRejects:     mc.RateLimitRejects.MustCurryWith(labels),
	}
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (mc *MetricsCollector) MustRegister() {
	prometheus.MustRegister(
		mc.InFlightLimitRejects,
		mc.RateLimitRejects,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (mc *MetricsCollector) Unregister() {
	prometheus.Unregister(mc.InFlightLimitRejects)
	prometheus.Unregister(mc.RateLimitRejects)
}

func makeCommonPromLabels(dryRun bool, rule string) prometheus.Labels {
	dryRunVal := metricsValNo
	if dryRun {
		dryRunVal = metricsValYes
	}
	return prometheus.Labels{metricsLabelDryRun: dryRunVal, metricsLabelRule: rule}
}

func makePromLabelsForInFlightLimit(commonLabels prometheus.Labels, backlogged bool) prometheus.Labels {
	backloggedVal := metricsValNo
	if backlogged {
		backloggedVal = metricsValYes
	}
	return prometheus.Labels{
		metricsLabelDryRun:     commonLabels[metricsLabelDryRun],
		metricsLabelRule:       commonLabels[metricsLabelRule],
		metricsLabelBacklogged: backloggedVal,
	}
}

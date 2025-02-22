/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import "github.com/prometheus/client_golang/prometheus"

// MetricsCollector represents a collector of metrics to analyze how (effectively or not) cache is used.
type MetricsCollector interface {
	// SetAmount sets the total number of entries in the cache.
	SetAmount(int)

	// IncHits increments the total number of successfully found keys in the cache.
	IncHits()

	// IncMisses increments the total number of not found keys in the cache.
	IncMisses()

	// AddEvictions increments the total number of evicted entries.
	AddEvictions(int)
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

// PrometheusMetrics represents a Prometheus metrics for the cache.
type PrometheusMetrics struct {
	EntriesAmount  *prometheus.GaugeVec
	HitsTotal      *prometheus.CounterVec
	MissesTotal    *prometheus.CounterVec
	EvictionsTotal *prometheus.CounterVec
}

// NewPrometheusMetrics creates a new instance of PrometheusMetrics with default options.
func NewPrometheusMetrics() *PrometheusMetrics {
	return NewPrometheusMetricsWithOpts(PrometheusMetricsOpts{})
}

// NewPrometheusMetricsWithOpts creates a new instance of PrometheusMetrics with the provided options.
func NewPrometheusMetricsWithOpts(opts PrometheusMetricsOpts) *PrometheusMetrics {
	entriesAmount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace:   opts.Namespace,
			Name:        "cache_entries_amount",
			Help:        "Total number of entries in the cache.",
			ConstLabels: opts.ConstLabels,
		},
		opts.CurriedLabelNames,
	)

	hitsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   opts.Namespace,
			Name:        "cache_hits_total",
			Help:        "Number of successfully found keys in the cache.",
			ConstLabels: opts.ConstLabels,
		},
		opts.CurriedLabelNames,
	)

	missesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   opts.Namespace,
			Name:        "cache_misses_total",
			Help:        "Number of not found keys in cache.",
			ConstLabels: opts.ConstLabels,
		},
		opts.CurriedLabelNames,
	)

	evictionsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace:   opts.Namespace,
			Name:        "cache_evictions_total",
			Help:        "Number of evicted entries.",
			ConstLabels: opts.ConstLabels,
		},
		opts.CurriedLabelNames,
	)

	return &PrometheusMetrics{
		EntriesAmount:  entriesAmount,
		HitsTotal:      hitsTotal,
		MissesTotal:    missesTotal,
		EvictionsTotal: evictionsTotal,
	}
}

// MustCurryWith curries the metrics collector with the provided labels.
func (pm *PrometheusMetrics) MustCurryWith(labels prometheus.Labels) *PrometheusMetrics {
	return &PrometheusMetrics{
		EntriesAmount:  pm.EntriesAmount.MustCurryWith(labels),
		HitsTotal:      pm.HitsTotal.MustCurryWith(labels),
		MissesTotal:    pm.MissesTotal.MustCurryWith(labels),
		EvictionsTotal: pm.EvictionsTotal.MustCurryWith(labels),
	}
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (pm *PrometheusMetrics) MustRegister() {
	prometheus.MustRegister(
		pm.EntriesAmount,
		pm.HitsTotal,
		pm.MissesTotal,
		pm.EvictionsTotal,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (pm *PrometheusMetrics) Unregister() {
	prometheus.Unregister(pm.EntriesAmount)
	prometheus.Unregister(pm.HitsTotal)
	prometheus.Unregister(pm.MissesTotal)
	prometheus.Unregister(pm.EvictionsTotal)
}

// SetAmount sets the total number of entries in the cache.
func (pm *PrometheusMetrics) SetAmount(amount int) {
	pm.EntriesAmount.With(nil).Set(float64(amount))
}

// IncHits increments the total number of successfully found keys in the cache.
func (pm *PrometheusMetrics) IncHits() {
	pm.HitsTotal.With(nil).Inc()
}

// IncMisses increments the total number of not found keys in the cache.
func (pm *PrometheusMetrics) IncMisses() {
	pm.MissesTotal.With(nil).Inc()
}

// AddEvictions increments the total number of evicted entries.
func (pm *PrometheusMetrics) AddEvictions(n int) {
	pm.EvictionsTotal.With(nil).Add(float64(n))
}

type disabledMetrics struct{}

func (disabledMetrics) SetAmount(int)    {}
func (disabledMetrics) IncHits()         {}
func (disabledMetrics) IncMisses()       {}
func (disabledMetrics) AddEvictions(int) {}

var disabledMetricsCollector = disabledMetrics{}

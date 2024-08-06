/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import "github.com/prometheus/client_golang/prometheus"

const metricsLabelEntryType = "entry_type"

type entryTypeMetrics struct {
	Amount         prometheus.Gauge
	HitsTotal      prometheus.Counter
	MissesTotal    prometheus.Counter
	EvictionsTotal prometheus.Counter
}

// MetricsCollector represents collector of metrics for analyze how (effectively or not) cache is used.
type MetricsCollector struct {
	EntriesAmount  *prometheus.GaugeVec
	HitsTotal      *prometheus.CounterVec
	MissesTotal    *prometheus.CounterVec
	EvictionsTotal *prometheus.CounterVec

	entryTypeMetrics []*entryTypeMetrics
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(namespace string) *MetricsCollector {
	entriesAmount := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "cache_entries_amount",
			Help:      "Total number of entries in the cache.",
		},
		[]string{metricsLabelEntryType},
	)

	hitsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_hits_total",
			Help:      "Number of successfully found keys in the cache.",
		},
		[]string{metricsLabelEntryType},
	)

	missesTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_misses_total",
			Help:      "Number of not found keys in cache.",
		},
		[]string{metricsLabelEntryType},
	)

	evictionsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cache_evictions_total",
			Help:      "Number of evicted entries.",
		},
		[]string{metricsLabelEntryType},
	)

	mc := &MetricsCollector{
		EntriesAmount:  entriesAmount,
		HitsTotal:      hitsTotal,
		MissesTotal:    missesTotal,
		EvictionsTotal: evictionsTotal,
	}
	mc.SetupEntryTypeLabels(map[EntryType]string{EntryTypeDefault: "default"})
	return mc
}

// MustRegister does registration of metrics collector in Prometheus and panics if any error occurs.
func (c *MetricsCollector) MustRegister() {
	prometheus.MustRegister(
		c.EntriesAmount,
		c.HitsTotal,
		c.MissesTotal,
		c.EvictionsTotal,
	)
}

// Unregister cancels registration of metrics collector in Prometheus.
func (c *MetricsCollector) Unregister() {
	prometheus.Unregister(c.EntriesAmount)
	prometheus.Unregister(c.HitsTotal)
	prometheus.Unregister(c.MissesTotal)
	prometheus.Unregister(c.EvictionsTotal)
}

// SetupEntryTypeLabels setups its own Prometheus metrics label for each storing cacheEntry type.
func (c *MetricsCollector) SetupEntryTypeLabels(labels map[EntryType]string) {
	metrics := make([]*entryTypeMetrics, len(labels))
	for entryType, label := range labels {
		metrics[entryType] = &entryTypeMetrics{
			Amount:         c.EntriesAmount.WithLabelValues(label),
			HitsTotal:      c.HitsTotal.WithLabelValues(label),
			MissesTotal:    c.MissesTotal.WithLabelValues(label),
			EvictionsTotal: c.EvictionsTotal.WithLabelValues(label),
		}
	}
	c.entryTypeMetrics = metrics
}

func (c *MetricsCollector) getEntryTypeMetrics(entryType EntryType) *entryTypeMetrics {
	return c.entryTypeMetrics[entryType]
}

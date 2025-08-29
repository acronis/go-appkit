/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/acronis/go-appkit/internal/libinfo"
)

var metricsResponseErrors *prometheus.CounterVec

const (
	metricsSubsystem = "restapi"

	metricsLabelResponseErrorDomain = "domain"
	metricsLabelResponseErrorCode   = "code"
)

// MustInitAndRegisterMetrics initializes and registers restapi global metrics. Panic will be raised in case of error.
func MustInitAndRegisterMetrics(namespace string) {
	metricsResponseErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Subsystem: metricsSubsystem,
		Name:      "response_errors",
		Help:      "The total number of REST API errors that were respond.",
		ConstLabels: prometheus.Labels{
			libinfo.PrometheusLibVersionLabel: libinfo.GetLibVersion(),
		},
	}, []string{metricsLabelResponseErrorDomain, metricsLabelResponseErrorCode})
	prometheus.MustRegister(metricsResponseErrors)
}

// UnregisterMetrics unregisters restapi global metrics.
func UnregisterMetrics() {
	if metricsResponseErrors != nil {
		prometheus.Unregister(metricsResponseErrors)
	}
}

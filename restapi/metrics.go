/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import "github.com/prometheus/client_golang/prometheus"

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
	}, []string{metricsLabelResponseErrorDomain, metricsLabelResponseErrorCode})
	prometheus.MustRegister(metricsResponseErrors)
}

// UnregisterMetrics unregisters restapi global metrics.
func UnregisterMetrics() {
	if metricsResponseErrors != nil {
		prometheus.Unregister(metricsResponseErrors)
	}
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

// Unit is a common interface for all service units. Unit is a single component in service with its own life-cycle.
type Unit interface {
	Start(fatalError chan<- error)
	Stop(gracefully bool) error
}

// MetricsRegisterer is an interface for objects that can register its own metrics.
type MetricsRegisterer interface {
	MustRegisterMetrics()
	UnregisterMetrics()
}

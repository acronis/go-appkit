/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

// Unit represents a service unit that can be started and stopped.
// Each Unit is a distinct component within a service, with its own lifecycle.
type Unit interface {
	// Start begins the unit's operation.
	//
	// On the happy path, an implementation may perform necessary initialization and return immediately,
	// or block the calling goroutine for the duration of the unit's lifetime.
	// The Stop method may be called regardless of whether the unit has started successfully, failed, or
	// is still running.
	//
	// If Start succeeds, it must not write anything to the provided error channel.
	// Additionally, the channel must not be used after the Start method has returned.
	Start(fatalErr chan<- error)

	// Stop halts the unit.
	//
	// If 'gracefully' is true, the unit should attempt a clean shutdown.
	// Note that this method may be called even if Start has failed or was never called.
	Stop(gracefully bool) error
}

// MetricsRegisterer is an interface for objects that can register its own metrics.
type MetricsRegisterer interface {
	MustRegisterMetrics()
	UnregisterMetrics()
}

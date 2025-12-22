/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"strings"
	"sync"
	"sync/atomic"
)

// CompositeUnit represents a composition of service units and implements Composite design pattern.
type CompositeUnit struct {
	Units []Unit
}

// NewCompositeUnit creates a new composite unit.
func NewCompositeUnit(units ...Unit) *CompositeUnit {
	return &CompositeUnit{units}
}

// Start launches all units in the composition concurrently, each in its own goroutine, by calling their Start methods.
// It blocks until all Start method invocations return.
//
// If any unit writes to the provided error channel upon returning, the method attempts to stop all other units
// (non-gracefully) by calling their Stop methods. A CompositeUnitError - potentially including errors from the stop
// operations — is then sent to the provided channel.
func (cu *CompositeUnit) Start(fatalError chan<- error) {
	fatalErrs := make([]chan error, len(cu.Units))
	for i := 0; i < len(fatalErrs); i++ {
		fatalErrs[i] = make(chan error, 1)
	}

	ok := make(chan bool, len(cu.Units))
	runningOrFailedUnits := int32(len(cu.Units)) //nolint:gosec // unit count is reasonable
	for i := 0; i < len(cu.Units); i++ {
		go func(i int) {
			cu.Units[i].Start(fatalErrs[i])
			if len(fatalErrs[i]) != 0 {
				ok <- false
				return
			}
			if atomic.AddInt32(&runningOrFailedUnits, -1) == 0 {
				ok <- true
			}
		}(i)
	}

	if <-ok {
		return
	}

	stopErr := cu.Stop(false)

	var errs []error
	for _, fatalErr := range fatalErrs {
		select {
		case err := <-fatalErr:
			errs = append(errs, err)
		default:
		}
	}
	if stopErr != nil {
		errs = append(errs, stopErr.(*CompositeUnitError).UnitErrors...)
	}
	if len(errs) > 0 {
		fatalError <- &CompositeUnitError{errs}
	}
}

// Stop stops all units in the composition (each in its own separate goroutine).
// Errors that occurred while stopping the units are collected and single CompositeUnitError is returned.
func (cu *CompositeUnit) Stop(gracefully bool) error {
	results := make(chan error, len(cu.Units))

	var wg sync.WaitGroup
	wg.Add(len(cu.Units))
	for _, s := range cu.Units {
		go func(s Unit) {
			defer wg.Done()
			results <- s.Stop(gracefully)
		}(s)
	}
	wg.Wait()

	var errs []error
	for i := 0; i < len(cu.Units); i++ {
		if err := <-results; err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return &CompositeUnitError{errs}
	}
	return nil
}

// MustRegisterMetrics registers metrics in Prometheus client and panics if any error occurs.
func (cu *CompositeUnit) MustRegisterMetrics() {
	for _, s := range cu.Units {
		if mr, ok := s.(MetricsRegisterer); ok {
			mr.MustRegisterMetrics()
		}
	}
}

// UnregisterMetrics unregisters metrics in Prometheus client.
func (cu *CompositeUnit) UnregisterMetrics() {
	for _, s := range cu.Units {
		if mr, ok := s.(MetricsRegisterer); ok {
			mr.UnregisterMetrics()
		}
	}
}

// CompositeUnitError is an error which may occurs in CompositeUnit's methods.
type CompositeUnitError struct {
	UnitErrors []error
}

// Error returns a string representation of a units composition error.
func (cue *CompositeUnitError) Error() string {
	msgs := make([]string, 0, len(cue.UnitErrors))
	for _, err := range cue.UnitErrors {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

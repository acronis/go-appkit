/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"context"
	"errors"
	"time"
)

// ErrWorkerUnitStopTimeoutExceeded is an error that occurs when WorkerUnit's gracefully stop timeout is exceeded.
var ErrWorkerUnitStopTimeoutExceeded = errors.New("worker unit stop timeout exceeded")

// WorkerUnit allows presenting Worker as Unit.
type WorkerUnit struct {
	start             func(fatalError chan<- error)
	stop              func(gracefully bool) error
	metricsRegisterer MetricsRegisterer
}

// WorkerUnitOpts contains optional parameters for constructing PeriodicWorker.
type WorkerUnitOpts struct {
	MetricsRegisterer   MetricsRegisterer
	GracefulStopTimeout time.Duration
}

// NewWorkerUnit creates a new instance of WorkerUnit.
func NewWorkerUnit(worker Worker) *WorkerUnit {
	return NewWorkerUnitWithOpts(worker, WorkerUnitOpts{})
}

// NewWorkerUnitWithOpts creates a new instance of WorkerUnit
// with an ability to specify different optional parameters.
func NewWorkerUnitWithOpts(worker Worker, opts WorkerUnitOpts) *WorkerUnit {
	ctx, ctxCancel := context.WithCancel(context.Background())
	stopDone := make(chan struct{}, 1)

	start := func(fatalError chan<- error) {
		if err := worker.Run(ctx); err != nil {
			fatalError <- err
		}
		stopDone <- struct{}{}
	}

	stop := func(gracefully bool) error {
		ctxCancel()
		if !gracefully {
			return nil
		}
		if opts.GracefulStopTimeout == 0 {
			<-stopDone
			return nil
		}
		select {
		case <-stopDone:
		case <-time.After(opts.GracefulStopTimeout):
			return ErrWorkerUnitStopTimeoutExceeded
		}
		return nil
	}

	return &WorkerUnit{start: start, stop: stop, metricsRegisterer: opts.MetricsRegisterer}
}

// Start starts (call Run() method) underlying Worker.
func (u *WorkerUnit) Start(fatalError chan<- error) {
	u.start(fatalError)
}

// Stop stops underlying Worker.
func (u *WorkerUnit) Stop(gracefully bool) error {
	return u.stop(gracefully)
}

// MustRegisterMetrics registers underlying Worker's metrics.
func (u *WorkerUnit) MustRegisterMetrics() {
	if u.metricsRegisterer != nil {
		u.metricsRegisterer.MustRegisterMetrics()
	}
}

// UnregisterMetrics unregisters underlying Worker's metrics.
func (u *WorkerUnit) UnregisterMetrics() {
	if u.metricsRegisterer != nil {
		u.metricsRegisterer.UnregisterMetrics()
	}
}

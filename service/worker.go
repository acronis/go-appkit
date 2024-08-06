/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"git.acronis.com/abc/go-libs/v2/log"
)

// ErrPeriodicWorkerStop is an error that may be used for interrupting PeriodicWorker's loop.
var ErrPeriodicWorkerStop = errors.New("stop periodic worker error")

// Worker performs some (usually long-running) work.
type Worker interface {
	Run(ctx context.Context) error
}

// WorkerFunc is an adapter to allow the use of ordinary functions as Worker.
type WorkerFunc func(ctx context.Context) error

// Run is a part of Worker interface.
func (f WorkerFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// PeriodicWorker represents a worker that runs underlying worker periodically.
type PeriodicWorker struct {
	worker            Worker
	logger            log.FieldLogger
	initialDelay      time.Duration
	intervalDelay     time.Duration
	intervalDelayFunc func(worker Worker, err error) time.Duration
}

// PeriodicWorkerOpts contains optional parameters for constructing PeriodicWorker.
type PeriodicWorkerOpts struct {
	InitialDelay      time.Duration
	IntervalDelayFunc func(worker Worker, err error) time.Duration
}

// NewPeriodicWorker creates a new instance of PeriodicWorker with constant delays.
func NewPeriodicWorker(worker Worker, intervalDelay time.Duration, logger log.FieldLogger) *PeriodicWorker {
	return NewPeriodicWorkerWithOpts(worker, intervalDelay, logger, PeriodicWorkerOpts{})
}

// NewPeriodicWorkerWithOpts creates a new instance of PeriodicWorker
// with an ability to specify different optional parameters.
func NewPeriodicWorkerWithOpts(
	worker Worker, intervalDelay time.Duration, logger log.FieldLogger, opts PeriodicWorkerOpts,
) *PeriodicWorker {
	return &PeriodicWorker{
		worker:            worker,
		initialDelay:      opts.InitialDelay,
		intervalDelay:     intervalDelay,
		intervalDelayFunc: opts.IntervalDelayFunc,
		logger:            logger,
	}
}

// Run runs PeriodicWorker loop.
func (pw *PeriodicWorker) Run(ctx context.Context) (resErr error) {
	defer func() {
		if p := recover(); p != nil {
			const logStackSize = 8192
			stack := make([]byte, logStackSize)
			stack = stack[:runtime.Stack(stack, false)]
			pw.logger.Error(fmt.Sprintf("panic: %+v", p), log.Bytes("stack", stack))
			panic(p)
		}
		if resErr != nil {
			pw.logger.Error("periodic worker stopped with error", log.Error(resErr))
			return
		}
		pw.logger.Info("periodic worker stopped successfully")
	}()

	pw.logger.Infof("running periodic worker (initialDelay=%s, intervalDelay=%s)...",
		pw.initialDelay, pw.intervalDelay)

	timer := time.NewTimer(pw.initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}

		err := pw.worker.Run(ctx)
		if err != nil {
			if errors.Is(err, ErrPeriodicWorkerStop) {
				return nil
			}
			pw.logger.Error("periodically running worker finished with error", log.Error(err))
		}

		nextDelay := pw.intervalDelay
		if pw.intervalDelayFunc != nil {
			nextDelay = pw.intervalDelayFunc(pw.worker, err)
		}

		timer.Stop()
		timer = time.NewTimer(nextDelay)
	}
}

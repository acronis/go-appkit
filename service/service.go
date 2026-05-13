/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/acronis/go-appkit/log"
)

// Opts represents an options for Service.
type Opts struct {
	// ShutdownSignals is the list of OS signals that trigger a graceful
	// shutdown of the Service when received. New defaults to SIGINT and SIGTERM.
	ShutdownSignals []os.Signal

	// StopTimeout, when greater than zero, makes StartContext wait for the
	// Unit's Start goroutine to exit before returning, up to this duration.
	// Zero means do not wait.
	StopTimeout time.Duration
}

// Service represents a service which can register metrics in Prometheus client,
// start unit and stop it in a graceful way by OS signal.
type Service struct {
	Unit    Unit
	Signals chan os.Signal
	Logger  log.FieldLogger
	Opts    Opts
}

// New creates new Service which will start and stop passing unit.
func New(logger log.FieldLogger, unit Unit) *Service {
	return NewWithOpts(logger, unit, Opts{
		ShutdownSignals: []os.Signal{syscall.SIGINT, syscall.SIGTERM},
	})
}

// NewWithOpts is a more configurable version of New.
func NewWithOpts(logger log.FieldLogger, unit Unit, opts Opts) *Service {
	return &Service{
		Signals: make(chan os.Signal, 1),
		Unit:    unit,
		Logger:  logger,
		Opts:    opts,
	}
}

// Start wraps StartContext using the background context.
// See StartContext for the full description of the return-timing semantics,
// in particular how Opts.StopTimeout affects when this method returns.
func (s *Service) Start() error {
	return s.StartContext(context.Background())
}

// StartContext starts the service Unit in a separate goroutine and blocks
// until one of the following happens:
//   - ctx is canceled — Unit.Stop(true) is called;
//   - Unit reports a fatal error on its error channel — returned as an error;
//   - one of Opts.ShutdownSignals is received — Unit.Stop(true) is called.
//
// Return-timing depends on Opts.StopTimeout:
//   - 0 (default): returns once the trigger is handled. Synchronizing
//     Start's exit is up to the Unit's Stop, otherwise Start may outlive
//     StartContext.
//   - > 0: additionally waits for Unit.Start to exit, up to StopTimeout;
//     on timeout returns anyway and logs a warning. Recommended when Stop
//     does not itself guarantee Start has unwound.
func (s *Service) StartContext(ctx context.Context) error {
	if mr, ok := s.Unit.(MetricsRegisterer); ok {
		mr.MustRegisterMetrics()
		defer mr.UnregisterMetrics()
	}

	fatalError := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		s.Unit.Start(fatalError)
	}()

	signal.Notify(s.Signals, s.Opts.ShutdownSignals...)

	var retErr error
	select {
	case <-ctx.Done():
		s.Logger.Info("context is canceled, service will be stopped")
		if err := s.Unit.Stop(true); err != nil {
			retErr = fmt.Errorf("stop service gracefully: %w", err)
		}
	case err := <-fatalError:
		s.Logger.Error("service fatal error", log.Error(err))
		retErr = fmt.Errorf("fatal error: %w", err)
	case sig := <-s.Signals:
		s.Logger.Info("service got signal", log.String("signal", sig.String()))
		if err := s.Unit.Stop(true); err != nil {
			retErr = fmt.Errorf("stop service gracefully: %w", err)
		}
	}

	if s.Opts.StopTimeout > 0 {
		select {
		case <-done:
		case <-time.After(s.Opts.StopTimeout):
			s.Logger.Warn("unit's Start did not exit within stop timeout",
				log.String("timeout", s.Opts.StopTimeout.String()))
		}
	}

	return retErr
}

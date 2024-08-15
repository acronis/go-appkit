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

	"github.com/acronis/go-appkit/log"
)

// Opts represents an options for Service.
type Opts struct {
	ShutdownSignals []os.Signal
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
func (s *Service) Start() error {
	return s.StartContext(context.Background())
}

// StartContext starts service unit in the separate goroutine and
// blocks until fatal error occurs or any of the OS shutting down signals are received.
func (s *Service) StartContext(ctx context.Context) error {
	if mr, ok := s.Unit.(MetricsRegisterer); ok {
		mr.MustRegisterMetrics()
		defer mr.UnregisterMetrics()
	}

	fatalError := make(chan error, 1)

	go s.Unit.Start(fatalError)

	signal.Notify(s.Signals, s.Opts.ShutdownSignals...)

	select {
	case <-ctx.Done():
		s.Logger.Info("context is canceled, service will be stopped")
		if err := s.Unit.Stop(true); err != nil {
			return fmt.Errorf("stop service gracefully: %w", err)
		}
	case err := <-fatalError:
		s.Logger.Error("service fatal error", log.Error(err))
		return fmt.Errorf("fatal error: %w", err)
	case sig := <-s.Signals:
		s.Logger.Info("service got signal", log.String("signal", sig.String()))
		if err := s.Unit.Stop(true); err != nil {
			return fmt.Errorf("stop service gracefully: %w", err)
		}
	}

	return nil
}

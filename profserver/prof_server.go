/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package profserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"git.acronis.com/abc/go-libs/v2/httpserver/middleware"
	"git.acronis.com/abc/go-libs/v2/log"
	"git.acronis.com/abc/go-libs/v2/service"
)

// ProfServer represents HTTP server for profiling. pprof is used under the hood.
// It implements service.Unit interface.
type ProfServer struct {
	URL            string
	HTTPServer     *http.Server
	httpServerDone chan struct{}
	Logger         log.FieldLogger
}

var _ service.Unit = (*ProfServer)(nil)

// New creates a new HTTP server (pprof) for profiling.
func New(cfg *Config, logger log.FieldLogger) *ProfServer {
	router := chi.NewRouter()
	router.Use(
		middleware.RequestID(),
		middleware.LoggingWithOpts(logger, middleware.LoggingOpts{RequestStart: true}),
	)
	router.Mount("/debug", chimiddleware.Profiler())

	httpServer := &http.Server{
		Addr:              cfg.Address,
		Handler:           router,
		ReadHeaderTimeout: time.Second * 5,
	}

	return &ProfServer{
		URL:            "http://" + httpServer.Addr,
		HTTPServer:     httpServer,
		httpServerDone: make(chan struct{}),
		Logger:         logger,
	}
}

// Start starts profiling HTTP server in a blocking way. Supposed this methods will be called in a separate goroutine.
// If a fatal error occurs, it's sent into passed fatalError channel and should be processed outside.
func (s *ProfServer) Start(fatalError chan<- error) {
	defer close(s.httpServerDone)

	logger := s.Logger.With(log.String("address", s.HTTPServer.Addr))

	logger.Info("starting profiling HTTP server...")
	if err := s.HTTPServer.ListenAndServe(); err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			logger.Info("profiling HTTP served closed")
			return
		}
		logger.Error("profiling HTTP server error", log.Error(err))
		fatalError <- err
		return
	}
}

// Stop stops profiling HTTP server (always in no gracefully way).
func (s *ProfServer) Stop(gracefully bool) error {
	s.Logger.Info("closing profiling HTTP server...")
	if err := s.HTTPServer.Close(); err != nil {
		s.Logger.Error("profiling HTTP server closing error", log.Error(err))
		return err
	}
	<-s.httpServerDone // Wait closing of listener.
	return nil
}

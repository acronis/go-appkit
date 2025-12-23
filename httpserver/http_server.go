/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/service"
)

const (
	networkTCP  = "tcp"
	networkUnix = "unix"
)

// systemEndpoints is a list of endpoints which are not involved in metrics collecting, and in-flight requests limiting.
var systemEndpoints = []string{"/metrics", "/healthz"}

// APIVersion is a type alias for API version.
type APIVersion = int

// APIRoute is a type alias for single API route.
type APIRoute = func(router chi.Router)

// HTTPRequestMetricsOpts represents options for HTTPRequestMetricsOpts middleware that used in HTTPServer.
type HTTPRequestMetricsOpts struct {
	// Metrics opts.
	Namespace       string
	DurationBuckets []float64
	ConstLabels     prometheus.Labels

	// Middleware opts.
	// Deprecated: GetUserAgentType be removed in the next major version. Please use CustomLabels with Context instead.
	GetUserAgentType middleware.UserAgentTypeGetterFunc
	GetRoutePattern  middleware.RoutePatternGetterFunc
}

// Opts represents options for creating HTTPServer.
type Opts struct {
	// ServiceNameInURL is a prefix for API routes (e.g., "/api/service_name/v1").
	ServiceNameInURL string
	// APIRoutes is a map of API versions to their route configuration functions.
	APIRoutes map[APIVersion]APIRoute
	// RootMiddlewares is a list of middlewares to be applied to the root router.
	RootMiddlewares []func(http.Handler) http.Handler
	// ErrorDomain is used for error response formatting.
	ErrorDomain string
	// HealthCheck is a function that performs health check logic.
	HealthCheck HealthCheck
	// HealthCheckContext is a function that performs context-aware health check logic.
	HealthCheckContext HealthCheckContext
	// MetricsHandler is a custom handler for the /metrics endpoint (e.g., Prometheus handler).
	MetricsHandler http.Handler
	// HTTPRequestMetrics contains options for configuring HTTP request metrics middleware.
	HTTPRequestMetrics HTTPRequestMetricsOpts
	// Handler is a custom HTTP handler to use instead of the default router with middlewares.
	// When provided, default middlewares are not applied.
	Handler http.Handler
	// Listener is a pre-configured network listener to use instead of creating a new one.
	// Useful for custom listener configurations or testing with mock listeners.
	Listener net.Listener
}

func (opts Opts) routerOpts() RouterOpts {
	return RouterOpts{
		ServiceNameInURL:   opts.ServiceNameInURL,
		APIRoutes:          opts.APIRoutes,
		RootMiddlewares:    opts.RootMiddlewares,
		ErrorDomain:        opts.ErrorDomain,
		HealthCheck:        opts.HealthCheck,
		HealthCheckContext: opts.HealthCheckContext,
		MetricsHandler:     opts.MetricsHandler,
	}
}

// HTTPServer represents a wrapper around http.Server with additional fields and methods.
// chi.Router is used as a handler for the server by default.
// It also implements service.Unit and service.MetricsRegisterer interfaces.
type HTTPServer struct {
	// TODO: URL does not contain port when port is dynamically chosen
	URL             string
	HTTPServer      *http.Server
	UnixSocketPath  string
	TLS             TLSConfig
	HTTPRouter      chi.Router
	Logger          log.FieldLogger
	ShutdownTimeout time.Duration

	listener                 net.Listener
	port                     int32
	httpServerDone           atomic.Value
	httpReqPrometheusMetrics *middleware.HTTPRequestPrometheusMetrics
}

var _ service.Unit = (*HTTPServer)(nil)
var _ service.MetricsRegisterer = (*HTTPServer)(nil)

// New creates a new HTTPServer with predefined logging, metrics collecting,
// recovering after panics and health-checking functionality.
func New(cfg *Config, logger log.FieldLogger, opts Opts) (*HTTPServer, error) { //nolint // hugeParam: opts is heavy, it's ok in this case.
	if opts.Handler != nil {
		return newWithHandler(cfg, logger, opts.Handler, opts.Listener), nil
	}

	httpReqPromMetrics := middleware.NewHTTPRequestPrometheusMetricsWithOpts(
		middleware.HTTPRequestPrometheusMetricsOpts{
			Namespace:       opts.HTTPRequestMetrics.Namespace,
			DurationBuckets: opts.HTTPRequestMetrics.DurationBuckets,
			ConstLabels:     opts.HTTPRequestMetrics.ConstLabels,
		})
	router := chi.NewRouter()
	if err := applyDefaultMiddlewaresToRouter(router, cfg, logger, opts, httpReqPromMetrics); err != nil {
		return nil, err
	}
	configureRouter(router, logger, opts.routerOpts())

	appSrv := newWithHandler(cfg, logger, router, opts.Listener)
	appSrv.httpReqPrometheusMetrics = httpReqPromMetrics
	return appSrv, nil
}

// NewWithHandler creates a new HTTPServer receiving already created http.Handler.
// Unlike the New constructor, it doesn't add any middlewares.
// Typical use case: create a chi.Router using NewRouter and pass it into NewWithHandler.
// Deprecated: Will be removed in the next major version. Please use New with Handler options instead.
func NewWithHandler(cfg *Config, logger log.FieldLogger, handler http.Handler) *HTTPServer {
	return newWithHandler(cfg, logger, handler, nil)
}

func newWithHandler(cfg *Config, logger log.FieldLogger, handler http.Handler, listener net.Listener) *HTTPServer {
	httpServer := &http.Server{
		Addr:              cfg.Address,
		WriteTimeout:      time.Duration(cfg.Timeouts.Write),
		ReadTimeout:       time.Duration(cfg.Timeouts.Read),
		ReadHeaderTimeout: time.Duration(cfg.Timeouts.ReadHeader),
		IdleTimeout:       time.Duration(cfg.Timeouts.Idle),
		Handler:           handler,
	}

	buildServerURL := func() string {
		serverURL := httpServer.Addr
		if cfg.UnixSocketPath != "" {
			serverURL = "localhost" // Any domain can be used here. It will not be used in unix-socket case.
		}
		if cfg.TLS.Enabled {
			return "https://" + serverURL
		}
		return "http://" + serverURL
	}

	router, _ := handler.(chi.Router)

	return &HTTPServer{
		URL:             buildServerURL(),
		HTTPServer:      httpServer,
		UnixSocketPath:  cfg.UnixSocketPath,
		Logger:          logger,
		TLS:             cfg.TLS,
		ShutdownTimeout: time.Duration(cfg.Timeouts.Shutdown),
		HTTPRouter:      router,
		listener:        listener,
	}
}

// Start starts application HTTP server in a blocking way.
// It's supposed that this method will be called in a separate goroutine.
// If a fatal error occurs, it will be sent to the fatalError channel.
func (s *HTTPServer) Start(fatalError chan<- error) {
	done := make(chan struct{})
	defer close(done)
	s.httpServerDone.Store(done)

	logger := s.Logger.With(
		log.String("address", s.HTTPServer.Addr),
		log.Duration("write_timeout", s.HTTPServer.WriteTimeout),
		log.Duration("read_timeout", s.HTTPServer.ReadTimeout),
		log.Duration("read_header_timeout", s.HTTPServer.ReadHeaderTimeout),
		log.Duration("idle_timeout", s.HTTPServer.IdleTimeout),
		log.Duration("shutdown_timeout", s.ShutdownTimeout),
	)
	if s.UnixSocketPath != "" {
		logger = logger.With(log.String("unix_socket_path", s.UnixSocketPath))
		if err := os.Remove(s.UnixSocketPath); err != nil && !os.IsNotExist(err) {
			fatalError <- fmt.Errorf("remove unix socket file %q: %w", s.UnixSocketPath, err)
			return
		}
	}

	logger.Info("starting application HTTP server...")

	var err error
	if s.listener == nil {
		network, addr := s.NetworkAndAddr()
		if s.listener, err = net.Listen(network, addr); err != nil {
			logger.Error("application HTTP server error", log.Error(err))
			fatalError <- err
			return
		}
	}

	if s.listener.Addr().Network() == networkTCP {
		var portStr string
		_, portStr, err = net.SplitHostPort(s.listener.Addr().String())
		if err != nil {
			logger.Error("unexpected format of TCP listener address: unable to split host and port", log.Error(err))
			fatalError <- err
			return
		}

		var port int64
		port, err = strconv.ParseInt(portStr, 10, 32)
		if err != nil {
			logger.Error("unexpected format of TCP listener address: no numeric port", log.Error(err))
			fatalError <- err
			return
		}
		atomic.StoreInt32(&s.port, int32(port))
	}

	if s.TLS.Enabled {
		err = s.HTTPServer.ServeTLS(s.listener, s.TLS.Certificate, s.TLS.Key)
	} else {
		err = s.HTTPServer.Serve(s.listener)
	}

	if err != nil {
		if errors.Is(err, http.ErrServerClosed) {
			logger.Info("application HTTP server closed")
			return
		}
		logger.Error("application HTTP server error", log.Error(err))
		fatalError <- err
		return
	}
}

// Stop stops application HTTP server (gracefully or not).
func (s *HTTPServer) Stop(gracefully bool) error {
	if !gracefully {
		s.Logger.Info("closing application HTTP server...")
		if err := s.HTTPServer.Close(); err != nil {
			s.Logger.Error("application HTTP server closing error", log.Error(err))
			return err
		}
		if done, ok := s.httpServerDone.Load().(chan struct{}); ok && done != nil {
			<-done // Wait for the listener to be closed.
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.ShutdownTimeout)
	defer cancel()

	s.Logger.Info("shutting down application HTTP server...", log.Duration("timeout", s.ShutdownTimeout))
	if err := s.HTTPServer.Shutdown(ctx); err != nil {
		s.Logger.Error("application HTTP server shutting down error", log.Error(err))
		return err
	}
	s.Logger.Info("application HTTP server shut down")

	if done, ok := s.httpServerDone.Load().(chan struct{}); ok && done != nil {
		<-done // Wait for the listener to be closed.
	}

	return nil
}

// MustRegisterMetrics registers metrics in Prometheus client and panics if any error occurs.
func (s *HTTPServer) MustRegisterMetrics() {
	if s.httpReqPrometheusMetrics != nil {
		s.httpReqPrometheusMetrics.MustRegister()
	}
}

// UnregisterMetrics unregisters metrics in Prometheus client.
func (s *HTTPServer) UnregisterMetrics() {
	if s.httpReqPrometheusMetrics != nil {
		s.httpReqPrometheusMetrics.Unregister()
	}
}

// NetworkAndAddr returns network type ("tcp" or "unix") and address (path to unix socket in case of "unix" network).
func (s *HTTPServer) NetworkAndAddr() (network string, addr string) {
	if s.UnixSocketPath != "" {
		return networkUnix, s.UnixSocketPath
	}
	return networkTCP, s.HTTPServer.Addr
}

func (s *HTTPServer) GetPort() int {
	return int(atomic.LoadInt32(&s.port))
}

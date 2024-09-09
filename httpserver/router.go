/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/restapi"
)

// RouterOpts represents options for creating chi.Router.
type RouterOpts struct {
	ServiceNameInURL   string
	APIRoutes          map[APIVersion]APIRoute
	RootMiddlewares    []func(http.Handler) http.Handler
	ErrorDomain        string
	HealthCheck        HealthCheck
	HealthCheckContext HealthCheckContext
	MetricsHandler     http.Handler
}

// NewRouter creates a new chi.Router and performs its basic configuration.
func NewRouter(logger log.FieldLogger, opts RouterOpts) chi.Router {
	router := chi.NewRouter()
	configureRouter(router, logger, opts)
	return router
}

// nolint // hugeParam: opts is heavy, it's ok in this case.
func configureRouter(router chi.Router, logger log.FieldLogger, opts RouterOpts) {
	router.Use(opts.RootMiddlewares...)

	// Expose endpoint for Prometheus.
	metricsHandler := opts.MetricsHandler
	if opts.MetricsHandler == nil {
		metricsHandler = promhttp.Handler()
	}
	router.Method(http.MethodGet, "/metrics", metricsHandler)

	if opts.HealthCheckContext != nil {
		router.Method(http.MethodGet, "/healthz", NewHealthCheckHandlerContext(opts.HealthCheckContext))
	} else {
		router.Method(http.MethodGet, "/healthz", NewHealthCheckHandler(opts.HealthCheck))
	}

	router.Route(fmt.Sprintf("/api/%s", opts.ServiceNameInURL), func(router chi.Router) {
		for ver, r := range opts.APIRoutes {
			router.Route(fmt.Sprintf("/v%d", ver), r)
		}
	})

	router.NotFound(func(rw http.ResponseWriter, r *http.Request) {
		apiErr := restapi.NewError(opts.ErrorDomain, restapi.ErrCodeNotFound, restapi.ErrMessageNotFound)
		restapi.RespondError(rw, http.StatusNotFound, apiErr, logger)
	})

	router.MethodNotAllowed(func(rw http.ResponseWriter, r *http.Request) {
		apiErr := restapi.NewError(opts.ErrorDomain, restapi.ErrCodeMethodNotAllowed, restapi.ErrMessageMethodNotAllowed)
		restapi.RespondError(rw, http.StatusMethodNotAllowed, apiErr, logger)
	})
}

// nolint // hugeParam: opts is heavy, it's ok in this case.
func applyDefaultMiddlewaresToRouter(
	router chi.Router, cfg *Config, logger log.FieldLogger, opts Opts, promMetrics *middleware.HTTPRequestPrometheusMetrics,
) error {
	router.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(rw, r.WithContext(middleware.NewContextWithRequestStartTime(r.Context(), time.Now())))
		})
	})

	// Request ID middleware.
	router.Use(middleware.RequestID())

	// Logging middleware.
	loggingOpts := middleware.LoggingOpts{
		RequestStart:           cfg.Log.RequestStart,
		RequestHeaders:         make(map[string]string, len(cfg.Log.RequestHeaders)),
		ExcludedEndpoints:      cfg.Log.ExcludedEndpoints,
		SecretQueryParams:      cfg.Log.SecretQueryParams,
		AddRequestInfoToLogger: cfg.Log.AddRequestInfoToLogger,
		SlowRequestThreshold:   cfg.Log.SlowRequestThreshold,
	}
	for _, headerName := range cfg.Log.RequestHeaders {
		logFieldKey := "req_header_" + strings.ToLower(strings.ReplaceAll(headerName, "-", "_"))
		loggingOpts.RequestHeaders[headerName] = logFieldKey
	}
	router.Use(middleware.LoggingWithOpts(logger, loggingOpts))

	// Recovery middleware.
	router.Use(middleware.Recovery(opts.ErrorDomain))

	// Metrics middleware
	getRoutePattern := GetChiRoutePattern
	if opts.HTTPRequestMetrics.GetRoutePattern != nil {
		// Custom route pattern parser
		getRoutePattern = opts.HTTPRequestMetrics.GetRoutePattern
	}
	metricsMiddleware := middleware.HTTPRequestMetricsWithOpts(promMetrics, getRoutePattern,
		middleware.HTTPRequestMetricsOpts{
			GetUserAgentType:  opts.HTTPRequestMetrics.GetUserAgentType,
			ExcludedEndpoints: systemEndpoints,
		})
	router.Use(metricsMiddleware)

	if cfg.Limits.MaxRequests != 0 {
		inFlightLimitMw, err := middleware.InFlightLimit(cfg.Limits.MaxRequests, opts.ErrorDomain)
		if err != nil {
			return fmt.Errorf("create in-flight limit middleware: %w", err)
		}
		router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				for i := 0; i < len(systemEndpoints); i++ {
					if r.URL.Path == systemEndpoints[i] {
						next.ServeHTTP(rw, r)
						return
					}
				}
				inFlightLimitMw(next).ServeHTTP(rw, r)
			})
		})
	}

	// Middleware to limit max request body.
	if cfg.Limits.MaxBodySizeBytes > 0 {
		router.Use(middleware.RequestBodyLimit(cfg.Limits.MaxBodySizeBytes, opts.ErrorDomain))
	}

	return nil
}

// GetChiRoutePattern extracts chi route pattern from request.
func GetChiRoutePattern(r *http.Request) string {
	// modified code from https://github.com/go-chi/chi/issues/270#issuecomment-479184559
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return ""
	}
	if pattern := rctx.RoutePattern(); pattern != "" {
		// Pattern is already available
		return pattern
	}

	routePath := r.URL.RawPath
	if routePath == "" {
		routePath = r.URL.Path
	}

	tctx := chi.NewRouteContext()
	if !rctx.Routes.Match(tctx, r.Method, routePath) {
		return ""
	}
	return tctx.RoutePattern()
}

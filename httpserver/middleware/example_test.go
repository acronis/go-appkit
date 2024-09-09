/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/acronis/go-appkit/log"
)

func Example() {
	const errDomain = "MyService"

	logger, closeFn := log.NewLogger(&log.Config{Output: log.OutputStdout, Format: log.FormatJSON})
	defer closeFn()

	router := chi.NewRouter()

	router.Use(
		RequestID(),
		LoggingWithOpts(logger, LoggingOpts{RequestStart: true}),
		Recovery(errDomain),
		RequestBodyLimit(1024*1024, errDomain),
	)

	metricsCollector := NewHTTPRequestPrometheusMetrics()
	router.Use(HTTPRequestMetricsWithOpts(metricsCollector, getChiRoutePattern, HTTPRequestMetricsOpts{
		ExcludedEndpoints: []string{"/metrics", "/healthz"}, // Metrics will not be collected for "/metrics" and "/healthz" endpoints.
	}))

	userCreateInFlightLimitMiddleware := MustInFlightLimitWithOpts(32, errDomain, InFlightLimitOpts{
		GetKey: func(r *http.Request) (string, bool, error) {
			key := r.Header.Get("X-Client-ID")
			return key, key == "", nil
		},
		MaxKeys:            1000,
		ResponseStatusCode: http.StatusTooManyRequests,
		GetRetryAfter: func(r *http.Request) time.Duration {
			return time.Second * 15
		},
		BacklogLimit:   64,
		BacklogTimeout: time.Second * 10,
	})

	usersListRateLimitMiddleware := MustRateLimit(Rate{Count: 100, Duration: time.Second}, errDomain)

	router.Route("/users", func(r chi.Router) {
		r.With(usersListRateLimitMiddleware).Get("/", func(rw http.ResponseWriter, req *http.Request) {
			// Returns list of users.
		})
		r.With(userCreateInFlightLimitMiddleware).Post("/", func(rw http.ResponseWriter, req *http.Request) {
			// Create new user.
		})
	})
}

// Example of how this function may look for gorilla/mux router is given in the RoutePatternGetterFunc documentation.
// GetChiRoutePattern extracts chi route pattern from request.
func getChiRoutePattern(r *http.Request) string {
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

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/vasayxtx/go-glob"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/restapi"
)

// RuleLogFieldName is a logged field that contains the name of the throttling rule.
const RuleLogFieldName = "throttle_rule"

// MiddlewareOpts represents an options for Middleware.
type MiddlewareOpts struct {
	// GetKeyIdentity is a function that returns identity string representation.
	// The returned string is used as a key for zone when key.type is "identity".
	GetKeyIdentity func(r *http.Request) (key string, bypass bool, err error)

	// RateLimitOnReject is a callback called for rejecting HTTP request when the rate limit is exceeded.
	RateLimitOnReject middleware.RateLimitOnRejectFunc

	// RateLimitOnRejectInDryRun is a callback called for rejecting HTTP request in the dry-run mode
	// when the rate limit is exceeded.
	RateLimitOnRejectInDryRun middleware.RateLimitOnRejectFunc

	// RateLimitOnError is a callback called in case of any error that may occur during the rate limiting.
	RateLimitOnError middleware.RateLimitOnErrorFunc

	// InFlightLimitOnReject is a callback called for rejecting HTTP request when the in-flight limit is exceeded.
	InFlightLimitOnReject middleware.InFlightLimitOnRejectFunc

	// RateLimitOnRejectInDryRun is a callback called for rejecting HTTP request in the dry-run mode
	// when the in-flight limit is exceeded.
	InFlightLimitOnRejectInDryRun middleware.InFlightLimitOnRejectFunc

	// RateLimitOnError is a callback called in case of any error that may occur during the in-flight limiting.
	InFlightLimitOnError middleware.InFlightLimitOnErrorFunc

	// Tags is a list of tags for filtering throttling rules from the config. If it's empty, all rules can be applied.
	Tags []string

	// BuildHandlerAtInit determines where the final handler will be constructed.
	// If true, it will be done at the initialization step (i.e., in the constructor),
	// false (default) - right in the ServeHTTP() method (gorilla/mux case).
	BuildHandlerAtInit bool
}

// rateLimitOpts returns options for constructing rate limiting middleware.
func (opts MiddlewareOpts) rateLimitOpts() rateLimitMiddlewareOpts {
	return rateLimitMiddlewareOpts{
		GetKeyIdentity:            opts.GetKeyIdentity,
		RateLimitOnReject:         opts.RateLimitOnReject,
		RateLimitOnRejectInDryRun: opts.RateLimitOnRejectInDryRun,
		RateLimitOnError:          opts.RateLimitOnError,
	}
}

// inFlightLimitOpts returns options for constructing in-flight limiting middleware.
func (opts MiddlewareOpts) inFlightLimitOpts() inFlightLimitMiddlewareOpts {
	return inFlightLimitMiddlewareOpts{
		GetKeyIdentity:                opts.GetKeyIdentity,
		InFlightLimitOnReject:         opts.InFlightLimitOnReject,
		InFlightLimitOnRejectInDryRun: opts.InFlightLimitOnRejectInDryRun,
		InFlightLimitOnError:          opts.InFlightLimitOnError,
	}
}

// Middleware is a middleware that throttles incoming HTTP requests based on the passed configuration.
func Middleware(cfg *Config, errDomain string, mc MetricsCollector) (func(next http.Handler) http.Handler, error) {
	return MiddlewareWithOpts(cfg, errDomain, mc, MiddlewareOpts{})
}

// MiddlewareWithOpts is a more configurable version of Middleware.
func MiddlewareWithOpts(
	cfg *Config, errDomain string, mc MetricsCollector, opts MiddlewareOpts,
) (func(next http.Handler) http.Handler, error) {
	if mc == nil {
		mc = disabledMetrics{}
	}

	routes, err := makeRoutes(cfg, errDomain, mc, opts)
	if err != nil {
		return nil, err
	}

	if opts.BuildHandlerAtInit {
		return func(next http.Handler) http.Handler {
			for i := range routes {
				route := &routes[i]
				route.Handler = next
				for j := len(route.Middlewares) - 1; j >= 0; j-- {
					route.Handler = route.Middlewares[j](route.Handler)
				}
			}
			return &handler{next: next, routesManager: restapi.NewRoutesManager(routes)}
		}, nil
	}

	return func(next http.Handler) http.Handler {
		return &handler{next: next, routesManager: restapi.NewRoutesManager(routes)}
	}, nil
}

type handler struct {
	next          http.Handler
	routesManager *restapi.RoutesManager
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	matchedRoute, ok := h.routesManager.SearchMatchedRouteForRequest(r)
	if !ok {
		h.next.ServeHTTP(rw, r)
		return
	}

	if matchedRoute.Handler != nil {
		matchedRoute.Handler.ServeHTTP(rw, r)
		return
	}

	// We build a final handler here and not in the constructor because it is how the gorilla/mux works:
	// all middlewares apply only after the matched route is found
	// (https://github.com/gorilla/mux/blob/d07530f46e1eec4e40346e24af34dcc6750ad39f/mux.go#L138-L146).
	nextHandler := h.next
	for i := len(matchedRoute.Middlewares) - 1; i >= 0; i-- {
		nextHandler = matchedRoute.Middlewares[i](nextHandler)
	}
	nextHandler.ServeHTTP(rw, r)
}

// nolint: gocyclo // we would like to have high functional cohesion here.
func makeRoutes(
	cfg *Config, errDomain string, mc MetricsCollector, opts MiddlewareOpts,
) (routes []restapi.Route, err error) {
	for _, rule := range cfg.Rules {
		if len(rule.RateLimits) == 0 && len(rule.InFlightLimits) == 0 {
			continue
		}

		if len(opts.Tags) != 0 && !checkStringSlicesIntersect(opts.Tags, rule.Tags) {
			continue
		}

		var middlewares []func(http.Handler) http.Handler

		// Build in-flight limiting middleware.
		for i := 0; i < len(rule.InFlightLimits); i++ {
			zoneName := rule.InFlightLimits[i].Zone
			cfgZone, ok := cfg.InFlightLimitZones[zoneName]
			if !ok {
				return nil, fmt.Errorf("in-flight zone %q is not defined", zoneName)
			}
			var inFlightLimitMw func(next http.Handler) http.Handler
			inFlightLimitMw, err = makeInFlightLimitMiddleware(
				&cfgZone, errDomain, rule.Name(), mc, opts.inFlightLimitOpts())
			if err != nil {
				return nil, fmt.Errorf("make in-flight limit middleware for zone %q: %w", zoneName, err)
			}
			middlewares = append(middlewares, inFlightLimitMw)
		}

		// Build rate limiting middleware.
		for i := 0; i < len(rule.RateLimits); i++ {
			zoneName := rule.RateLimits[i].Zone
			cfgZone, ok := cfg.RateLimitZones[zoneName]
			if !ok {
				return nil, fmt.Errorf("rate limit zone %q is not defined", zoneName)
			}
			var rateLimitMw func(next http.Handler) http.Handler
			rateLimitMw, err = makeRateLimitMiddleware(
				&cfgZone, errDomain, rule.Name(), mc, opts.rateLimitOpts())
			if err != nil {
				return nil, fmt.Errorf("make rate limit middleware for zone %q: %w", zoneName, err)
			}
			middlewares = append(middlewares, rateLimitMw)
		}

		for _, cfgRoute := range rule.Routes {
			routes = append(routes, restapi.NewRoute(cfgRoute, nil, middlewares))
		}
		for _, exclCfgRoute := range rule.ExcludedRoutes {
			routes = append(routes, restapi.NewExcludedRoute(exclCfgRoute))
		}
	}

	return routes, nil
}

// rateLimitMiddlewareOpts represents an options for RateLimitMiddleware.
type rateLimitMiddlewareOpts struct {
	// GetKeyIdentity is a function that returns identity string representation.
	// The returned string is used as a key for zone when key.type is "identity".
	GetKeyIdentity func(r *http.Request) (key string, bypass bool, err error)

	// RateLimitOnReject is a callback that is called for rejecting HTTP request when the rate limit is exceeded.
	RateLimitOnReject middleware.RateLimitOnRejectFunc

	// RateLimitOnRejectInDryRun is a callback that is called for rejecting HTTP request in the dry-run mode
	// when the rate limit is exceeded.
	RateLimitOnRejectInDryRun middleware.RateLimitOnRejectFunc

	// RateLimitOnError is a callback that is called in case of any error that may occur during the rate limiting.
	RateLimitOnError middleware.RateLimitOnErrorFunc
}

func makeRateLimitMiddleware(
	cfg *RateLimitZoneConfig,
	errDomain string,
	ruleName string,
	mc MetricsCollector,
	opts rateLimitMiddlewareOpts,
) (func(next http.Handler) http.Handler, error) {
	var alg middleware.RateLimitAlg
	switch cfg.Alg {
	case "", RateLimitAlgLeakyBucket:
		alg = middleware.RateLimitAlgLeakyBucket
	case RateLimitAlgSlidingWindow:
		alg = middleware.RateLimitAlgSlidingWindow
	default:
		return nil, fmt.Errorf("unknown rate limit alg %q", cfg.Alg)
	}

	if cfg.Key.Type == ZoneKeyTypeIdentity && opts.GetKeyIdentity == nil {
		return nil, fmt.Errorf("GetKeyIdentity is required for identity key type")
	}

	getKey, err := makeGetKeyFunc(cfg.Key, opts.GetKeyIdentity, cfg.ExcludedKeys, cfg.IncludedKeys)
	if err != nil {
		return nil, err
	}

	var getRetryAfter func(r *http.Request, estimatedTime time.Duration) time.Duration
	switch {
	case cfg.ResponseRetryAfter.IsAuto:
		getRetryAfter = middleware.GetRetryAfterEstimatedTime
	case cfg.ResponseRetryAfter.Duration == 0:
		getRetryAfter = nil
	default:
		getRetryAfter = func(_ *http.Request, _ time.Duration) time.Duration {
			return cfg.ResponseRetryAfter.Duration
		}
	}

	onReject := opts.RateLimitOnReject
	if onReject == nil {
		onReject = middleware.DefaultRateLimitOnReject
	}
	onRejectWithMetrics := func(
		rw http.ResponseWriter, r *http.Request, params middleware.RateLimitParams, next http.Handler, logger log.FieldLogger,
	) {
		mc.IncRateLimitRejects(ruleName, false)
		if logger != nil {
			logger = logger.With(log.String(RuleLogFieldName, ruleName))
		}
		onReject(rw, r, params, next, logger)
	}

	onRejectInDryRun := opts.RateLimitOnRejectInDryRun
	if onRejectInDryRun == nil {
		onRejectInDryRun = middleware.DefaultRateLimitOnRejectInDryRun
	}
	onRejectInDryRunWithMetrics := func(
		rw http.ResponseWriter, r *http.Request, params middleware.RateLimitParams, next http.Handler, logger log.FieldLogger,
	) {
		mc.IncRateLimitRejects(ruleName, true)
		if logger != nil {
			logger = logger.With(log.String(RuleLogFieldName, ruleName))
		}
		onRejectInDryRun(rw, r, params, next, logger)
	}

	rate := middleware.Rate{Count: cfg.RateLimit.Count, Duration: cfg.RateLimit.Duration}
	return middleware.RateLimitWithOpts(rate, errDomain, middleware.RateLimitOpts{
		Alg:                alg,
		MaxBurst:           cfg.BurstLimit,
		GetKey:             getKey,
		MaxKeys:            cfg.MaxKeys,
		BacklogLimit:       cfg.BacklogLimit,
		BacklogTimeout:     time.Duration(cfg.BacklogTimeout),
		ResponseStatusCode: cfg.getResponseStatusCode(),
		GetRetryAfter:      getRetryAfter,
		DryRun:             cfg.DryRun,
		OnReject:           onRejectWithMetrics,
		OnRejectInDryRun:   onRejectInDryRunWithMetrics,
		OnError:            opts.RateLimitOnError,
	})
}

type inFlightLimitMiddlewareOpts struct {
	// GetKeyIdentity is a function that returns identity string representation.
	// The returned string is used as a key for zone when key.type is "identity".
	GetKeyIdentity func(r *http.Request) (key string, bypass bool, err error)

	// InFlightLimitOnReject is a callback that is called for rejecting HTTP request when the in-flight limit is exceeded.
	InFlightLimitOnReject middleware.InFlightLimitOnRejectFunc

	// RateLimitOnRejectInDryRun is a callback that is called for rejecting HTTP request in the dry-run mode
	// when the in-flight limit is exceeded.
	InFlightLimitOnRejectInDryRun middleware.InFlightLimitOnRejectFunc

	// RateLimitOnError is a callback that is called in case of any error that may occur during the in-flight limiting.
	InFlightLimitOnError middleware.InFlightLimitOnErrorFunc
}

func makeInFlightLimitMiddleware(
	cfg *InFlightLimitZoneConfig,
	errDomain string,
	ruleName string,
	mc MetricsCollector,
	opts inFlightLimitMiddlewareOpts,
) (func(next http.Handler) http.Handler, error) {
	if cfg.Key.Type == ZoneKeyTypeIdentity && opts.GetKeyIdentity == nil {
		return nil, fmt.Errorf("GetKeyIdentity is required for identity key type")
	}

	getKey, err := makeGetKeyFunc(cfg.Key, opts.GetKeyIdentity, cfg.ExcludedKeys, cfg.IncludedKeys)
	if err != nil {
		return nil, err
	}

	var getRetryAfter func(r *http.Request) time.Duration
	if cfg.ResponseRetryAfter != 0 {
		getRetryAfter = func(_ *http.Request) time.Duration {
			return time.Duration(cfg.ResponseRetryAfter)
		}
	}

	onReject := opts.InFlightLimitOnReject
	if onReject == nil {
		onReject = middleware.DefaultInFlightLimitOnReject
	}
	onRejectWithMetrics := func(
		rw http.ResponseWriter, r *http.Request, params middleware.InFlightLimitParams, next http.Handler, logger log.FieldLogger,
	) {
		mc.IncInFlightLimitRejects(ruleName, false, params.RequestBacklogged)
		if logger != nil {
			logger = logger.With(log.String(RuleLogFieldName, ruleName))
		}
		onReject(rw, r, params, next, logger)
	}

	onRejectInDryRun := opts.InFlightLimitOnRejectInDryRun
	if onRejectInDryRun == nil {
		onRejectInDryRun = middleware.DefaultInFlightLimitOnRejectInDryRun
	}
	onRejectInDryRunWithMetrics := func(
		rw http.ResponseWriter, r *http.Request, params middleware.InFlightLimitParams, next http.Handler, logger log.FieldLogger,
	) {
		mc.IncInFlightLimitRejects(ruleName, true, params.RequestBacklogged)
		if logger != nil {
			logger = logger.With(log.String(RuleLogFieldName, ruleName))
		}
		onRejectInDryRun(rw, r, params, next, logger)
	}

	return middleware.InFlightLimitWithOpts(cfg.InFlightLimit, errDomain, middleware.InFlightLimitOpts{
		GetKey:             getKey,
		MaxKeys:            cfg.MaxKeys,
		ResponseStatusCode: cfg.getResponseStatusCode(),
		GetRetryAfter:      getRetryAfter,
		BacklogLimit:       cfg.BacklogLimit,
		BacklogTimeout:     time.Duration(cfg.BacklogTimeout),
		DryRun:             cfg.DryRun,
		OnReject:           onRejectWithMetrics,
		OnRejectInDryRun:   onRejectInDryRunWithMetrics,
		OnError:            opts.InFlightLimitOnError,
	})
}

// nolint: gocyclo // we would like to have high functional cohesion here.
func makeGetKeyFunc(
	cfg ZoneKeyConfig,
	getKeyIdentity func(r *http.Request) (string, bool, error),
	excludedKeys []string,
	includedKeys []string,
) (func(r *http.Request) (string, bool, error), error) {
	makeByType := func() (func(r *http.Request) (string, bool, error), error) {
		switch cfg.Type {
		case ZoneKeyTypeIdentity:
			return getKeyIdentity, nil
		case ZoneKeyTypeHTTPHeader:
			return func(r *http.Request) (string, bool, error) {
				headerVal := strings.TrimSpace(r.Header.Get(cfg.HeaderName))
				if cfg.NoBypassEmpty {
					return headerVal, false, nil
				}
				return headerVal, headerVal == "", nil
			}, nil
		case ZoneKeyTypeRemoteAddr:
			return func(r *http.Request) (string, bool, error) {
				host, _, err := net.SplitHostPort(r.RemoteAddr)
				return host, false, err
			}, nil
		case ZoneKeyTypeNoKey:
			return nil, nil
		}
		return nil, fmt.Errorf("unknown key type %q", cfg.Type)
	}

	getKey, err := makeByType()
	if err != nil || getKey == nil {
		return nil, err
	}
	if len(excludedKeys) == 0 && len(includedKeys) == 0 {
		return getKey, nil
	}

	if len(excludedKeys) != 0 && len(includedKeys) != 0 {
		return nil, fmt.Errorf("excluded and included keys cannot be used together")
	}

	makeWithPredefinedKeys := func(keys []string, exclude bool) func(r *http.Request) (string, bool, error) {
		compiledKeys := make([]func(s string) bool, 0, len(keys))
		for _, key := range keys {
			compiledKeys = append(compiledKeys, glob.Compile(key))
		}
		return func(r *http.Request) (string, bool, error) {
			key, bypass, getKeyErr := getKey(r)
			if getKeyErr != nil {
				return key, bypass, getKeyErr
			}
			if bypass {
				return key, bypass, nil
			}
			keyFound := false
			for i := range compiledKeys {
				if compiledKeys[i](key) {
					keyFound = true
					break
				}
			}
			return key, keyFound == exclude, nil
		}
	}

	if len(excludedKeys) != 0 {
		return makeWithPredefinedKeys(excludedKeys, true), nil
	}
	return makeWithPredefinedKeys(includedKeys, false), nil
}

func checkStringSlicesIntersect(slice1, slice2 []string) bool {
	for i := range slice1 {
		for j := range slice2 {
			if slice1[i] == slice2[j] {
				return true
			}
		}
	}
	return false
}

/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"context"
	"fmt"
	"time"

	"github.com/vasayxtx/go-glob"
	"google.golang.org/grpc"

	"github.com/acronis/go-appkit/grpcserver/interceptor"
)

type UnaryGetKeyFunc func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error)

type unaryInterceptorOptions struct {
	getKeyIdentity                UnaryGetKeyFunc
	rateLimitOnReject             interceptor.RateLimitUnaryOnRejectFunc
	rateLimitOnRejectInDryRun     interceptor.RateLimitUnaryOnRejectFunc
	rateLimitOnError              interceptor.RateLimitUnaryOnErrorFunc
	inFlightLimitOnReject         interceptor.InFlightLimitUnaryOnRejectFunc
	inFlightLimitOnRejectInDryRun interceptor.InFlightLimitUnaryOnRejectFunc
	inFlightLimitOnError          interceptor.InFlightLimitUnaryOnErrorFunc
	tags                          []string
	metricsCollector              MetricsCollector
}

type UnaryInterceptorOption func(*unaryInterceptorOptions)

// WithUnaryGetKeyIdentity sets the function to extract identity string from the request context for unary interceptors.
func WithUnaryGetKeyIdentity(getKeyIdentity UnaryGetKeyFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.getKeyIdentity = getKeyIdentity
	}
}

// WithUnaryRateLimitOnReject sets the callback for handling rejected unary requests when rate limit is exceeded.
func WithUnaryRateLimitOnReject(onReject interceptor.RateLimitUnaryOnRejectFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.rateLimitOnReject = onReject
	}
}

// WithUnaryRateLimitOnRejectInDryRun sets the callback for handling rejected unary requests in dry run mode when rate limit is exceeded.
func WithUnaryRateLimitOnRejectInDryRun(onReject interceptor.RateLimitUnaryOnRejectFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.rateLimitOnRejectInDryRun = onReject
	}
}

// WithUnaryRateLimitOnError sets the callback for handling rate limiting errors in unary requests.
func WithUnaryRateLimitOnError(onError interceptor.RateLimitUnaryOnErrorFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.rateLimitOnError = onError
	}
}

// WithUnaryInFlightLimitOnReject sets the callback for handling rejected unary requests when in-flight limit is exceeded.
func WithUnaryInFlightLimitOnReject(onReject interceptor.InFlightLimitUnaryOnRejectFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.inFlightLimitOnReject = onReject
	}
}

// WithUnaryInFlightLimitOnRejectInDryRun sets the callback for handling rejected unary requests in dry run mode
// when in-flight limit is exceeded.
func WithUnaryInFlightLimitOnRejectInDryRun(onReject interceptor.InFlightLimitUnaryOnRejectFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.inFlightLimitOnRejectInDryRun = onReject
	}
}

// WithUnaryInFlightLimitOnError sets the callback for handling in-flight limiting errors in unary requests.
func WithUnaryInFlightLimitOnError(onError interceptor.InFlightLimitUnaryOnErrorFunc) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.inFlightLimitOnError = onError
	}
}

// WithUnaryTags sets the list of tags for filtering throttling rules from the config for unary interceptors.
func WithUnaryTags(tags []string) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.tags = tags
	}
}

// WithUnaryMetricsCollector sets the metrics collector for collecting throttling metrics.
func WithUnaryMetricsCollector(collector MetricsCollector) UnaryInterceptorOption {
	return func(opts *unaryInterceptorOptions) {
		opts.metricsCollector = collector
	}
}

// UnaryInterceptor returns a gRPC unary interceptor that throttles requests based on the provided configuration.
func UnaryInterceptor(cfg *Config, options ...UnaryInterceptorOption) (grpc.UnaryServerInterceptor, error) {
	opts := &unaryInterceptorOptions{metricsCollector: disabledMetrics{}}
	for _, option := range options {
		option(opts)
	}

	unaryRoutes, err := makeUnaryRoutes(cfg, opts)
	if err != nil {
		return nil, err
	}
	if len(unaryRoutes) == 0 {
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			return handler(ctx, req)
		}, nil
	}

	routesManager := newServiceRouteManager(unaryRoutes)

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		matchedRoute, ok := routesManager.SearchMatchedRouteForRequest(info.FullMethod)
		if !ok {
			return handler(ctx, req)
		}
		return matchedRoute.interceptor(ctx, req, info, handler)
	}, nil
}

func makeUnaryRoutes(cfg *Config, opts *unaryInterceptorOptions) ([]serviceRoute[grpc.UnaryServerInterceptor], error) {
	constructor := func(cfg *Config, rule *RuleConfig) ([]grpc.UnaryServerInterceptor, error) {
		return makeUnaryInterceptorsForRule(cfg, rule, opts)
	}
	return makeServiceRoutes[grpc.UnaryServerInterceptor](cfg, opts.tags, constructor, chainUnaryInterceptors)
}

func chainUnaryInterceptors(interceptors []grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return interceptors[0](ctx, req, info, getChainUnaryHandler(interceptors, 0, info, handler))
	}
}

func getChainUnaryHandler(
	interceptors []grpc.UnaryServerInterceptor, curr int, info *grpc.UnaryServerInfo, finalHandler grpc.UnaryHandler,
) grpc.UnaryHandler {
	if curr == len(interceptors)-1 {
		return finalHandler
	}
	return func(ctx context.Context, req any) (any, error) {
		return interceptors[curr+1](ctx, req, info, getChainUnaryHandler(interceptors, curr+1, info, finalHandler))
	}
}

// nolint:dupl // false positive
func makeUnaryInterceptorsForRule(cfg *Config, rule *RuleConfig, opts *unaryInterceptorOptions) ([]grpc.UnaryServerInterceptor, error) {
	var interceptors []grpc.UnaryServerInterceptor

	for _, inFlightLimit := range rule.InFlightLimits {
		iflInterceptor, err := makeInFlightUnaryInterceptor(cfg, inFlightLimit, rule, opts)
		if err != nil {
			return nil, fmt.Errorf("create in-flight limit unary interceptor for zone %q: %w", inFlightLimit.Zone, err)
		}
		interceptors = append(interceptors, iflInterceptor)
	}

	for _, rateLimit := range rule.RateLimits {
		rlInterceptor, err := makeRateLimitUnaryInterceptor(cfg, rateLimit, rule, opts)
		if err != nil {
			return nil, fmt.Errorf("create rate limit unary interceptor for zone %q: %w", rateLimit.Zone, err)
		}
		interceptors = append(interceptors, rlInterceptor)
	}

	return interceptors, nil
}

func makeInFlightUnaryInterceptor(
	cfg *Config, inFlightLimit RuleInFlightLimit, rule *RuleConfig, opts *unaryInterceptorOptions,
) (grpc.UnaryServerInterceptor, error) {
	zoneCfg, ok := cfg.InFlightLimitZones[inFlightLimit.Zone]
	if !ok {
		return nil, fmt.Errorf("in-flight limit zone %q is not defined", inFlightLimit.Zone)
	}

	getKey, err := makeUnaryGetKeyFunc(zoneCfg.ZoneConfig, opts.getKeyIdentity)
	if err != nil {
		return nil, fmt.Errorf("make get key func: %w", err)
	}

	interceptorOpts := []interceptor.InFlightLimitOption{
		interceptor.WithInFlightLimitUnaryGetKey(interceptor.InFlightLimitUnaryGetKeyFunc(getKey)),
		interceptor.WithInFlightLimitDryRun(zoneCfg.DryRun),
	}
	if zoneCfg.MaxKeys > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitMaxKeys(zoneCfg.MaxKeys))
	}
	if zoneCfg.BacklogLimit > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitBacklogLimit(zoneCfg.BacklogLimit))
	}
	if zoneCfg.BacklogTimeout > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitBacklogTimeout(time.Duration(zoneCfg.BacklogTimeout)))
	}
	if zoneCfg.ResponseRetryAfter > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitUnaryGetRetryAfter(
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) time.Duration {
				return time.Duration(zoneCfg.ResponseRetryAfter)
			},
		))
	}
	if opts.inFlightLimitOnError != nil {
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitUnaryOnError(opts.inFlightLimitOnError))
	}
	onReject := opts.inFlightLimitOnReject
	if onReject == nil {
		onReject = interceptor.DefaultInFlightLimitUnaryOnReject
	}
	wrappedOnReject := func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler, params interceptor.InFlightLimitParams,
	) (interface{}, error) {
		opts.metricsCollector.IncInFlightLimitRejects(rule.Name(), false, params.RequestBacklogged)
		return onReject(ctx, req, info, handler, params)
	}
	onRejectInDryRun := opts.inFlightLimitOnRejectInDryRun
	if onRejectInDryRun == nil {
		onRejectInDryRun = interceptor.DefaultInFlightLimitUnaryOnRejectInDryRun
	}
	wrappedOnRejectInDryRun := func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler, params interceptor.InFlightLimitParams,
	) (interface{}, error) {
		opts.metricsCollector.IncInFlightLimitRejects(rule.Name(), true, params.RequestBacklogged)
		return onRejectInDryRun(ctx, req, info, handler, params)
	}
	interceptorOpts = append(interceptorOpts,
		interceptor.WithInFlightLimitUnaryOnReject(wrappedOnReject),
		interceptor.WithInFlightLimitUnaryOnRejectInDryRun(wrappedOnRejectInDryRun))

	return interceptor.InFlightLimitUnaryInterceptor(zoneCfg.InFlightLimit, interceptorOpts...)
}

func makeRateLimitUnaryInterceptor(
	cfg *Config, rateLimit RuleRateLimit, rule *RuleConfig, opts *unaryInterceptorOptions,
) (grpc.UnaryServerInterceptor, error) {
	zoneCfg, ok := cfg.RateLimitZones[rateLimit.Zone]
	if !ok {
		return nil, fmt.Errorf("rate limit zone %q is not defined", rateLimit.Zone)
	}

	getKey, err := makeUnaryGetKeyFunc(zoneCfg.ZoneConfig, opts.getKeyIdentity)
	if err != nil {
		return nil, fmt.Errorf("make get key func: %w", err)
	}

	var alg interceptor.RateLimitAlg
	switch zoneCfg.Alg {
	case "", RateLimitAlgLeakyBucket:
		alg = interceptor.RateLimitAlgLeakyBucket
	case RateLimitAlgSlidingWindow:
		alg = interceptor.RateLimitAlgSlidingWindow
	default:
		return nil, fmt.Errorf("unknown rate limit alg %q", zoneCfg.Alg)
	}

	interceptorOpts := []interceptor.RateLimitOption{
		interceptor.WithRateLimitAlg(alg),
		interceptor.WithRateLimitUnaryGetKey(interceptor.RateLimitUnaryGetKeyFunc(getKey)),
		interceptor.WithRateLimitDryRun(zoneCfg.DryRun),
	}
	if zoneCfg.BurstLimit > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitMaxBurst(zoneCfg.BurstLimit))
	}
	if zoneCfg.MaxKeys > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitMaxKeys(zoneCfg.MaxKeys))
	}
	if zoneCfg.BacklogLimit > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitBacklogLimit(zoneCfg.BacklogLimit))
	}
	if zoneCfg.BacklogTimeout > 0 {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitBacklogTimeout(time.Duration(zoneCfg.BacklogTimeout)))
	}
	if zoneCfg.ResponseRetryAfter.Duration > 0 || zoneCfg.ResponseRetryAfter.IsAuto {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitUnaryGetRetryAfter(
			func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, estimatedTime time.Duration) time.Duration {
				if zoneCfg.ResponseRetryAfter.IsAuto {
					return estimatedTime
				}
				return zoneCfg.ResponseRetryAfter.Duration
			},
		))
	}
	if opts.rateLimitOnError != nil {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitUnaryOnError(opts.rateLimitOnError))
	}
	onReject := opts.rateLimitOnReject
	if onReject == nil {
		onReject = interceptor.DefaultRateLimitUnaryOnReject
	}
	wrappedOnReject := func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler, params interceptor.RateLimitParams,
	) (interface{}, error) {
		opts.metricsCollector.IncRateLimitRejects(rule.Name(), false)
		return onReject(ctx, req, info, handler, params)
	}
	onRejectInDryRun := opts.rateLimitOnRejectInDryRun
	if onRejectInDryRun == nil {
		onRejectInDryRun = interceptor.DefaultRateLimitUnaryOnRejectInDryRun
	}
	wrappedOnRejectInDryRun := func(
		ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler, params interceptor.RateLimitParams,
	) (interface{}, error) {
		opts.metricsCollector.IncRateLimitRejects(rule.Name(), true)
		return onRejectInDryRun(ctx, req, info, handler, params)
	}
	interceptorOpts = append(interceptorOpts,
		interceptor.WithRateLimitUnaryOnReject(wrappedOnReject),
		interceptor.WithRateLimitUnaryOnRejectInDryRun(wrappedOnRejectInDryRun))

	rate := interceptor.Rate{Count: zoneCfg.RateLimit.Count, Duration: zoneCfg.RateLimit.Duration}

	return interceptor.RateLimitUnaryInterceptor(rate, interceptorOpts...)
}

func makeUnaryGetKeyFunc(
	cfg ZoneConfig,
	getKeyIdentity UnaryGetKeyFunc,
) (UnaryGetKeyFunc, error) {
	var getKey UnaryGetKeyFunc
	switch cfg.Key.Type {
	case ZoneKeyTypeIdentity:
		if getKeyIdentity == nil {
			return nil, fmt.Errorf("GetKeyIdentity is required for identity key type")
		}
		getKey = getKeyIdentity

	case ZoneKeyTypeHeader:
		if cfg.Key.HeaderName == "" {
			return nil, fmt.Errorf("HeaderName is required for header key type")
		}
		getKey = func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
			return getKeyFromHeader(ctx, cfg.Key.HeaderName, cfg.Key.NoBypassEmpty)
		}

	case ZoneKeyTypeRemoteAddr:
		getKey = func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
			return getKeyFromRemoteAddr(ctx)
		}

	case ZoneKeyTypeNoKey:
		return nil, nil

	default:
		return nil, fmt.Errorf("unknown key type %q", cfg.Key.Type)
	}

	if len(cfg.ExcludedKeys) == 0 && len(cfg.IncludedKeys) == 0 {
		return getKey, nil
	}

	if len(cfg.ExcludedKeys) != 0 && len(cfg.IncludedKeys) != 0 {
		return nil, fmt.Errorf("excluded and included keys cannot be used together")
	}

	return makeUnaryKeyFilter(getKey, cfg.ExcludedKeys, cfg.IncludedKeys), nil
}

func makeUnaryKeyFilter(getKey UnaryGetKeyFunc, excludedKeys, includedKeys []string) UnaryGetKeyFunc {
	keys, exclude := excludedKeys, true
	if len(includedKeys) != 0 {
		keys, exclude = includedKeys, false
	}

	compiledKeys := make([]func(s string) bool, 0, len(keys))
	for _, key := range keys {
		compiledKeys = append(compiledKeys, glob.Compile(key))
	}

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (key string, bypass bool, err error) {
		key, bypass, getKeyErr := getKey(ctx, req, info)
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

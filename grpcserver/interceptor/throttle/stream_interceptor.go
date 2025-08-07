/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"fmt"
	"time"

	"github.com/vasayxtx/go-glob"
	"google.golang.org/grpc"

	"github.com/acronis/go-appkit/grpcserver/interceptor"
)

type StreamGetKeyFunc func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error)

type streamInterceptorOptions struct {
	getKeyIdentity                StreamGetKeyFunc
	rateLimitOnReject             interceptor.RateLimitStreamOnRejectFunc
	rateLimitOnRejectInDryRun     interceptor.RateLimitStreamOnRejectFunc
	rateLimitOnError              interceptor.RateLimitStreamOnErrorFunc
	inFlightLimitOnReject         interceptor.InFlightLimitStreamOnRejectFunc
	inFlightLimitOnRejectInDryRun interceptor.InFlightLimitStreamOnRejectFunc
	inFlightLimitOnError          interceptor.InFlightLimitStreamOnErrorFunc
	tags                          []string
	metricsCollector              MetricsCollector
}

type StreamInterceptorOption func(*streamInterceptorOptions)

// WithStreamGetKeyIdentity sets the function to extract identity string from the request context for stream interceptors.
func WithStreamGetKeyIdentity(getKeyIdentity StreamGetKeyFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.getKeyIdentity = getKeyIdentity
	}
}

// WithStreamRateLimitOnReject sets the callback for handling rejected stream requests when rate limit is exceeded.
func WithStreamRateLimitOnReject(onReject interceptor.RateLimitStreamOnRejectFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.rateLimitOnReject = onReject
	}
}

// WithStreamRateLimitOnRejectInDryRun sets the callback for handling rejected stream requests in dry run mode when rate limit is exceeded.
func WithStreamRateLimitOnRejectInDryRun(onReject interceptor.RateLimitStreamOnRejectFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.rateLimitOnRejectInDryRun = onReject
	}
}

// WithStreamRateLimitOnError sets the callback for handling rate limiting errors in stream requests.
func WithStreamRateLimitOnError(onError interceptor.RateLimitStreamOnErrorFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.rateLimitOnError = onError
	}
}

// WithStreamInFlightLimitOnReject sets the callback for handling rejected stream requests when in-flight limit is exceeded.
func WithStreamInFlightLimitOnReject(onReject interceptor.InFlightLimitStreamOnRejectFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.inFlightLimitOnReject = onReject
	}
}

// WithStreamInFlightLimitOnRejectInDryRun sets the callback for handling rejected stream requests in dry run mode
// when in-flight limit is exceeded.
func WithStreamInFlightLimitOnRejectInDryRun(onReject interceptor.InFlightLimitStreamOnRejectFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.inFlightLimitOnRejectInDryRun = onReject
	}
}

// WithStreamInFlightLimitOnError sets the callback for handling in-flight limiting errors in stream requests.
func WithStreamInFlightLimitOnError(onError interceptor.InFlightLimitStreamOnErrorFunc) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.inFlightLimitOnError = onError
	}
}

// WithStreamTags sets the list of tags for filtering throttling rules from the config for stream interceptors.
func WithStreamTags(tags []string) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.tags = tags
	}
}

// WithStreamMetricsCollector sets the metrics collector for collecting throttling metrics.
func WithStreamMetricsCollector(collector MetricsCollector) StreamInterceptorOption {
	return func(opts *streamInterceptorOptions) {
		opts.metricsCollector = collector
	}
}

// StreamInterceptor returns a gRPC stream interceptor that throttles requests based on the provided configuration.
func StreamInterceptor(cfg *Config, options ...StreamInterceptorOption) (grpc.StreamServerInterceptor, error) {
	opts := &streamInterceptorOptions{metricsCollector: disabledMetrics{}}
	for _, option := range options {
		option(opts)
	}

	streamRoutes, err := makeStreamRoutes(cfg, opts)
	if err != nil {
		return nil, err
	}
	if len(streamRoutes) == 0 {
		return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}, nil
	}

	routesManager := newServiceRouteManager(streamRoutes)

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		matchedRoute, ok := routesManager.SearchMatchedRouteForRequest(info.FullMethod)
		if !ok {
			return handler(srv, ss)
		}
		return matchedRoute.interceptor(srv, ss, info, handler)
	}, nil
}

func makeStreamRoutes(cfg *Config, opts *streamInterceptorOptions) ([]serviceRoute[grpc.StreamServerInterceptor], error) {
	constructor := func(cfg *Config, rule *RuleConfig) ([]grpc.StreamServerInterceptor, error) {
		return makeStreamInterceptorsForRule(cfg, rule, opts)
	}
	return makeServiceRoutes[grpc.StreamServerInterceptor](cfg, opts.tags, constructor, chainStreamInterceptors)
}

func chainStreamInterceptors(interceptors []grpc.StreamServerInterceptor) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return interceptors[0](srv, ss, info, getChainStreamHandler(interceptors, 0, info, handler))
	}
}

func getChainStreamHandler(
	interceptors []grpc.StreamServerInterceptor, curr int, info *grpc.StreamServerInfo, finalHandler grpc.StreamHandler,
) grpc.StreamHandler {
	if curr == len(interceptors)-1 {
		return finalHandler
	}
	return func(srv any, stream grpc.ServerStream) error {
		return interceptors[curr+1](srv, stream, info, getChainStreamHandler(interceptors, curr+1, info, finalHandler))
	}
}

// nolint:dupl // false positive
func makeStreamInterceptorsForRule(cfg *Config, rule *RuleConfig, opts *streamInterceptorOptions) ([]grpc.StreamServerInterceptor, error) {
	var interceptors []grpc.StreamServerInterceptor

	for _, inFlightLimit := range rule.InFlightLimits {
		iflInterceptor, err := makeInFlightStreamInterceptor(cfg, inFlightLimit, rule, opts)
		if err != nil {
			return nil, fmt.Errorf("create in-flight limit stream interceptor for zone %q: %w", inFlightLimit.Zone, err)
		}
		interceptors = append(interceptors, iflInterceptor)
	}

	for _, rateLimit := range rule.RateLimits {
		rlInterceptor, err := makeRateLimitStreamInterceptor(cfg, rateLimit, rule, opts)
		if err != nil {
			return nil, fmt.Errorf("create rate limit stream interceptor for zone %q: %w", rateLimit.Zone, err)
		}
		interceptors = append(interceptors, rlInterceptor)
	}

	return interceptors, nil
}

func makeInFlightStreamInterceptor(
	cfg *Config, inFlightLimit RuleInFlightLimit, rule *RuleConfig, opts *streamInterceptorOptions,
) (grpc.StreamServerInterceptor, error) {
	zoneCfg, ok := cfg.InFlightLimitZones[inFlightLimit.Zone]
	if !ok {
		return nil, fmt.Errorf("in-flight limit zone %q is not defined", inFlightLimit.Zone)
	}

	getKey, err := makeStreamGetKeyFunc(zoneCfg.ZoneConfig, opts.getKeyIdentity)
	if err != nil {
		return nil, fmt.Errorf("make get key func: %w", err)
	}

	interceptorOpts := []interceptor.InFlightLimitOption{
		interceptor.WithInFlightLimitStreamGetKey(interceptor.InFlightLimitStreamGetKeyFunc(getKey)),
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
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitStreamGetRetryAfter(
			func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) time.Duration {
				return time.Duration(zoneCfg.ResponseRetryAfter)
			},
		))
	}
	if opts.inFlightLimitOnError != nil {
		interceptorOpts = append(interceptorOpts, interceptor.WithInFlightLimitStreamOnError(opts.inFlightLimitOnError))
	}
	onReject := opts.inFlightLimitOnReject
	if onReject == nil {
		onReject = interceptor.DefaultInFlightLimitStreamOnReject
	}
	wrappedOnReject := func(
		srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler, params interceptor.InFlightLimitParams,
	) error {
		opts.metricsCollector.IncInFlightLimitRejects(rule.Name(), false, params.RequestBacklogged)
		return onReject(srv, ss, info, handler, params)
	}
	onRejectInDryRun := opts.inFlightLimitOnRejectInDryRun
	if onRejectInDryRun == nil {
		onRejectInDryRun = interceptor.DefaultInFlightLimitStreamOnRejectInDryRun
	}
	wrappedOnRejectInDryRun := func(
		srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler, params interceptor.InFlightLimitParams,
	) error {
		opts.metricsCollector.IncInFlightLimitRejects(rule.Name(), true, params.RequestBacklogged)
		return onRejectInDryRun(srv, ss, info, handler, params)
	}
	interceptorOpts = append(interceptorOpts,
		interceptor.WithInFlightLimitStreamOnReject(wrappedOnReject),
		interceptor.WithInFlightLimitStreamOnRejectInDryRun(wrappedOnRejectInDryRun))

	return interceptor.InFlightLimitStreamInterceptor(zoneCfg.InFlightLimit, interceptorOpts...)
}

func makeRateLimitStreamInterceptor(
	cfg *Config, rateLimit RuleRateLimit, rule *RuleConfig, opts *streamInterceptorOptions,
) (grpc.StreamServerInterceptor, error) {
	zoneCfg, ok := cfg.RateLimitZones[rateLimit.Zone]
	if !ok {
		return nil, fmt.Errorf("rate limit zone %q is not defined", rateLimit.Zone)
	}

	getKey, err := makeStreamGetKeyFunc(zoneCfg.ZoneConfig, opts.getKeyIdentity)
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
		interceptor.WithRateLimitStreamGetKey(interceptor.RateLimitStreamGetKeyFunc(getKey)),
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
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitStreamGetRetryAfter(
			func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, estimatedTime time.Duration) time.Duration {
				if zoneCfg.ResponseRetryAfter.IsAuto {
					return estimatedTime
				}
				return zoneCfg.ResponseRetryAfter.Duration
			},
		))
	}
	if opts.rateLimitOnError != nil {
		interceptorOpts = append(interceptorOpts, interceptor.WithRateLimitStreamOnError(opts.rateLimitOnError))
	}
	onReject := opts.rateLimitOnReject
	if onReject == nil {
		onReject = interceptor.DefaultRateLimitStreamOnReject
	}
	wrappedOnReject := func(
		srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler, params interceptor.RateLimitParams,
	) error {
		opts.metricsCollector.IncRateLimitRejects(rule.Name(), false)
		return onReject(srv, ss, info, handler, params)
	}
	onRejectInDryRun := opts.rateLimitOnRejectInDryRun
	if onRejectInDryRun == nil {
		onRejectInDryRun = interceptor.DefaultRateLimitStreamOnRejectInDryRun
	}
	wrappedOnRejectInDryRun := func(
		srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo,
		handler grpc.StreamHandler, params interceptor.RateLimitParams,
	) error {
		opts.metricsCollector.IncRateLimitRejects(rule.Name(), true)
		return onRejectInDryRun(srv, ss, info, handler, params)
	}
	interceptorOpts = append(interceptorOpts,
		interceptor.WithRateLimitStreamOnReject(wrappedOnReject),
		interceptor.WithRateLimitStreamOnRejectInDryRun(wrappedOnRejectInDryRun))

	rate := interceptor.Rate{Count: zoneCfg.RateLimit.Count, Duration: zoneCfg.RateLimit.Duration}
	return interceptor.RateLimitStreamInterceptor(rate, interceptorOpts...)
}

func makeStreamGetKeyFunc(
	cfg ZoneConfig,
	getKeyIdentity StreamGetKeyFunc,
) (StreamGetKeyFunc, error) {
	var getKey StreamGetKeyFunc
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
		getKey = func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
			return getKeyFromHeader(ss.Context(), cfg.Key.HeaderName, cfg.Key.NoBypassEmpty)
		}

	case ZoneKeyTypeRemoteAddr:
		getKey = func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
			return getKeyFromRemoteAddr(ss.Context())
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

	return makeStreamKeyFilter(getKey, cfg.ExcludedKeys, cfg.IncludedKeys), nil
}

func makeStreamKeyFilter(getKey StreamGetKeyFunc, excludedKeys, includedKeys []string) StreamGetKeyFunc {
	keys, exclude := excludedKeys, true
	if len(includedKeys) != 0 {
		keys, exclude = includedKeys, false
	}

	compiledKeys := make([]func(s string) bool, 0, len(keys))
	for _, key := range keys {
		compiledKeys = append(compiledKeys, glob.Compile(key))
	}

	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (key string, bypass bool, err error) {
		key, bypass, getKeyErr := getKey(srv, ss, info)
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

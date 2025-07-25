/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/acronis/go-appkit/internal/ratelimit"
	"github.com/acronis/go-appkit/log"
)

// DefaultRateLimitMaxKeys is a default value of maximum keys number for the RateLimit interceptor.
const DefaultRateLimitMaxKeys = 10000

// DefaultRateLimitBacklogTimeout determines how long the gRPC request may be in the backlog status.
const DefaultRateLimitBacklogTimeout = ratelimit.DefaultRateLimitBacklogTimeout

// RateLimitLogFieldKey it is the name of the logged field that contains a key for the requests rate limiter.
const RateLimitLogFieldKey = "rate_limit_key"

// RateLimitAlg represents a type for specifying rate-limiting algorithm.
type RateLimitAlg int

// Supported rate-limiting algorithms.
const (
	RateLimitAlgLeakyBucket RateLimitAlg = iota
	RateLimitAlgSlidingWindow
)

// RateLimitParams contains data that relates to the rate limiting procedure
// and could be used for rejecting or handling an occurred error.
type RateLimitParams struct {
	Key                 string
	RequestBacklogged   bool
	EstimatedRetryAfter time.Duration
	UnaryGetRetryAfter  RateLimitUnaryGetRetryAfterFunc
	StreamGetRetryAfter RateLimitStreamGetRetryAfterFunc
}

// RateLimitUnaryGetKeyFunc is a function that is called for getting key for rate limiting in unary requests.
type RateLimitUnaryGetKeyFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo) (key string, bypass bool, err error)

// RateLimitStreamGetKeyFunc is a function that is called for getting key for rate limiting in stream requests.
type RateLimitStreamGetKeyFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo) (key string, bypass bool, err error)

// RateLimitUnaryOnRejectFunc is a function that is called for rejecting gRPC unary request when the rate limit is exceeded.
type RateLimitUnaryOnRejectFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams) (interface{}, error)

// RateLimitStreamOnRejectFunc is a function that is called for rejecting gRPC stream request when the rate limit is exceeded.
type RateLimitStreamOnRejectFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams) error

// RateLimitUnaryOnErrorFunc is a function that is called when an error occurs during rate limiting in unary requests.
type RateLimitUnaryOnErrorFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams, err error) (interface{}, error)

// RateLimitStreamOnErrorFunc is a function that is called when an error occurs during rate limiting in stream requests.
type RateLimitStreamOnErrorFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams, err error) error

// RateLimitUnaryGetRetryAfterFunc is a function that is called to get a value for retry-after header
// when the rate limit is exceeded in unary requests.
type RateLimitUnaryGetRetryAfterFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, estimatedTime time.Duration) time.Duration

// RateLimitStreamGetRetryAfterFunc is a function that is called to get a value for retry-after header
// when the rate limit is exceeded in stream requests.
type RateLimitStreamGetRetryAfterFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, estimatedTime time.Duration) time.Duration

// Rate describes the frequency of requests.
type Rate = ratelimit.Rate

// RateLimitOption represents a configuration option for the rate limit interceptor.
type RateLimitOption func(*rateLimitOptions)

type rateLimitOptions struct {
	alg                    RateLimitAlg
	maxBurst               int
	unaryGetKey            RateLimitUnaryGetKeyFunc
	streamGetKey           RateLimitStreamGetKeyFunc
	maxKeys                int
	dryRun                 bool
	backlogLimit           int
	backlogTimeout         time.Duration
	unaryOnReject          RateLimitUnaryOnRejectFunc
	streamOnReject         RateLimitStreamOnRejectFunc
	unaryOnRejectInDryRun  RateLimitUnaryOnRejectFunc
	streamOnRejectInDryRun RateLimitStreamOnRejectFunc
	unaryOnError           RateLimitUnaryOnErrorFunc
	streamOnError          RateLimitStreamOnErrorFunc
	unaryGetRetryAfter     RateLimitUnaryGetRetryAfterFunc
	streamGetRetryAfter    RateLimitStreamGetRetryAfterFunc
}

// WithRateLimitAlg sets the rate limiting algorithm.
func WithRateLimitAlg(alg RateLimitAlg) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.alg = alg
	}
}

// WithRateLimitMaxBurst sets the maximum burst size for leaky bucket algorithm.
func WithRateLimitMaxBurst(maxBurst int) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.maxBurst = maxBurst
	}
}

// WithRateLimitUnaryGetKey sets the function to extract rate limiting key from unary gRPC requests.
func WithRateLimitUnaryGetKey(getKey RateLimitUnaryGetKeyFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.unaryGetKey = getKey
	}
}

// WithRateLimitStreamGetKey sets the function to extract rate limiting key from stream gRPC requests.
func WithRateLimitStreamGetKey(getKey RateLimitStreamGetKeyFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.streamGetKey = getKey
	}
}

// WithRateLimitMaxKeys sets the maximum number of keys to track.
func WithRateLimitMaxKeys(maxKeys int) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.maxKeys = maxKeys
	}
}

// WithRateLimitDryRun enables dry run mode where limits are checked but not enforced.
func WithRateLimitDryRun(dryRun bool) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.dryRun = dryRun
	}
}

// WithRateLimitBacklogLimit sets the backlog limit for queuing requests.
func WithRateLimitBacklogLimit(backlogLimit int) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.backlogLimit = backlogLimit
	}
}

// WithRateLimitBacklogTimeout sets the timeout for backlogged requests.
func WithRateLimitBacklogTimeout(backlogTimeout time.Duration) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.backlogTimeout = backlogTimeout
	}
}

// WithRateLimitUnaryOnReject sets the callback for handling rejected unary requests.
func WithRateLimitUnaryOnReject(onReject RateLimitUnaryOnRejectFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.unaryOnReject = onReject
	}
}

// WithRateLimitStreamOnReject sets the callback for handling rejected stream requests.
func WithRateLimitStreamOnReject(onReject RateLimitStreamOnRejectFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.streamOnReject = onReject
	}
}

// WithRateLimitUnaryOnRejectInDryRun sets the callback for handling rejected unary requests in dry run mode.
func WithRateLimitUnaryOnRejectInDryRun(onReject RateLimitUnaryOnRejectFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.unaryOnRejectInDryRun = onReject
	}
}

// WithRateLimitStreamOnRejectInDryRun sets the callback for handling rejected stream requests in dry run mode.
func WithRateLimitStreamOnRejectInDryRun(onReject RateLimitStreamOnRejectFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.streamOnRejectInDryRun = onReject
	}
}

// WithRateLimitUnaryOnError sets the callback for handling rate limiting errors in unary requests.
func WithRateLimitUnaryOnError(onError RateLimitUnaryOnErrorFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.unaryOnError = onError
	}
}

// WithRateLimitStreamOnError sets the callback for handling rate limiting errors in stream requests.
func WithRateLimitStreamOnError(onError RateLimitStreamOnErrorFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.streamOnError = onError
	}
}

// WithRateLimitUnaryGetRetryAfter sets the function to calculate retry-after value for unary requests.
func WithRateLimitUnaryGetRetryAfter(getRetryAfter RateLimitUnaryGetRetryAfterFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.unaryGetRetryAfter = getRetryAfter
	}
}

// WithRateLimitStreamGetRetryAfter sets the function to calculate retry-after value for stream requests.
func WithRateLimitStreamGetRetryAfter(getRetryAfter RateLimitStreamGetRetryAfterFunc) RateLimitOption {
	return func(opts *rateLimitOptions) {
		opts.streamGetRetryAfter = getRetryAfter
	}
}

// RateLimitUnaryInterceptor is a gRPC unary interceptor that limits the rate of requests.
func RateLimitUnaryInterceptor(maxRate Rate, options ...RateLimitOption) (func(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error), error) {
	rlHandler, err := newRateLimitHandler(maxRate, true, options...)
	if err != nil {
		return nil, err
	}
	return rlHandler.handleUnary, nil
}

// RateLimitStreamInterceptor is a gRPC stream interceptor that limits the rate of requests.
func RateLimitStreamInterceptor(maxRate Rate, options ...RateLimitOption) (func(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error, error) {
	rlHandler, err := newRateLimitHandler(maxRate, false, options...)
	if err != nil {
		return nil, err
	}
	return rlHandler.handleStream, nil
}

// rateLimitUnaryRequestHandler implements rateLimitRequestHandler for unary requests
type rateLimitUnaryRequestHandler struct {
	ctx     context.Context
	req     interface{}
	info    *grpc.UnaryServerInfo
	handler grpc.UnaryHandler
	parent  *rateLimitHandler
	result  interface{}
}

func (u *rateLimitUnaryRequestHandler) GetContext() context.Context {
	return u.ctx
}

func (u *rateLimitUnaryRequestHandler) Execute() error {
	var err error
	u.result, err = u.handler(u.ctx, u.req)
	return err
}

func (u *rateLimitUnaryRequestHandler) OnReject(params ratelimit.Params) error {
	var err error
	u.result, err = u.parent.unaryOnReject(u.ctx, u.req, u.info, u.handler, u.convertParams(params))
	return err
}

func (u *rateLimitUnaryRequestHandler) OnError(params ratelimit.Params, err error) error {
	var handlerErr error
	u.result, handlerErr = u.parent.unaryOnError(u.ctx, u.req, u.info, u.handler, u.convertParams(params), err)
	return handlerErr
}

func (u *rateLimitUnaryRequestHandler) GetKey() (key string, bypass bool, err error) {
	if u.parent.unaryGetKey != nil {
		return u.parent.unaryGetKey(u.ctx, u.req, u.info)
	}
	return "", false, nil
}

func (u *rateLimitUnaryRequestHandler) convertParams(params ratelimit.Params) RateLimitParams {
	return RateLimitParams{
		Key:                 params.Key,
		RequestBacklogged:   params.RequestBacklogged,
		EstimatedRetryAfter: params.EstimatedRetryAfter,
		UnaryGetRetryAfter:  u.parent.unaryGetRetryAfter,
		StreamGetRetryAfter: u.parent.streamGetRetryAfter,
	}
}

// rateLimitStreamRequestHandler implements rateLimitRequestHandler for stream requests
type rateLimitStreamRequestHandler struct {
	srv     interface{}
	ss      grpc.ServerStream
	info    *grpc.StreamServerInfo
	handler grpc.StreamHandler
	parent  *rateLimitHandler
}

func (s *rateLimitStreamRequestHandler) GetContext() context.Context {
	return s.ss.Context()
}

func (s *rateLimitStreamRequestHandler) Execute() error {
	return s.handler(s.srv, s.ss)
}

func (s *rateLimitStreamRequestHandler) OnReject(params ratelimit.Params) error {
	grpcParams := s.convertParams(params)
	return s.parent.streamOnReject(s.srv, s.ss, s.info, s.handler, grpcParams)
}

func (s *rateLimitStreamRequestHandler) OnError(params ratelimit.Params, err error) error {
	grpcParams := s.convertParams(params)
	return s.parent.streamOnError(s.srv, s.ss, s.info, s.handler, grpcParams, err)
}

func (s *rateLimitStreamRequestHandler) GetKey() (key string, bypass bool, err error) {
	if s.parent.streamGetKey != nil {
		return s.parent.streamGetKey(s.srv, s.ss, s.info)
	}
	return "", false, nil
}

func (s *rateLimitStreamRequestHandler) convertParams(params ratelimit.Params) RateLimitParams {
	return RateLimitParams{
		Key:                 params.Key,
		RequestBacklogged:   params.RequestBacklogged,
		EstimatedRetryAfter: params.EstimatedRetryAfter,
		UnaryGetRetryAfter:  s.parent.unaryGetRetryAfter,
		StreamGetRetryAfter: s.parent.streamGetRetryAfter,
	}
}

type rateLimitHandler struct {
	processor           *ratelimit.RequestProcessor
	unaryGetKey         RateLimitUnaryGetKeyFunc
	streamGetKey        RateLimitStreamGetKeyFunc
	unaryOnReject       RateLimitUnaryOnRejectFunc
	streamOnReject      RateLimitStreamOnRejectFunc
	unaryOnError        RateLimitUnaryOnErrorFunc
	streamOnError       RateLimitStreamOnErrorFunc
	unaryGetRetryAfter  RateLimitUnaryGetRetryAfterFunc
	streamGetRetryAfter RateLimitStreamGetRetryAfterFunc
}

func newRateLimitHandler(maxRate Rate, isUnary bool, options ...RateLimitOption) (*rateLimitHandler, error) {
	opts := &rateLimitOptions{
		alg:                    RateLimitAlgLeakyBucket,
		backlogTimeout:         DefaultRateLimitBacklogTimeout,
		unaryOnReject:          DefaultRateLimitUnaryOnReject,
		streamOnReject:         DefaultRateLimitStreamOnReject,
		unaryOnRejectInDryRun:  DefaultRateLimitUnaryOnRejectInDryRun,
		streamOnRejectInDryRun: DefaultRateLimitStreamOnRejectInDryRun,
		unaryOnError:           DefaultRateLimitUnaryOnError,
		streamOnError:          DefaultRateLimitStreamOnError,
	}
	for _, option := range options {
		option(opts)
	}

	maxKeys := 0
	if (opts.unaryGetKey != nil && isUnary) || (opts.streamGetKey != nil && !isUnary) {
		maxKeys = opts.maxKeys
		if maxKeys == 0 {
			maxKeys = DefaultRateLimitMaxKeys
		}
	}

	var limiter ratelimit.Limiter
	var err error
	switch opts.alg {
	case RateLimitAlgLeakyBucket:
		limiter, err = ratelimit.NewLeakyBucketLimiter(maxRate, opts.maxBurst, maxKeys)
	case RateLimitAlgSlidingWindow:
		limiter, err = ratelimit.NewSlidingWindowLimiter(maxRate, maxKeys)
	default:
		return nil, fmt.Errorf("unknown rate limit algorithm")
	}
	if err != nil {
		return nil, err
	}

	backlogParams := ratelimit.BacklogParams{
		MaxKeys: maxKeys,
		Limit:   opts.backlogLimit,
		Timeout: opts.backlogTimeout,
	}
	if opts.dryRun {
		backlogParams.Limit = 0 // Backlogging should be disabled in dry-run mode to avoid blocking requests.
	}
	processor, err := ratelimit.NewRequestProcessor(limiter, backlogParams)
	if err != nil {
		return nil, fmt.Errorf("new rate limit request processor: %w", err)
	}

	return &rateLimitHandler{
		processor:           processor,
		unaryGetKey:         opts.unaryGetKey,
		streamGetKey:        opts.streamGetKey,
		unaryOnReject:       makeRateLimitUnaryOnRejectFunc(opts),
		streamOnReject:      makeRateLimitStreamOnRejectFunc(opts),
		unaryOnError:        opts.unaryOnError,
		streamOnError:       opts.streamOnError,
		unaryGetRetryAfter:  opts.unaryGetRetryAfter,
		streamGetRetryAfter: opts.streamGetRetryAfter,
	}, nil
}

func (h *rateLimitHandler) handleUnary(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (interface{}, error) {
	rh := &rateLimitUnaryRequestHandler{
		ctx:     ctx,
		req:     req,
		info:    info,
		handler: handler,
		parent:  h,
	}
	err := h.processor.ProcessRequest(rh)
	return rh.result, err
}

func (h *rateLimitHandler) handleStream(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	rh := &rateLimitStreamRequestHandler{
		srv:     srv,
		ss:      ss,
		info:    info,
		handler: handler,
		parent:  h,
	}
	return h.processor.ProcessRequest(rh)
}

// defaultRateLimitError contains the shared logic for handling rate limit errors
func defaultRateLimitError(ctx context.Context, params RateLimitParams, err error) error {
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Error("rate limiting error",
			log.String(RateLimitLogFieldKey, params.Key),
			log.Error(err),
		)
	}
	return status.Error(codes.Internal, "Internal server error")
}

// DefaultRateLimitUnaryOnReject sends gRPC error response when the rate limit is exceeded for unary requests.
func DefaultRateLimitUnaryOnReject(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams,
) (interface{}, error) {
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Warn("rate limit exceeded", log.String(RateLimitLogFieldKey, params.Key))
	}

	// Calculate retry-after using custom function if available, otherwise use estimated time
	retryAfter := params.EstimatedRetryAfter
	if params.UnaryGetRetryAfter != nil {
		retryAfter = params.UnaryGetRetryAfter(ctx, req, info, params.EstimatedRetryAfter)
	}

	// Set retry after header in gRPC metadata
	retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
	md := metadata.Pairs("retry-after", strconv.Itoa(retryAfterSeconds))
	if err := grpc.SetHeader(ctx, md); err != nil && logger != nil {
		logger.Warn("failed to set retry-after header", log.Error(err))
	}

	return nil, status.Error(codes.ResourceExhausted, "Too many requests")
}

// DefaultRateLimitStreamOnReject sends gRPC error response when the rate limit is exceeded for stream requests.
func DefaultRateLimitStreamOnReject(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams,
) error {
	ctx := ss.Context()
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Warn("rate limit exceeded", log.String(RateLimitLogFieldKey, params.Key))
	}

	// Calculate retry-after using custom function if available, otherwise use estimated time
	retryAfter := params.EstimatedRetryAfter
	if params.StreamGetRetryAfter != nil {
		retryAfter = params.StreamGetRetryAfter(srv, ss, info, params.EstimatedRetryAfter)
	}

	// Set retry after header in gRPC metadata
	retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
	md := metadata.Pairs("retry-after", strconv.Itoa(retryAfterSeconds))
	if err := ss.SetHeader(md); err != nil && logger != nil {
		logger.Warn("failed to set retry-after header", log.Error(err))
	}

	return status.Error(codes.ResourceExhausted, "Too many requests")
}

// DefaultRateLimitUnaryOnError sends gRPC error response when an error occurs during rate limiting in unary requests.
func DefaultRateLimitUnaryOnError(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams, err error,
) (interface{}, error) {
	handlerErr := defaultRateLimitError(ctx, params, err)
	return nil, handlerErr
}

// DefaultRateLimitStreamOnError sends gRPC error response when an error occurs during rate limiting in stream requests.
func DefaultRateLimitStreamOnError(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams, err error,
) error {
	return defaultRateLimitError(ss.Context(), params, err)
}

// DefaultRateLimitUnaryOnRejectInDryRun continues processing unary requests when rate limit is exceeded in dry run mode.
func DefaultRateLimitUnaryOnRejectInDryRun(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params RateLimitParams,
) (interface{}, error) {
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Warn("rate limit exceeded, continuing in dry run mode", log.String(RateLimitLogFieldKey, params.Key))
	}
	return handler(ctx, req)
}

// DefaultRateLimitStreamOnRejectInDryRun continues processing stream requests when rate limit is exceeded in dry run mode.
func DefaultRateLimitStreamOnRejectInDryRun(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params RateLimitParams,
) error {
	logger := GetLoggerFromContext(ss.Context())
	if logger != nil {
		logger.Warn("rate limit exceeded, continuing in dry run mode", log.String(RateLimitLogFieldKey, params.Key))
	}
	return handler(srv, ss)
}

func makeRateLimitUnaryOnRejectFunc(opts *rateLimitOptions) RateLimitUnaryOnRejectFunc {
	if opts.dryRun {
		if opts.unaryOnRejectInDryRun != nil {
			return opts.unaryOnRejectInDryRun
		}
		return DefaultRateLimitUnaryOnRejectInDryRun
	}
	if opts.unaryOnReject != nil {
		return opts.unaryOnReject
	}
	return DefaultRateLimitUnaryOnReject
}

func makeRateLimitStreamOnRejectFunc(opts *rateLimitOptions) RateLimitStreamOnRejectFunc {
	if opts.dryRun {
		if opts.streamOnRejectInDryRun != nil {
			return opts.streamOnRejectInDryRun
		}
		return DefaultRateLimitStreamOnRejectInDryRun
	}
	if opts.streamOnReject != nil {
		return opts.streamOnReject
	}
	return DefaultRateLimitStreamOnReject
}

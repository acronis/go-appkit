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

	"github.com/acronis/go-appkit/internal/inflightlimit"
	"github.com/acronis/go-appkit/log"
)

// DefaultInFlightLimitMaxKeys is a default value of maximum keys number for the InFlightLimit interceptor.
const DefaultInFlightLimitMaxKeys = 10000

// InFlightLimitLogFieldKey is the name of the logged field that contains a key for the in-flight limiting.
const InFlightLimitLogFieldKey = "in_flight_limit_key"

// InFlightLimitLogFieldBacklogged is the name of the logged field that indicates if the request was backlogged.
const InFlightLimitLogFieldBacklogged = "in_flight_limit_backlogged"

// InFlightLimitParams contains data that relates to the in-flight limiting procedure
// and could be used for rejecting or handling an occurred error.
type InFlightLimitParams struct {
	UnaryGetRetryAfter  InFlightLimitUnaryGetRetryAfterFunc
	StreamGetRetryAfter InFlightLimitStreamGetRetryAfterFunc
	Key                 string
	RequestBacklogged   bool
}

// InFlightLimitUnaryGetKeyFunc is a function that is called for getting key for in-flight limiting in unary requests.
type InFlightLimitUnaryGetKeyFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo) (key string, bypass bool, err error)

// InFlightLimitStreamGetKeyFunc is a function that is called for getting key for in-flight limiting in stream requests.
type InFlightLimitStreamGetKeyFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo) (key string, bypass bool, err error)

// InFlightLimitUnaryOnRejectFunc is a function that is called for rejecting gRPC unary request when the in-flight limit is exceeded.
type InFlightLimitUnaryOnRejectFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams) (interface{}, error)

// InFlightLimitStreamOnRejectFunc is a function that is called for rejecting gRPC stream request when the in-flight limit is exceeded.
type InFlightLimitStreamOnRejectFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams) error

// InFlightLimitUnaryOnErrorFunc is a function that is called when an error occurs during in-flight limiting in unary requests.
type InFlightLimitUnaryOnErrorFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams, err error) (interface{}, error)

// InFlightLimitStreamOnErrorFunc is a function that is called when an error occurs during in-flight limiting in stream requests.
type InFlightLimitStreamOnErrorFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams, err error) error

// InFlightLimitUnaryGetRetryAfterFunc is a function that is called to get a value for retry-after header
// when the in-flight limit is exceeded in unary requests.
type InFlightLimitUnaryGetRetryAfterFunc func(ctx context.Context, req interface{},
	info *grpc.UnaryServerInfo) time.Duration

// InFlightLimitStreamGetRetryAfterFunc is a function that is called to get a value for retry-after header
// when the in-flight limit is exceeded in stream requests.
type InFlightLimitStreamGetRetryAfterFunc func(srv interface{}, ss grpc.ServerStream,
	info *grpc.StreamServerInfo) time.Duration

// InFlightLimitOption represents a configuration option for the in-flight limit interceptor.
type InFlightLimitOption func(*inFlightLimitOptions)

type inFlightLimitOptions struct {
	unaryGetKey            InFlightLimitUnaryGetKeyFunc
	streamGetKey           InFlightLimitStreamGetKeyFunc
	maxKeys                int
	dryRun                 bool
	backlogLimit           int
	backlogTimeout         time.Duration
	unaryOnReject          InFlightLimitUnaryOnRejectFunc
	streamOnReject         InFlightLimitStreamOnRejectFunc
	unaryOnRejectInDryRun  InFlightLimitUnaryOnRejectFunc
	streamOnRejectInDryRun InFlightLimitStreamOnRejectFunc
	unaryOnError           InFlightLimitUnaryOnErrorFunc
	streamOnError          InFlightLimitStreamOnErrorFunc
	unaryGetRetryAfter     InFlightLimitUnaryGetRetryAfterFunc
	streamGetRetryAfter    InFlightLimitStreamGetRetryAfterFunc
}

// WithInFlightLimitUnaryGetKey sets the function to extract in-flight limiting key from unary gRPC requests.
func WithInFlightLimitUnaryGetKey(getKey InFlightLimitUnaryGetKeyFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.unaryGetKey = getKey
	}
}

// WithInFlightLimitStreamGetKey sets the function to extract in-flight limiting key from stream gRPC requests.
func WithInFlightLimitStreamGetKey(getKey InFlightLimitStreamGetKeyFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.streamGetKey = getKey
	}
}

// WithInFlightLimitMaxKeys sets the maximum number of keys to track.
func WithInFlightLimitMaxKeys(maxKeys int) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.maxKeys = maxKeys
	}
}

// WithInFlightLimitDryRun enables dry run mode where limits are checked but not enforced.
func WithInFlightLimitDryRun(dryRun bool) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.dryRun = dryRun
	}
}

// WithInFlightLimitBacklogLimit sets the backlog limit for queuing requests.
func WithInFlightLimitBacklogLimit(backlogLimit int) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.backlogLimit = backlogLimit
	}
}

// WithInFlightLimitBacklogTimeout sets the timeout for backlogged requests.
func WithInFlightLimitBacklogTimeout(backlogTimeout time.Duration) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.backlogTimeout = backlogTimeout
	}
}

// WithInFlightLimitUnaryOnReject sets the callback for handling rejected unary requests.
func WithInFlightLimitUnaryOnReject(onReject InFlightLimitUnaryOnRejectFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.unaryOnReject = onReject
	}
}

// WithInFlightLimitStreamOnReject sets the callback for handling rejected stream requests.
func WithInFlightLimitStreamOnReject(onReject InFlightLimitStreamOnRejectFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.streamOnReject = onReject
	}
}

// WithInFlightLimitUnaryOnRejectInDryRun sets the callback for handling rejected unary requests in dry run mode.
func WithInFlightLimitUnaryOnRejectInDryRun(onReject InFlightLimitUnaryOnRejectFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.unaryOnRejectInDryRun = onReject
	}
}

// WithInFlightLimitStreamOnRejectInDryRun sets the callback for handling rejected stream requests in dry run mode.
func WithInFlightLimitStreamOnRejectInDryRun(onReject InFlightLimitStreamOnRejectFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.streamOnRejectInDryRun = onReject
	}
}

// WithInFlightLimitUnaryOnError sets the callback for handling in-flight limiting errors in unary requests.
func WithInFlightLimitUnaryOnError(onError InFlightLimitUnaryOnErrorFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.unaryOnError = onError
	}
}

// WithInFlightLimitStreamOnError sets the callback for handling in-flight limiting errors in stream requests.
func WithInFlightLimitStreamOnError(onError InFlightLimitStreamOnErrorFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.streamOnError = onError
	}
}

// WithInFlightLimitUnaryGetRetryAfter sets the function to calculate retry-after value for unary requests.
func WithInFlightLimitUnaryGetRetryAfter(getRetryAfter InFlightLimitUnaryGetRetryAfterFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.unaryGetRetryAfter = getRetryAfter
	}
}

// WithInFlightLimitStreamGetRetryAfter sets the function to calculate retry-after value for stream requests.
func WithInFlightLimitStreamGetRetryAfter(getRetryAfter InFlightLimitStreamGetRetryAfterFunc) InFlightLimitOption {
	return func(opts *inFlightLimitOptions) {
		opts.streamGetRetryAfter = getRetryAfter
	}
}

// InFlightLimitUnaryInterceptor is a gRPC unary interceptor that limits the number of in-flight requests.
func InFlightLimitUnaryInterceptor(limit int, options ...InFlightLimitOption) (func(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error), error) {
	handler, err := newInFlightLimitHandler(limit, true, options...)
	if err != nil {
		return nil, err
	}
	return handler.handleUnary, nil
}

// InFlightLimitStreamInterceptor is a gRPC stream interceptor that limits the number of in-flight requests.
func InFlightLimitStreamInterceptor(limit int, options ...InFlightLimitOption) (func(
	srv interface{},
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error, error) {
	handler, err := newInFlightLimitHandler(limit, false, options...)
	if err != nil {
		return nil, err
	}
	return handler.handleStream, nil
}

type inFlightLimitHandler struct {
	processor           *inflightlimit.RequestProcessor
	unaryGetKey         InFlightLimitUnaryGetKeyFunc
	streamGetKey        InFlightLimitStreamGetKeyFunc
	unaryOnReject       InFlightLimitUnaryOnRejectFunc
	streamOnReject      InFlightLimitStreamOnRejectFunc
	unaryOnError        InFlightLimitUnaryOnErrorFunc
	streamOnError       InFlightLimitStreamOnErrorFunc
	unaryGetRetryAfter  InFlightLimitUnaryGetRetryAfterFunc
	streamGetRetryAfter InFlightLimitStreamGetRetryAfterFunc
}

func newInFlightLimitHandler(limit int, isUnary bool, options ...InFlightLimitOption) (*inFlightLimitHandler, error) {
	opts := &inFlightLimitOptions{
		backlogTimeout:         inflightlimit.DefaultInFlightLimitBacklogTimeout,
		unaryOnReject:          DefaultInFlightLimitUnaryOnReject,
		streamOnReject:         DefaultInFlightLimitStreamOnReject,
		unaryOnRejectInDryRun:  DefaultInFlightLimitUnaryOnRejectInDryRun,
		streamOnRejectInDryRun: DefaultInFlightLimitStreamOnRejectInDryRun,
		unaryOnError:           DefaultInFlightLimitUnaryOnError,
		streamOnError:          DefaultInFlightLimitStreamOnError,
	}
	for _, option := range options {
		option(opts)
	}

	maxKeys := 0
	if (opts.unaryGetKey != nil && isUnary) || (opts.streamGetKey != nil && !isUnary) {
		maxKeys = opts.maxKeys
		if maxKeys == 0 {
			maxKeys = DefaultInFlightLimitMaxKeys
		}
	}

	backlogParams := inflightlimit.BacklogParams{
		MaxKeys: maxKeys,
		Limit:   opts.backlogLimit,
		Timeout: opts.backlogTimeout,
	}

	processor, err := inflightlimit.NewRequestProcessor(limit, backlogParams, opts.dryRun)
	if err != nil {
		return nil, fmt.Errorf("create request processor: %w", err)
	}

	return &inFlightLimitHandler{
		processor:           processor,
		unaryGetKey:         opts.unaryGetKey,
		streamGetKey:        opts.streamGetKey,
		unaryOnReject:       makeInFlightLimitUnaryOnRejectFunc(opts),
		streamOnReject:      makeInFlightLimitStreamOnRejectFunc(opts),
		unaryOnError:        opts.unaryOnError,
		streamOnError:       opts.streamOnError,
		unaryGetRetryAfter:  opts.unaryGetRetryAfter,
		streamGetRetryAfter: opts.streamGetRetryAfter,
	}, nil
}

// unaryInFlightLimitRequestHandler implements inflightlimit.RequestHandler for unary gRPC requests.
type unaryInFlightLimitRequestHandler struct {
	ctx     context.Context
	req     interface{}
	info    *grpc.UnaryServerInfo
	handler grpc.UnaryHandler
	parent  *inFlightLimitHandler
	result  interface{}
}

func (rh *unaryInFlightLimitRequestHandler) GetContext() context.Context {
	return rh.ctx
}

func (rh *unaryInFlightLimitRequestHandler) GetKey() (key string, bypass bool, err error) {
	if rh.parent.unaryGetKey != nil {
		return rh.parent.unaryGetKey(rh.ctx, rh.req, rh.info)
	}
	return "", false, nil
}

func (rh *unaryInFlightLimitRequestHandler) Execute() error {
	var err error
	rh.result, err = rh.handler(rh.ctx, rh.req)
	return err
}

func (rh *unaryInFlightLimitRequestHandler) OnReject(params inflightlimit.Params) error {
	var handlerErr error
	rh.result, handlerErr = rh.parent.unaryOnReject(rh.ctx, rh.req, rh.info, rh.handler, rh.convertParams(params))
	return handlerErr
}

func (rh *unaryInFlightLimitRequestHandler) OnError(params inflightlimit.Params, err error) error {
	var handlerErr error
	rh.result, handlerErr = rh.parent.unaryOnError(rh.ctx, rh.req, rh.info, rh.handler, rh.convertParams(params), err)
	return handlerErr
}

func (rh *unaryInFlightLimitRequestHandler) convertParams(params inflightlimit.Params) InFlightLimitParams {
	return InFlightLimitParams{
		UnaryGetRetryAfter: rh.parent.unaryGetRetryAfter,
		Key:                params.Key,
		RequestBacklogged:  params.RequestBacklogged,
	}
}

// streamInFlightLimitRequestHandler implements inflightlimit.RequestHandler for stream gRPC requests.
type streamInFlightLimitRequestHandler struct {
	srv     interface{}
	ss      grpc.ServerStream
	info    *grpc.StreamServerInfo
	handler grpc.StreamHandler
	parent  *inFlightLimitHandler
}

func (rh *streamInFlightLimitRequestHandler) GetContext() context.Context {
	return rh.ss.Context()
}

func (rh *streamInFlightLimitRequestHandler) GetKey() (key string, bypass bool, err error) {
	if rh.parent.streamGetKey != nil {
		return rh.parent.streamGetKey(rh.srv, rh.ss, rh.info)
	}
	return "", false, nil
}

func (rh *streamInFlightLimitRequestHandler) Execute() error {
	return rh.handler(rh.srv, rh.ss)
}

func (rh *streamInFlightLimitRequestHandler) OnReject(params inflightlimit.Params) error {
	return rh.parent.streamOnReject(rh.srv, rh.ss, rh.info, rh.handler, rh.convertParams(params))
}

func (rh *streamInFlightLimitRequestHandler) OnError(params inflightlimit.Params, err error) error {
	return rh.parent.streamOnError(rh.srv, rh.ss, rh.info, rh.handler, rh.convertParams(params), err)
}

func (rh *streamInFlightLimitRequestHandler) convertParams(params inflightlimit.Params) InFlightLimitParams {
	return InFlightLimitParams{
		StreamGetRetryAfter: rh.parent.streamGetRetryAfter,
		Key:                 params.Key,
		RequestBacklogged:   params.RequestBacklogged,
	}
}

func (h *inFlightLimitHandler) handleUnary(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (interface{}, error) {
	requestHandler := &unaryInFlightLimitRequestHandler{ctx: ctx, req: req, info: info, handler: handler, parent: h}
	err := h.processor.ProcessRequest(requestHandler)
	return requestHandler.result, err
}

func (h *inFlightLimitHandler) handleStream(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler,
) error {
	requestHandler := &streamInFlightLimitRequestHandler{srv: srv, ss: ss, info: info, handler: handler, parent: h}
	return h.processor.ProcessRequest(requestHandler)
}

// DefaultInFlightLimitUnaryOnReject sends gRPC error response when the in-flight limit is exceeded for unary requests.
func DefaultInFlightLimitUnaryOnReject(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams,
) (interface{}, error) {
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Warn("in-flight limit exceeded",
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Bool(InFlightLimitLogFieldBacklogged, params.RequestBacklogged),
		)
	}

	// Set retry after header in gRPC metadata if available
	if params.UnaryGetRetryAfter != nil {
		retryAfter := params.UnaryGetRetryAfter(ctx, req, info)
		retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
		md := metadata.Pairs("retry-after", strconv.Itoa(retryAfterSeconds))
		if err := grpc.SetHeader(ctx, md); err != nil && logger != nil {
			logger.Warn("failed to set retry-after header", log.Error(err))
		}
	}

	return nil, status.Error(codes.ResourceExhausted, "Too many in-flight requests")
}

// DefaultInFlightLimitStreamOnReject sends gRPC error response when the in-flight limit is exceeded for stream requests.
func DefaultInFlightLimitStreamOnReject(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams,
) error {
	ctx := ss.Context()
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Warn("in-flight limit exceeded",
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Bool(InFlightLimitLogFieldBacklogged, params.RequestBacklogged),
		)
	}

	// Set retry after header in gRPC metadata if available
	if params.StreamGetRetryAfter != nil {
		retryAfter := params.StreamGetRetryAfter(srv, ss, info)
		retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
		md := metadata.Pairs("retry-after", strconv.Itoa(retryAfterSeconds))
		if err := ss.SetHeader(md); err != nil && logger != nil {
			logger.Warn("failed to set retry-after header", log.Error(err))
		}
	}

	return status.Error(codes.ResourceExhausted, "Too many in-flight requests")
}

// defaultInFlightLimitError contains the shared logic for handling in-flight limit errors
func defaultInFlightLimitError(ctx context.Context, params InFlightLimitParams, err error) error {
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Error("in-flight limiting error",
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Error(err),
		)
	}
	return status.Error(codes.Internal, "Internal server error")
}

// DefaultInFlightLimitUnaryOnError sends gRPC error response when an error occurs during in-flight limiting in unary requests.
func DefaultInFlightLimitUnaryOnError(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams, err error,
) (interface{}, error) {
	handlerErr := defaultInFlightLimitError(ctx, params, err)
	return nil, handlerErr
}

// DefaultInFlightLimitStreamOnError sends gRPC error response when an error occurs during in-flight limiting in stream requests.
func DefaultInFlightLimitStreamOnError(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams, err error,
) error {
	return defaultInFlightLimitError(ss.Context(), params, err)
}

// DefaultInFlightLimitUnaryOnRejectInDryRun continues processing unary requests when in-flight limit is exceeded in dry run mode.
func DefaultInFlightLimitUnaryOnRejectInDryRun(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params InFlightLimitParams,
) (interface{}, error) {
	logger := GetLoggerFromContext(ctx)
	if logger != nil {
		logger.Warn("in-flight limit exceeded, continuing in dry run mode",
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Bool(InFlightLimitLogFieldBacklogged, params.RequestBacklogged),
		)
	}
	return handler(ctx, req)
}

// DefaultInFlightLimitStreamOnRejectInDryRun continues processing stream requests when in-flight limit is exceeded in dry run mode.
func DefaultInFlightLimitStreamOnRejectInDryRun(
	srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler, params InFlightLimitParams,
) error {
	logger := GetLoggerFromContext(ss.Context())
	if logger != nil {
		logger.Warn("in-flight limit exceeded, continuing in dry run mode",
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Bool(InFlightLimitLogFieldBacklogged, params.RequestBacklogged),
		)
	}
	return handler(srv, ss)
}

func makeInFlightLimitUnaryOnRejectFunc(opts *inFlightLimitOptions) InFlightLimitUnaryOnRejectFunc {
	if opts.dryRun {
		if opts.unaryOnRejectInDryRun != nil {
			return opts.unaryOnRejectInDryRun
		}
		return DefaultInFlightLimitUnaryOnRejectInDryRun
	}
	if opts.unaryOnReject != nil {
		return opts.unaryOnReject
	}
	return DefaultInFlightLimitUnaryOnReject
}

func makeInFlightLimitStreamOnRejectFunc(opts *inFlightLimitOptions) InFlightLimitStreamOnRejectFunc {
	if opts.dryRun {
		if opts.streamOnRejectInDryRun != nil {
			return opts.streamOnRejectInDryRun
		}
		return DefaultInFlightLimitStreamOnRejectInDryRun
	}
	if opts.streamOnReject != nil {
		return opts.streamOnReject
	}
	return DefaultInFlightLimitStreamOnReject
}

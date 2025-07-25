/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/acronis/go-appkit/internal/ratelimit"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/restapi"
)

// DefaultRateLimitMaxKeys is a default value of maximum keys number for the RateLimit middleware.
const DefaultRateLimitMaxKeys = 10000

// DefaultRateLimitBacklogTimeout determines how long the HTTP request may be in the backlog status.
const DefaultRateLimitBacklogTimeout = ratelimit.DefaultRateLimitBacklogTimeout

// RateLimitErrCode is an error code that is used in a response body
// if the request is rejected by the middleware that limits the rate of HTTP requests.
const RateLimitErrCode = "tooManyRequests"

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
	ErrDomain           string
	ResponseStatusCode  int
	GetRetryAfter       RateLimitGetRetryAfterFunc
	Key                 string
	RequestBacklogged   bool
	EstimatedRetryAfter time.Duration
}

// RateLimitGetRetryAfterFunc is a function that is called to get a value for Retry-After response HTTP header
// when the rate limit is exceeded.
type RateLimitGetRetryAfterFunc func(r *http.Request, estimatedTime time.Duration) time.Duration

// RateLimitOnRejectFunc is a function that is called for rejecting HTTP request when the rate limit is exceeded.
type RateLimitOnRejectFunc func(
	rw http.ResponseWriter, r *http.Request, params RateLimitParams, next http.Handler, logger log.FieldLogger)

// RateLimitOnErrorFunc is a function that is called for rejecting HTTP request when the rate limit is exceeded.
type RateLimitOnErrorFunc func(
	rw http.ResponseWriter, r *http.Request, params RateLimitParams, err error, next http.Handler, logger log.FieldLogger)

// RateLimitGetKeyFunc is a function that is called for getting key for rate limiting.
type RateLimitGetKeyFunc func(r *http.Request) (key string, bypass bool, err error)

type rateLimitHandler struct {
	next           http.Handler
	processor      *ratelimit.RequestProcessor
	getKey         RateLimitGetKeyFunc
	errDomain      string
	respStatusCode int
	getRetryAfter  RateLimitGetRetryAfterFunc

	onReject RateLimitOnRejectFunc
	onError  RateLimitOnErrorFunc
}

func (h *rateLimitHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	requestHandler := &rateLimitRequestHandler{rw: rw, r: r, parent: h}
	_ = h.processor.ProcessRequest(requestHandler) // Error is always nil, as it is handled in the rateLimitRequestHandler methods.
}

// rateLimitRequestHandler implements ratelimit.RequestHandler for HTTP requests
type rateLimitRequestHandler struct {
	rw     http.ResponseWriter
	r      *http.Request
	parent *rateLimitHandler
}

func (h *rateLimitRequestHandler) GetContext() context.Context {
	return h.r.Context()
}

func (h *rateLimitRequestHandler) GetKey() (key string, bypass bool, err error) {
	if h.parent.getKey != nil {
		return h.parent.getKey(h.r)
	}
	return "", false, nil
}

func (h *rateLimitRequestHandler) Execute() error {
	h.parent.next.ServeHTTP(h.rw, h.r)
	return nil
}

func (h *rateLimitRequestHandler) OnReject(params ratelimit.Params) error {
	h.parent.onReject(h.rw, h.r, h.convertParams(params), h.parent.next, GetLoggerFromContext(h.r.Context()))
	return nil
}

func (h *rateLimitRequestHandler) OnError(params ratelimit.Params, err error) error {
	h.parent.onError(h.rw, h.r, h.convertParams(params), err, h.parent.next, GetLoggerFromContext(h.r.Context()))
	return nil
}

func (h *rateLimitRequestHandler) convertParams(params ratelimit.Params) RateLimitParams {
	return RateLimitParams{
		ErrDomain:           h.parent.errDomain,
		ResponseStatusCode:  h.parent.respStatusCode,
		GetRetryAfter:       h.parent.getRetryAfter,
		Key:                 params.Key,
		RequestBacklogged:   params.RequestBacklogged,
		EstimatedRetryAfter: params.EstimatedRetryAfter,
	}
}

// RateLimitOpts represents an options for the RateLimit middleware.
type RateLimitOpts struct {
	Alg                RateLimitAlg
	MaxBurst           int
	GetKey             RateLimitGetKeyFunc
	MaxKeys            int
	ResponseStatusCode int
	GetRetryAfter      RateLimitGetRetryAfterFunc
	DryRun             bool
	BacklogLimit       int
	BacklogTimeout     time.Duration

	OnReject         RateLimitOnRejectFunc
	OnRejectInDryRun RateLimitOnRejectFunc
	OnError          RateLimitOnErrorFunc
}

// Rate describes the frequency of requests.
type Rate = ratelimit.Rate

// RateLimit is a middleware that limits the rate of HTTP requests.
func RateLimit(maxRate Rate, errDomain string) (func(next http.Handler) http.Handler, error) {
	return RateLimitWithOpts(maxRate, errDomain, RateLimitOpts{GetRetryAfter: GetRetryAfterEstimatedTime})
}

// MustRateLimit is a version of RateLimit that panics if an error occurs.
func MustRateLimit(maxRate Rate, errDomain string) func(next http.Handler) http.Handler {
	mw, err := RateLimit(maxRate, errDomain)
	if err != nil {
		panic(err)
	}
	return mw
}

// RateLimitWithOpts is a configurable version of a middleware to limit the rate of HTTP requests.
func RateLimitWithOpts(maxRate Rate, errDomain string, opts RateLimitOpts) (func(next http.Handler) http.Handler, error) {
	maxKeys := 0
	if opts.GetKey != nil {
		maxKeys = opts.MaxKeys
		if maxKeys == 0 {
			maxKeys = DefaultRateLimitMaxKeys
		}
	}

	respStatusCode := opts.ResponseStatusCode
	if respStatusCode == 0 {
		respStatusCode = http.StatusServiceUnavailable
	}

	makeLimiter := func() (ratelimit.Limiter, error) {
		switch opts.Alg {
		case RateLimitAlgLeakyBucket:
			return ratelimit.NewLeakyBucketLimiter(maxRate, opts.MaxBurst, maxKeys)
		case RateLimitAlgSlidingWindow:
			return ratelimit.NewSlidingWindowLimiter(maxRate, maxKeys)
		default:
			return nil, fmt.Errorf("unknown rate limit alg")
		}
	}
	limiter, err := makeLimiter()
	if err != nil {
		return nil, err
	}

	backlogParams := ratelimit.BacklogParams{
		MaxKeys: maxKeys,
		Limit:   opts.BacklogLimit,
		Timeout: opts.BacklogTimeout,
	}
	if opts.DryRun {
		backlogParams.Limit = 0 // Backlogging should be disabled in dry-run mode to avoid blocking requests.
	}
	processor, err := ratelimit.NewRequestProcessor(limiter, backlogParams)
	if err != nil {
		return nil, fmt.Errorf("new rate limit request processor: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return &rateLimitHandler{
			next:           next,
			processor:      processor,
			errDomain:      errDomain,
			getKey:         opts.GetKey,
			getRetryAfter:  opts.GetRetryAfter,
			respStatusCode: respStatusCode,
			onReject:       makeRateLimitOnRejectFunc(opts),
			onError:        makeRateLimitOnErrorFunc(opts),
		}
	}, nil
}

// MustRateLimitWithOpts is a version of RateLimitWithOpts that panics if an error occurs.
func MustRateLimitWithOpts(maxRate Rate, errDomain string, opts RateLimitOpts) func(next http.Handler) http.Handler {
	mw, err := RateLimitWithOpts(maxRate, errDomain, opts)
	if err != nil {
		panic(err)
	}
	return mw
}

// GetRetryAfterEstimatedTime returns estimated time after that the client may retry the request.
func GetRetryAfterEstimatedTime(_ *http.Request, estimatedTime time.Duration) time.Duration {
	return estimatedTime
}

// DefaultRateLimitOnReject sends HTTP response in a typical go-appkit way when the rate limit is exceeded,
// or when the request is backlogged and the backlog limit is exceeded.
func DefaultRateLimitOnReject(
	rw http.ResponseWriter, r *http.Request, params RateLimitParams, next http.Handler, logger log.FieldLogger,
) {
	if logger != nil {
		logger = logger.With(
			log.String(RateLimitLogFieldKey, params.Key),
			log.String(userAgentLogFieldKey, r.UserAgent()),
		)
	}
	if params.GetRetryAfter != nil {
		rw.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(params.GetRetryAfter(r, params.EstimatedRetryAfter).Seconds()))))
	}
	apiErr := restapi.NewError(params.ErrDomain, RateLimitErrCode, "Too many requests.")
	restapi.RespondError(rw, params.ResponseStatusCode, apiErr, logger)
}

// DefaultRateLimitOnError sends HTTP response in a typical go-appkit way in case when the error occurs during the in-flight limiting.
func DefaultRateLimitOnError(
	rw http.ResponseWriter, r *http.Request, params RateLimitParams, err error, next http.Handler, logger log.FieldLogger,
) {
	if logger != nil {
		logger.Error(err.Error(), log.String(RateLimitLogFieldKey, params.Key))
	}
	restapi.RespondInternalError(rw, params.ErrDomain, logger)
}

// DefaultRateLimitOnRejectInDryRun sends HTTP response in a typical go-appkit way when the rate limit is exceeded in the dry-run mode.
func DefaultRateLimitOnRejectInDryRun(
	rw http.ResponseWriter, r *http.Request, params RateLimitParams, next http.Handler, logger log.FieldLogger,
) {
	if logger != nil {
		logger.Warn("too many requests, serving will be continued because of dry run mode",
			log.String(RateLimitLogFieldKey, params.Key),
			log.String(userAgentLogFieldKey, r.UserAgent()),
		)
	}
	next.ServeHTTP(rw, r)
}

func makeRateLimitOnRejectFunc(opts RateLimitOpts) RateLimitOnRejectFunc {
	if opts.DryRun {
		if opts.OnRejectInDryRun != nil {
			return opts.OnRejectInDryRun
		}
		return DefaultRateLimitOnRejectInDryRun
	}
	if opts.OnReject != nil {
		return opts.OnReject
	}
	return DefaultRateLimitOnReject
}

func makeRateLimitOnErrorFunc(opts RateLimitOpts) RateLimitOnErrorFunc {
	if opts.OnError != nil {
		return opts.OnError
	}
	return DefaultRateLimitOnError
}

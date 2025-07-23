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

	"github.com/RussellLuo/slidingwindow"
	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/memstore"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/lrucache"
	"github.com/acronis/go-appkit/restapi"
)

// DefaultRateLimitMaxKeys is a default value of maximum keys number for the RateLimit middleware.
const DefaultRateLimitMaxKeys = 10000

// DefaultRateLimitBacklogTimeout determines how long the HTTP request may be in the backlog status.
const DefaultRateLimitBacklogTimeout = time.Second * 5

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
type RateLimitOnRejectFunc func(rw http.ResponseWriter, r *http.Request,
	params RateLimitParams, next http.Handler, logger log.FieldLogger)

// RateLimitOnErrorFunc is a function that is called for rejecting HTTP request when the rate limit is exceeded.
type RateLimitOnErrorFunc func(rw http.ResponseWriter, r *http.Request,
	params RateLimitParams, err error, next http.Handler, logger log.FieldLogger)

// RateLimitGetKeyFunc is a function that is called for getting key for rate limiting.
type RateLimitGetKeyFunc func(r *http.Request) (key string, bypass bool, err error)

type rateLimitHandler struct {
	next            http.Handler
	limiter         rateLimiter
	getKey          RateLimitGetKeyFunc
	errDomain       string
	respStatusCode  int
	getRetryAfter   RateLimitGetRetryAfterFunc
	getBacklogSlots func(key string) chan struct{}
	backlogTimeout  time.Duration

	onReject RateLimitOnRejectFunc
	onError  RateLimitOnErrorFunc
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
type Rate struct {
	Count    int
	Duration time.Duration
}

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
	backlogLimit := opts.BacklogLimit
	if backlogLimit < 0 {
		return nil, fmt.Errorf("backlog limit should not be negative, got %d", backlogLimit)
	}
	if opts.DryRun {
		backlogLimit = 0 // Backlogging should be disabled in dry-run mode to avoid blocking requests.
	}

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

	makeLimiter := func() (rateLimiter, error) {
		switch opts.Alg {
		case RateLimitAlgLeakyBucket:
			return newLeakyBucketLimiter(maxRate, opts.MaxBurst, maxKeys)
		case RateLimitAlgSlidingWindow:
			return newSlidingWindowLimiter(maxRate, maxKeys)
		default:
			return nil, fmt.Errorf("unknown rate limit alg")
		}
	}
	limiter, err := makeLimiter()
	if err != nil {
		return nil, err
	}

	getBacklogSlots, err := makeRateLimitBacklogSlotsProvider(backlogLimit, maxKeys)
	if err != nil {
		return nil, fmt.Errorf("make rate limit backlog slots provider: %w", err)
	}

	backlogTimeout := opts.BacklogTimeout
	if backlogTimeout == 0 {
		backlogTimeout = DefaultRateLimitBacklogTimeout
	}

	return func(next http.Handler) http.Handler {
		return &rateLimitHandler{
			next:            next,
			errDomain:       errDomain,
			limiter:         limiter,
			getKey:          opts.GetKey,
			getRetryAfter:   opts.GetRetryAfter,
			respStatusCode:  respStatusCode,
			getBacklogSlots: getBacklogSlots,
			backlogTimeout:  backlogTimeout,
			onReject:        makeRateLimitOnRejectFunc(opts),
			onError:         makeRateLimitOnErrorFunc(opts),
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

func (h *rateLimitHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var key string
	if h.getKey != nil {
		var bypass bool
		var err error
		if key, bypass, err = h.getKey(r); err != nil {
			h.onError(rw, r, h.makeParams("", false, 0), fmt.Errorf("get key for rate limit: %w", err),
				h.next, GetLoggerFromContext(r.Context()))
			return
		}
		if bypass { // Rate limiting is bypassed for this request.
			h.next.ServeHTTP(rw, r)
			return
		}
	}

	allow, retryAfter, err := h.limiter.Allow(r.Context(), key)
	if err != nil {
		h.onError(rw, r, h.makeParams(key, false, 0), fmt.Errorf("rate limit: %w", err),
			h.next, GetLoggerFromContext(r.Context()))
		return
	}
	if allow {
		h.next.ServeHTTP(rw, r)
		return
	}

	if h.getBacklogSlots == nil { // Backlogging is disabled.
		h.onReject(rw, r, h.makeParams(key, false, retryAfter), h.next, GetLoggerFromContext(r.Context()))
		return
	}

	h.handleBacklogProcessing(rw, r, key, retryAfter)
}

func (h *rateLimitHandler) handleBacklogProcessing(
	rw http.ResponseWriter, r *http.Request, key string, retryAfter time.Duration,
) {
	backlogSlots := h.getBacklogSlots(key)
	backlogged := false
	select {
	case backlogSlots <- struct{}{}:
		backlogged = true
	default:
		// There are no free slots in the backlog, reject the request immediately.
		h.onReject(rw, r, h.makeParams(key, backlogged, retryAfter), h.next, GetLoggerFromContext(r.Context()))
		return
	}

	freeBacklogSlotIfNeeded := func() {
		if backlogged {
			select {
			case <-backlogSlots:
				backlogged = false
			default:
			}
		}
	}

	defer freeBacklogSlotIfNeeded()

	backlogTimeoutTimer := time.NewTimer(h.backlogTimeout)
	defer backlogTimeoutTimer.Stop()

	retryTimer := time.NewTimer(retryAfter)
	defer retryTimer.Stop()

	var allow bool
	var err error

	for {
		select {
		case <-retryTimer.C:
			// Will do another check of the rate limit.
		case <-backlogTimeoutTimer.C:
			freeBacklogSlotIfNeeded()
			h.onReject(rw, r, h.makeParams(key, backlogged, retryAfter), h.next, GetLoggerFromContext(r.Context()))
			return
		case <-r.Context().Done():
			freeBacklogSlotIfNeeded()
			h.onError(rw, r, h.makeParams(key, backlogged, retryAfter), r.Context().Err(), h.next, GetLoggerFromContext(r.Context()))
			return
		}

		if allow, retryAfter, err = h.limiter.Allow(r.Context(), key); err != nil {
			freeBacklogSlotIfNeeded()
			h.onError(rw, r, h.makeParams(key, backlogged, retryAfter), fmt.Errorf("requests rate limit: %w", err),
				h.next, GetLoggerFromContext(r.Context()))
			return
		}

		if allow {
			freeBacklogSlotIfNeeded()
			h.next.ServeHTTP(rw, r)
			return
		}

		if !retryTimer.Stop() {
			select {
			case <-retryTimer.C:
			default:
			}
		}
		retryTimer.Reset(retryAfter)
	}
}

func (h *rateLimitHandler) makeParams(key string, backlogged bool, estimatedRetryAfter time.Duration) RateLimitParams {
	return RateLimitParams{
		ErrDomain:           h.errDomain,
		ResponseStatusCode:  h.respStatusCode,
		GetRetryAfter:       h.getRetryAfter,
		Key:                 key,
		RequestBacklogged:   backlogged,
		EstimatedRetryAfter: estimatedRetryAfter,
	}
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

func makeRateLimitBacklogSlotsProvider(backlogLimit, maxKeys int) (func(key string) chan struct{}, error) {
	if backlogLimit == 0 {
		return nil, nil
	}
	if maxKeys == 0 {
		backlogSlots := make(chan struct{}, backlogLimit)
		return func(key string) chan struct{} {
			return backlogSlots
		}, nil
	}

	keysZone, err := lrucache.New[string, chan struct{}](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return func(key string) chan struct{} {
		backlogSlots, _ := keysZone.GetOrAdd(key, func() chan struct{} {
			return make(chan struct{}, backlogLimit)
		})
		return backlogSlots
	}, nil
}

type rateLimiter interface {
	Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error)
}

// leakyBucketLimiter implements GCRA (Generic Cell Rate Algorithm). It's a leaky bucket variant algorithm.
// More details and good explanation of this alg is provided here: https://brandur.org/rate-limiting#gcra.
type leakyBucketLimiter struct {
	limiter *throttled.GCRARateLimiterCtx
}

func newLeakyBucketLimiter(maxRate Rate, maxBurst, maxKeys int) (*leakyBucketLimiter, error) {
	gcraStore, err := memstore.NewCtx(maxKeys)
	if err != nil {
		return nil, fmt.Errorf("new in-memory store: %w", err)
	}
	reqQuota := throttled.RateQuota{
		MaxRate:  throttled.PerDuration(maxRate.Count, maxRate.Duration),
		MaxBurst: maxBurst,
	}
	gcraLimiter, err := throttled.NewGCRARateLimiterCtx(gcraStore, reqQuota)
	if err != nil {
		return nil, fmt.Errorf("new GCRA rate limiter: %w", err)
	}
	return &leakyBucketLimiter{gcraLimiter}, nil
}

func (l *leakyBucketLimiter) Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	limited, res, err := l.limiter.RateLimitCtx(ctx, key, 1)
	if err != nil {
		return false, 0, err
	}
	return !limited, res.RetryAfter, nil
}

type slidingWindowLimiter struct {
	getLimiter func(key string) *slidingwindow.Limiter
	maxRate    Rate
}

func newSlidingWindowLimiter(maxRate Rate, maxKeys int) (*slidingWindowLimiter, error) {
	if maxKeys == 0 {
		lim, _ := slidingwindow.NewLimiter(
			maxRate.Duration, int64(maxRate.Count), func() (slidingwindow.Window, slidingwindow.StopFunc) {
				return slidingwindow.NewLocalWindow()
			})
		return &slidingWindowLimiter{
			maxRate:    maxRate,
			getLimiter: func(_ string) *slidingwindow.Limiter { return lim },
		}, nil
	}

	store, err := lrucache.New[string, *slidingwindow.Limiter](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return &slidingWindowLimiter{
		maxRate: maxRate,
		getLimiter: func(key string) *slidingwindow.Limiter {
			lim, _ := store.GetOrAdd(key, func() *slidingwindow.Limiter {
				lim, _ := slidingwindow.NewLimiter(
					maxRate.Duration, int64(maxRate.Count), func() (slidingwindow.Window, slidingwindow.StopFunc) {
						return slidingwindow.NewLocalWindow()
					})
				return lim
			})
			return lim
		},
	}, nil
}

func (l *slidingWindowLimiter) Allow(_ context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	if l.getLimiter(key).Allow() {
		return true, 0, nil
	}
	now := time.Now()
	retryAfter = now.Truncate(l.maxRate.Duration).Add(l.maxRate.Duration).Sub(now)
	return false, retryAfter, nil
}

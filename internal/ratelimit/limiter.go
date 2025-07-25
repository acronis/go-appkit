/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/RussellLuo/slidingwindow"
	"github.com/throttled/throttled/v2"
	"github.com/throttled/throttled/v2/store/memstore"

	"github.com/acronis/go-appkit/lrucache"
)

// DefaultRateLimitBacklogTimeout determines the default timeout for backlog processing.
const DefaultRateLimitBacklogTimeout = time.Second * 5

// Rate describes the frequency of requests.
type Rate struct {
	Count    int
	Duration time.Duration
}

// Limiter interface defines the rate limiting contract.
type Limiter interface {
	Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error)
}

// LeakyBucketLimiter implements GCRA (Generic Cell Rate Algorithm). It's a leaky bucket variant algorithm.
// More details and good explanation of this alg is provided here: https://brandur.org/rate-limiting#gcra.
type LeakyBucketLimiter struct {
	limiter *throttled.GCRARateLimiterCtx
}

// NewLeakyBucketLimiter creates a new leaky bucket rate limiter.
func NewLeakyBucketLimiter(maxRate Rate, maxBurst, maxKeys int) (*LeakyBucketLimiter, error) {
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
	return &LeakyBucketLimiter{gcraLimiter}, nil
}

// Allow checks if the request should be allowed based on the rate limit.
func (l *LeakyBucketLimiter) Allow(ctx context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	limited, res, err := l.limiter.RateLimitCtx(ctx, key, 1)
	if err != nil {
		return false, 0, err
	}
	return !limited, res.RetryAfter, nil
}

// SlidingWindowLimiter implements sliding window rate limiting algorithm.
type SlidingWindowLimiter struct {
	getLimiter func(key string) *slidingwindow.Limiter
	maxRate    Rate
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter.
func NewSlidingWindowLimiter(maxRate Rate, maxKeys int) (*SlidingWindowLimiter, error) {
	if maxKeys == 0 {
		lim, _ := slidingwindow.NewLimiter(
			maxRate.Duration, int64(maxRate.Count), func() (slidingwindow.Window, slidingwindow.StopFunc) {
				return slidingwindow.NewLocalWindow()
			})
		return &SlidingWindowLimiter{
			maxRate:    maxRate,
			getLimiter: func(_ string) *slidingwindow.Limiter { return lim },
		}, nil
	}

	store, err := lrucache.New[string, *slidingwindow.Limiter](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return &SlidingWindowLimiter{
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

// Allow checks if the request should be allowed based on the rate limit.
func (l *SlidingWindowLimiter) Allow(_ context.Context, key string) (allow bool, retryAfter time.Duration, err error) {
	if l.getLimiter(key).Allow() {
		return true, 0, nil
	}
	now := time.Now()
	retryAfter = now.Truncate(l.maxRate.Duration).Add(l.maxRate.Duration).Sub(now)
	return false, retryAfter, nil
}

// backlogSlotsProvider provides backlog slots for rate limiting.
type backlogSlotsProvider func(key string) chan struct{}

// newBacklogSlotsProvider creates a new backlog slots provider.
func newBacklogSlotsProvider(backlogLimit, maxKeys int) backlogSlotsProvider {
	if maxKeys == 0 {
		backlogSlots := make(chan struct{}, backlogLimit)
		return func(key string) chan struct{} {
			return backlogSlots
		}
	}
	keysZone, _ := lrucache.New[string, chan struct{}](maxKeys, nil) // Error is always nil here.
	return func(key string) chan struct{} {
		backlogSlots, _ := keysZone.GetOrAdd(key, func() chan struct{} {
			return make(chan struct{}, backlogLimit)
		})
		return backlogSlots
	}
}

// Params contains common data that relates to the rate limiting procedure.
type Params struct {
	Key                 string
	RequestBacklogged   bool
	EstimatedRetryAfter time.Duration
}

// RequestHandler abstracts the common operations for both HTTP and gRPC requests.
type RequestHandler interface {
	// GetContext returns the request context.
	GetContext() context.Context

	// GetKey extracts the rate limiting key from the request.
	// Returns key, bypass (whether to bypass rate limiting), and error.
	GetKey() (string, bool, error)

	// Execute processes the actual request.
	Execute() error

	// OnReject handles request rejection when rate limit is exceeded.
	OnReject(params Params) error

	// OnError handles errors that occur during rate limiting.
	OnError(params Params, err error) error
}

// RequestProcessor handles the common rate limiting logic for any request type.
type RequestProcessor struct {
	limiter         Limiter
	getBacklogSlots backlogSlotsProvider
	backlogTimeout  time.Duration
}

// BacklogParams defines parameters for the backlog processing.
type BacklogParams struct {
	MaxKeys int
	Limit   int
	Timeout time.Duration
}

// NewRequestProcessor creates a new generic request processor.
func NewRequestProcessor(limiter Limiter, backlogParams BacklogParams) (*RequestProcessor, error) {
	if backlogParams.Limit < 0 {
		return nil, fmt.Errorf("backlog limit should not be negative, got %d", backlogParams.Limit)
	}
	if backlogParams.MaxKeys < 0 {
		return nil, fmt.Errorf("max keys for backlog should not be negative, got %d", backlogParams.MaxKeys)
	}
	var getBacklogSlots backlogSlotsProvider
	if backlogParams.Limit > 0 {
		getBacklogSlots = newBacklogSlotsProvider(backlogParams.Limit, backlogParams.MaxKeys)
	}

	if backlogParams.Timeout == 0 {
		backlogParams.Timeout = DefaultRateLimitBacklogTimeout
	}

	return &RequestProcessor{
		limiter:         limiter,
		getBacklogSlots: getBacklogSlots,
		backlogTimeout:  backlogParams.Timeout,
	}, nil
}

// ProcessRequest contains the shared rate limiting logic.
func (p *RequestProcessor) ProcessRequest(rh RequestHandler) error {
	ctx := rh.GetContext()

	key, bypass, err := rh.GetKey()
	if err != nil {
		return rh.OnError(Params{Key: key}, fmt.Errorf("get key for rate limit: %w", err))
	}
	if bypass { // Rate limiting is bypassed for this request.
		return rh.Execute()
	}

	allow, retryAfter, err := p.limiter.Allow(ctx, key)
	if err != nil {
		return rh.OnError(Params{Key: key}, fmt.Errorf("rate limit: %w", err))
	}

	if allow {
		return rh.Execute()
	}

	if p.getBacklogSlots == nil { // Backlogging is disabled.
		return rh.OnReject(Params{
			Key:                 key,
			RequestBacklogged:   false,
			EstimatedRetryAfter: retryAfter,
		})
	}

	return p.processBacklog(rh, key, retryAfter)
}

// processBacklog contains the shared backlog processing logic.
func (p *RequestProcessor) processBacklog(rh RequestHandler, key string, retryAfter time.Duration) error {
	ctx := rh.GetContext()

	backlogSlots := p.getBacklogSlots(key)
	backlogged := false
	select {
	case backlogSlots <- struct{}{}:
		backlogged = true
	default:
		// There are no free slots in the backlog, reject the request immediately.
		return rh.OnReject(Params{
			Key:                 key,
			RequestBacklogged:   backlogged,
			EstimatedRetryAfter: retryAfter,
		})
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

	backlogTimeoutTimer := time.NewTimer(p.backlogTimeout)
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
			return rh.OnReject(Params{
				Key:                 key,
				RequestBacklogged:   backlogged,
				EstimatedRetryAfter: retryAfter,
			})
		case <-ctx.Done():
			freeBacklogSlotIfNeeded()
			return rh.OnError(Params{
				Key:                 key,
				RequestBacklogged:   backlogged,
				EstimatedRetryAfter: retryAfter,
			}, ctx.Err())
		}

		if allow, retryAfter, err = p.limiter.Allow(ctx, key); err != nil {
			freeBacklogSlotIfNeeded()
			return rh.OnError(Params{
				Key:                 key,
				RequestBacklogged:   backlogged,
				EstimatedRetryAfter: retryAfter,
			}, fmt.Errorf("rate limit: %w", err))
		}

		if allow {
			freeBacklogSlotIfNeeded()
			return rh.Execute()
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

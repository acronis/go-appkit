/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/lrucache"
	"github.com/acronis/go-appkit/restapi"
)

// DefaultInFlightLimitMaxKeys is a default value of maximum keys number for the InFlightLimit middleware.
const DefaultInFlightLimitMaxKeys = 10000

// DefaultInFlightLimitBacklogTimeout determines how long the HTTP request may be in the backlog status.
const DefaultInFlightLimitBacklogTimeout = time.Second * 5

// InFlightLimitErrCode is the error code that is used in a response body
// if the request is rejected by the middleware that limits in-flight HTTP requests.
const InFlightLimitErrCode = "tooManyInFlightRequests"

// Log fields for InFlightLimit middleware.
const (
	InFlightLimitLogFieldKey        = "in_flight_limit_key"
	InFlightLimitLogFieldBacklogged = "in_flight_limit_backlogged"
)

// InFlightLimitParams contains data that relates to the in-flight limiting procedure
// and could be used for rejecting or handling an occurred error.
type InFlightLimitParams struct {
	ResponseStatusCode int
	GetRetryAfter      InFlightLimitGetRetryAfterFunc
	ErrDomain          string
	Key                string
	RequestBacklogged  bool
}

// InFlightLimitGetRetryAfterFunc is a function that is called to get a value for Retry-After response HTTP header
// when the in-flight limit is exceeded.
type InFlightLimitGetRetryAfterFunc func(r *http.Request) time.Duration

// InFlightLimitOnRejectFunc is a function that is called for rejecting HTTP request when the in-flight limit is exceeded.
type InFlightLimitOnRejectFunc func(rw http.ResponseWriter, r *http.Request,
	params InFlightLimitParams, next http.Handler, logger log.FieldLogger)

// InFlightLimitOnErrorFunc is a function that is called in case of any error that may occur during the in-flight limiting.
type InFlightLimitOnErrorFunc func(rw http.ResponseWriter, r *http.Request,
	params InFlightLimitParams, err error, next http.Handler, logger log.FieldLogger)

// InFlightLimitGetKeyFunc is a function that is called for getting key for in-flight limiting.
type InFlightLimitGetKeyFunc func(r *http.Request) (key string, bypass bool, err error)

type inFlightLimitHandler struct {
	next           http.Handler
	getKey         InFlightLimitGetKeyFunc
	getSlots       func(key string) (inFlightSlots chan struct{}, backlogSlots chan struct{})
	backlogTimeout time.Duration
	errDomain      string
	respStatusCode int
	getRetryAfter  InFlightLimitGetRetryAfterFunc
	dryRun         bool

	onReject InFlightLimitOnRejectFunc
	onError  InFlightLimitOnErrorFunc
}

// InFlightLimitOpts represents an options for the middleware to limit in-flight HTTP requests.
type InFlightLimitOpts struct {
	GetKey             InFlightLimitGetKeyFunc
	MaxKeys            int
	ResponseStatusCode int
	GetRetryAfter      InFlightLimitGetRetryAfterFunc
	BacklogLimit       int
	BacklogTimeout     time.Duration
	DryRun             bool

	OnReject         InFlightLimitOnRejectFunc
	OnRejectInDryRun InFlightLimitOnRejectFunc
	OnError          InFlightLimitOnErrorFunc
}

// InFlightLimit is a middleware that limits the total number of currently served (in-flight) HTTP requests.
// It checks how many requests are in-flight and rejects with 503 if exceeded.
func InFlightLimit(limit int, errDomain string) (func(next http.Handler) http.Handler, error) {
	return InFlightLimitWithOpts(limit, errDomain, InFlightLimitOpts{})
}

// MustInFlightLimit is a version of InFlightLimit that panics on error.
func MustInFlightLimit(limit int, errDomain string) func(next http.Handler) http.Handler {
	mw, err := InFlightLimit(limit, errDomain)
	if err != nil {
		panic(err)
	}
	return mw
}

// InFlightLimitWithOpts is a configurable version of a middleware to limit in-flight HTTP requests.
func InFlightLimitWithOpts(limit int, errDomain string, opts InFlightLimitOpts) (func(next http.Handler) http.Handler, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit should be positive, got %d", limit)
	}

	backlogLimit := opts.BacklogLimit
	if backlogLimit < 0 {
		return nil, fmt.Errorf("backlog limit should not be negative, got %d", backlogLimit)
	}

	backlogTimeout := opts.BacklogTimeout
	if backlogTimeout == 0 {
		backlogTimeout = DefaultInFlightLimitBacklogTimeout
	}

	maxKeys := 0
	if opts.GetKey != nil {
		maxKeys = opts.MaxKeys
		if maxKeys == 0 {
			maxKeys = DefaultInFlightLimitMaxKeys
		}
	}

	getSlots, err := makeInFlightLimitSlotsProvider(limit, backlogLimit, maxKeys)
	if err != nil {
		return nil, fmt.Errorf("make in-flight limit slots provider: %w", err)
	}

	respStatusCode := opts.ResponseStatusCode
	if respStatusCode == 0 {
		respStatusCode = http.StatusServiceUnavailable
	}

	return func(next http.Handler) http.Handler {
		return &inFlightLimitHandler{
			next:           next,
			getKey:         opts.GetKey,
			getSlots:       getSlots,
			backlogTimeout: backlogTimeout,
			errDomain:      errDomain,
			respStatusCode: respStatusCode,
			getRetryAfter:  opts.GetRetryAfter,
			dryRun:         opts.DryRun,
			onReject:       makeInFlightLimitOnRejectFunc(opts),
			onError:        makeInFlightLimitOnErrorFunc(opts),
		}
	}, nil
}

// MustInFlightLimitWithOpts is a version of InFlightLimitWithOpts that panics on error.
func MustInFlightLimitWithOpts(limit int, errDomain string, opts InFlightLimitOpts) func(next http.Handler) http.Handler {
	mw, err := InFlightLimitWithOpts(limit, errDomain, opts)
	if err != nil {
		panic(err)
	}
	return mw
}

func (h *inFlightLimitHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var key string
	if h.getKey != nil {
		var bypass bool
		var err error
		if key, bypass, err = h.getKey(r); err != nil {
			h.onError(rw, r, h.makeParams("", false), fmt.Errorf("get key for in-flight limit: %w", err),
				h.next, GetLoggerFromContext(r.Context()))
			return
		}
		if bypass { // In-flight limiting is bypassed for this request.
			h.next.ServeHTTP(rw, r)
			return
		}
	}

	slots, backlogSlots := h.getSlots(key)

	backlogged := false
	defer func() {
		if backlogged {
			select {
			case <-backlogSlots:
			default:
			}
		}
	}()

	select {
	case backlogSlots <- struct{}{}:
		backlogged = true
		h.serveBackloggedRequest(rw, r, key, slots)
	default:
		h.onReject(rw, r, h.makeParams(key, false), h.next, GetLoggerFromContext(r.Context()))
	}
}

func (h *inFlightLimitHandler) serveBackloggedRequest(
	rw http.ResponseWriter, r *http.Request, key string, slots chan struct{},
) {
	acquired := false
	defer func() {
		if acquired {
			select {
			case <-slots:
			default:
			}
		}
	}()

	if h.dryRun {
		// In dry-run mode we must not affect incoming HTTP request.
		// I.e. we don't need to wait for the backlog timeout and do other things that may slow down request handling.
		select {
		case slots <- struct{}{}:
			acquired = true
			h.next.ServeHTTP(rw, r)
		default:
			h.onReject(rw, r, h.makeParams(key, true), h.next, GetLoggerFromContext(r.Context()))
		}
		return
	}

	backlogTimeoutTimer := time.NewTimer(h.backlogTimeout)
	defer backlogTimeoutTimer.Stop()

	select {
	case slots <- struct{}{}:
		acquired = true
		h.next.ServeHTTP(rw, r)
	case <-backlogTimeoutTimer.C:
		h.onReject(rw, r, h.makeParams(key, true), h.next, GetLoggerFromContext(r.Context()))
	case <-r.Context().Done():
		h.onError(rw, r, h.makeParams(key, true), r.Context().Err(), h.next, GetLoggerFromContext(r.Context()))
	}
}

func (h *inFlightLimitHandler) makeParams(key string, backlogged bool) InFlightLimitParams {
	return InFlightLimitParams{
		ErrDomain:          h.errDomain,
		ResponseStatusCode: h.respStatusCode,
		GetRetryAfter:      h.getRetryAfter,
		Key:                key,
		RequestBacklogged:  backlogged,
	}
}

// DefaultInFlightLimitOnReject sends HTTP response in a typical go-appkit way when the in-flight limit is exceeded.
func DefaultInFlightLimitOnReject(
	rw http.ResponseWriter, r *http.Request, params InFlightLimitParams, next http.Handler, logger log.FieldLogger,
) {
	if logger != nil {
		logger = logger.With(
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Bool(InFlightLimitLogFieldBacklogged, params.RequestBacklogged),
			log.String(userAgentLogFieldKey, r.UserAgent()),
		)
	}
	if params.GetRetryAfter != nil {
		rw.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(params.GetRetryAfter(r).Seconds()))))
	}
	apiErr := restapi.NewError(params.ErrDomain, InFlightLimitErrCode, "Too many in-flight requests.")
	restapi.RespondError(rw, params.ResponseStatusCode, apiErr, logger)
}

// DefaultInFlightLimitOnRejectInDryRun sends HTTP response in a typical go-appkit way
// when the in-flight limit is exceeded in the dry-run mode.
func DefaultInFlightLimitOnRejectInDryRun(
	rw http.ResponseWriter, r *http.Request, params InFlightLimitParams, next http.Handler, logger log.FieldLogger,
) {
	if logger != nil {
		logger.Warn("too many in-flight requests, serving will be continued because of dry run mode",
			log.String(InFlightLimitLogFieldKey, params.Key),
			log.Bool(InFlightLimitLogFieldBacklogged, params.RequestBacklogged),
			log.String(userAgentLogFieldKey, r.UserAgent()),
		)
	}
	next.ServeHTTP(rw, r)
}

// DefaultInFlightLimitOnError sends HTTP response in a typical go-appkit way in case
// when the error occurs during the in-flight limiting.
func DefaultInFlightLimitOnError(
	rw http.ResponseWriter, r *http.Request, params InFlightLimitParams, err error, next http.Handler, logger log.FieldLogger,
) {
	if logger != nil {
		logger.Error(err.Error(), log.String(InFlightLimitLogFieldKey, params.Key))
	}
	restapi.RespondInternalError(rw, params.ErrDomain, logger)
}

func makeInFlightLimitSlotsProvider(limit, backlogLimit, maxKeys int) (func(key string) (chan struct{}, chan struct{}), error) {
	if maxKeys == 0 {
		slots := make(chan struct{}, limit)
		backlogSlots := make(chan struct{}, limit+backlogLimit)
		return func(key string) (chan struct{}, chan struct{}) {
			return slots, backlogSlots
		}, nil
	}

	type KeysZoneItem struct {
		slots        chan struct{}
		backlogSlots chan struct{}
	}

	keysZone, err := lrucache.New[string, KeysZoneItem](maxKeys, nil)
	if err != nil {
		return nil, fmt.Errorf("new LRU in-memory store for keys: %w", err)
	}
	return func(key string) (chan struct{}, chan struct{}) {
		keysZoneItem, _ := keysZone.GetOrAdd(key, func() KeysZoneItem {
			return KeysZoneItem{
				slots:        make(chan struct{}, limit),
				backlogSlots: make(chan struct{}, limit+backlogLimit),
			}
		})
		return keysZoneItem.slots, keysZoneItem.backlogSlots
	}, nil
}

func makeInFlightLimitOnRejectFunc(opts InFlightLimitOpts) InFlightLimitOnRejectFunc {
	if opts.DryRun {
		if opts.OnRejectInDryRun != nil {
			return opts.OnRejectInDryRun
		}
		return DefaultInFlightLimitOnRejectInDryRun
	}
	if opts.OnReject != nil {
		return opts.OnReject
	}
	return DefaultInFlightLimitOnReject
}

func makeInFlightLimitOnErrorFunc(opts InFlightLimitOpts) InFlightLimitOnErrorFunc {
	if opts.OnError != nil {
		return opts.OnError
	}
	return DefaultInFlightLimitOnError
}

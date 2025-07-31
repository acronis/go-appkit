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

	"github.com/acronis/go-appkit/internal/inflightlimit"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/restapi"
)

// DefaultInFlightLimitMaxKeys is a default value of maximum keys number for the InFlightLimit middleware.
const DefaultInFlightLimitMaxKeys = 10000

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
	processor      *inflightlimit.RequestProcessor
	next           http.Handler
	getKey         InFlightLimitGetKeyFunc
	errDomain      string
	respStatusCode int
	getRetryAfter  InFlightLimitGetRetryAfterFunc

	onReject         InFlightLimitOnRejectFunc
	onRejectInDryRun InFlightLimitOnRejectFunc
	onError          InFlightLimitOnErrorFunc
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
	backlogTimeout := opts.BacklogTimeout
	if backlogTimeout == 0 {
		backlogTimeout = inflightlimit.DefaultInFlightLimitBacklogTimeout
	}

	maxKeys := 0
	if opts.GetKey != nil {
		maxKeys = opts.MaxKeys
		if maxKeys == 0 {
			maxKeys = DefaultInFlightLimitMaxKeys
		}
	}

	backlogParams := inflightlimit.BacklogParams{
		MaxKeys: maxKeys,
		Limit:   opts.BacklogLimit,
		Timeout: backlogTimeout,
	}

	processor, err := inflightlimit.NewRequestProcessor(limit, backlogParams, opts.DryRun)
	if err != nil {
		return nil, fmt.Errorf("create request processor: %w", err)
	}

	respStatusCode := opts.ResponseStatusCode
	if respStatusCode == 0 {
		respStatusCode = http.StatusServiceUnavailable
	}

	return func(next http.Handler) http.Handler {
		return &inFlightLimitHandler{
			processor:        processor,
			next:             next,
			getKey:           opts.GetKey,
			errDomain:        errDomain,
			respStatusCode:   respStatusCode,
			getRetryAfter:    opts.GetRetryAfter,
			onReject:         makeInFlightLimitOnRejectFunc(opts),
			onRejectInDryRun: makeInFlightLimitOnRejectInDryRunFunc(opts),
			onError:          makeInFlightLimitOnErrorFunc(opts),
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

// inFlightLimitRequestHandler implements inflightlimit.RequestHandler for HTTP requests.
type inFlightLimitRequestHandler struct {
	rw     http.ResponseWriter
	r      *http.Request
	parent *inFlightLimitHandler
}

func (rh *inFlightLimitRequestHandler) GetContext() context.Context {
	return rh.r.Context()
}

func (rh *inFlightLimitRequestHandler) GetKey() (key string, bypass bool, err error) {
	if rh.parent.getKey != nil {
		return rh.parent.getKey(rh.r)
	}
	return "", false, nil
}

func (rh *inFlightLimitRequestHandler) Execute() error {
	rh.parent.next.ServeHTTP(rh.rw, rh.r)
	return nil
}

func (rh *inFlightLimitRequestHandler) OnReject(params inflightlimit.Params) error {
	rh.parent.onReject(rh.rw, rh.r, rh.convertParams(params), rh.parent.next, GetLoggerFromContext(rh.r.Context()))
	return nil
}

func (rh *inFlightLimitRequestHandler) OnRejectInDryRun(params inflightlimit.Params) error {
	rh.parent.onRejectInDryRun(rh.rw, rh.r, rh.convertParams(params), rh.parent.next, GetLoggerFromContext(rh.r.Context()))
	return nil
}

func (rh *inFlightLimitRequestHandler) OnError(params inflightlimit.Params, err error) error {
	rh.parent.onError(rh.rw, rh.r, rh.convertParams(params), err, rh.parent.next, GetLoggerFromContext(rh.r.Context()))
	return nil
}

func (rh *inFlightLimitRequestHandler) convertParams(params inflightlimit.Params) InFlightLimitParams {
	return InFlightLimitParams{
		ResponseStatusCode: rh.parent.respStatusCode,
		GetRetryAfter:      rh.parent.getRetryAfter,
		ErrDomain:          rh.parent.errDomain,
		Key:                params.Key,
		RequestBacklogged:  params.RequestBacklogged,
	}
}

func (h *inFlightLimitHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	requestHandler := &inFlightLimitRequestHandler{rw: rw, r: r, parent: h}
	_ = h.processor.ProcessRequest(requestHandler) // Error is always nil, as it is handled in the inFlightLimitRequestHandler methods.
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

func makeInFlightLimitOnRejectFunc(opts InFlightLimitOpts) InFlightLimitOnRejectFunc {
	if opts.OnReject != nil {
		return opts.OnReject
	}
	return DefaultInFlightLimitOnReject
}

func makeInFlightLimitOnRejectInDryRunFunc(opts InFlightLimitOpts) InFlightLimitOnRejectFunc {
	if opts.OnRejectInDryRun != nil {
		return opts.OnRejectInDryRun
	}
	return DefaultInFlightLimitOnRejectInDryRun
}

func makeInFlightLimitOnErrorFunc(opts InFlightLimitOpts) InFlightLimitOnErrorFunc {
	if opts.OnError != nil {
		return opts.OnError
	}
	return DefaultInFlightLimitOnError
}

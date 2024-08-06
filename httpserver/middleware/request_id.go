/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"

	"github.com/rs/xid"
)

const (
	headerRequestID         = "X-Request-ID"
	headerInternalRequestID = "X-Int-Request-ID"
)

// RequestIDOpts represents an options for RequestID middleware.
type RequestIDOpts struct {
	GenerateID         func() string
	GenerateInternalID func() string
}

type requestIDHandler struct {
	next http.Handler
	opts RequestIDOpts
}

func newID() string {
	return xid.New().String()
}

// RequestID is a middleware that reads value of X-Request-ID request's HTTP header and generates new one if it's empty.
// Also, the middleware generates yet another id which may be used for internal purposes.
// The first id is named external request id, the second one - internal request id.
// Both these ids are put into request's context and returned in HTTP response in X-Request-ID and X-Int-Request-ID headers.
// It's using xid (based on Mongo Object ID algorithm). This ID generator has high performance with pretty enough entropy.
func RequestID() func(next http.Handler) http.Handler {
	return RequestIDWithOpts(RequestIDOpts{
		GenerateID:         newID,
		GenerateInternalID: newID,
	})
}

// RequestIDWithOpts is a more configurable version of RequestID middleware.
func RequestIDWithOpts(opts RequestIDOpts) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &requestIDHandler{next: next, opts: opts}
	}
}

func (h *requestIDHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	requestID := r.Header.Get(headerRequestID)
	if requestID == "" {
		requestID = h.opts.GenerateID()
	}
	ctx = NewContextWithRequestID(ctx, requestID)
	rw.Header().Set(headerRequestID, requestID)

	internalRequestID := h.opts.GenerateInternalID()
	ctx = NewContextWithInternalRequestID(ctx, internalRequestID)
	rw.Header().Set(headerInternalRequestID, internalRequestID)

	h.next.ServeHTTP(rw, r.WithContext(ctx))
}

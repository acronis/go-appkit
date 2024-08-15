/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/restapi"
)

// RecoveryDefaultStackSize defines the default size of stack part which will be logged.
const RecoveryDefaultStackSize = 8192

// RecoveryOpts represents an options for Recovery middleware.
type RecoveryOpts struct {
	StackSize int
}

type recoveryHandler struct {
	next        http.Handler
	errorDomain string
	opts        RecoveryOpts
}

// Recovery is a middleware that recovers from panics, logs the panic value and a stacktrace,
// returns 500 HTTP status code and error in body in right format.
func Recovery(errDomain string) func(next http.Handler) http.Handler {
	return RecoveryWithOpts(errDomain, RecoveryOpts{StackSize: RecoveryDefaultStackSize})
}

// RecoveryWithOpts is a more configurable version of Recovery middleware.
func RecoveryWithOpts(errDomain string, opts RecoveryOpts) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &recoveryHandler{next: next, errorDomain: errDomain, opts: opts}
	}
}

func (h *recoveryHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	defer func() {
		if p := recover(); p != nil {
			logger := GetLoggerFromContext(r.Context())

			if p == http.ErrAbortHandler {
				// nolint:lll
				// ErrAbortHandler is a sentinel panic for aborting a handler.
				// Stack trace is not logged in http.Server
				// (https://github.com/golang/go/blob/a0d6420d8be2ae7164797051ec74fa2a2df466a1/src/net/http/server.go#L1761-L1775)
				// and should not be logged here.
				// ErrAbortHandler is used by httputil.ReverseProxy in case of error on response copy
				// (https://github.com/golang/go/blob/c33153f7b416c03983324b3e8f869ce1116d84bc/src/net/http/httputil/reverseproxy.go#L284).
				// It's a common practice to continue panic propagation in this case:
				// https://github.com/labstack/echo/blob/584cb85a6b749846ac26a8cd151244ab281f2abc/middleware/recover.go#L89
				// https://github.com/go-chi/chi/blob/58ca6d6119ed77f8c1d564bac109fc36db10e3d0/middleware/recoverer.go#L29
				if logger != nil {
					logger.Warn("request has been aborted", log.Error(http.ErrAbortHandler))
				}
				panic(p)
			}

			if logger != nil {
				var logFields []log.Field
				if h.opts.StackSize != 0 {
					stack := make([]byte, h.opts.StackSize)
					stack = stack[:runtime.Stack(stack, false)]
					logFields = append(logFields, log.Bytes("stack", stack))
				}
				logger.Error(fmt.Sprintf("Panic: %+v", p), logFields...)
			}

			restapi.RespondError(rw, http.StatusInternalServerError, restapi.NewInternalError(h.errorDomain), logger)
		}
	}()

	h.next.ServeHTTP(rw, r)
}

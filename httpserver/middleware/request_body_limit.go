/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"

	"github.com/acronis/go-appkit/restapi"
)

type requestBodyLimitHandler struct {
	next         http.Handler
	maxSizeBytes uint64
	errorDomain  string
}

// RequestBodyLimit is a middleware that sets the maximum allowed size for a request body.
// The body limit is determined based on both Content-Length request header and actual content read.
// Such limiting helps to prevent the server resources being wasted if a malicious client sends a very large request body.
func RequestBodyLimit(maxSizeBytes uint64, errDomain string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return &requestBodyLimitHandler{next, maxSizeBytes, errDomain}
	}
}

func (h *requestBodyLimitHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if r.ContentLength > int64(h.maxSizeBytes) { //nolint:gosec // maxSizeBytes is a reasonable value
		reqErr := restapi.NewTooLargeMalformedRequestError(h.maxSizeBytes)
		restapi.RespondMalformedRequestError(rw, h.errorDomain, reqErr, GetLoggerFromContext(r.Context()))
		return
	}

	restapi.SetRequestMaxBodySize(rw, r, h.maxSizeBytes)

	h.next.ServeHTTP(rw, r)
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"
)

// RoutePatternGetterFunc is a function for getting route pattern from the request. Used in multiple middlewares.
//
// Usually it depends on the router that is used in HTTP server:
//
//	func getGorillaMuxRoutePattern(r *http.Request) string {
//		curRoute := mux.CurrentRoute(r)
//		if curRoute == nil {
//			return ""
//		}
//		pathTemplate, err := curRoute.GetPathTemplate()
//		if err != nil {
//			return ""
//		}
//		return pathTemplate
//	}
//
//	func getChiRoutePattern(r *http.Request) string {
//		ctxVal := r.Context().Value(chi.RouteCtxKey)
//		if ctxVal == nil {
//			return ""
//		}
//		chiCtx := ctxVal.(*chi.Context)
//		if chiCtx == nil {
//			return ""
//		}
//		return chiCtx.RoutePattern()
//	}
type RoutePatternGetterFunc func(r *http.Request) string

// WrapResponseWriterIfNeeded wraps an http.ResponseWriter (if it is not already wrapped), returning a proxy that allows you to
// hook into various parts of the response process.
func WrapResponseWriterIfNeeded(rw http.ResponseWriter, protoMajor int) WrapResponseWriter {
	if wrw, ok := rw.(WrapResponseWriter); ok {
		return wrw
	}
	return NewWrapResponseWriter(rw, protoMajor)
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import "net/http"

// CloneHTTPRequest creates a shallow copy of the request along with a deep copy of the Headers.
func CloneHTTPRequest(req *http.Request) *http.Request {
	r := new(http.Request)
	*r = *req
	r.Header = CloneHTTPHeader(req.Header)
	return r
}

// CloneHTTPHeader creates a deep copy of an http.Header.
func CloneHTTPHeader(in http.Header) http.Header {
	out := make(http.Header, len(in))
	for key, values := range in {
		newValues := make([]string, len(values))
		copy(newValues, values)
		out[key] = newValues
	}
	return out
}

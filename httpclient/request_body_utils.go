/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/acronis/go-appkit/log"
)

// makeRequestBodyRewindable prepares a request body for potential retries by making it rewindable.
// It returns a function that can be called to rewind the request body to its initial state.
//
// Strategy (in order of preference):
// 1) Use http.Request.GetBody when available (best option, avoids buffering and works for large payloads).
// 2) If the Body implements io.ReadSeeker, remember current offset and Seek back on demand (no buffering).
// 3) As a last resort, buffer the entire body in memory and recreate readers for the initial request and the retry.
//
// WARNING: The fallback buffering approach (3) reads the entire request body into memory and is not suitable for very
// large uploads. For sizeable payloads, callers should provide req.GetBody or a seekable Body to avoid high memory
// usage during the single retry performed by AuthBearerRoundTripper.
func makeRequestBodyRewindable(req *http.Request) (func(*http.Request) error, error) {
	// 1) Use GetBody when provided by the caller. It allows us to obtain a fresh reader
	// for the initial request and all subsequent retries without buffering.
	if req.GetBody != nil {
		// Reset the initial body via GetBody to ensure the first attempt reads from a fresh reader.
		initialBody, err := req.GetBody()
		if err != nil {
			return nil, fmt.Errorf("get body before doing first request: %w", err)
		}
		req.Body = initialBody
		return func(r *http.Request) error {
			newBody, newBodyErr := r.GetBody()
			if newBodyErr != nil {
				return fmt.Errorf("get body for retry: %w", newBodyErr)
			}
			r.Body = newBody
			return nil
		}, nil
	}

	// 2) If the body is seekable, remember the current offset and rewind on demand.
	if reqBodySeeker, ok := req.Body.(io.ReadSeeker); ok {
		reqBodySeekOffset, err := reqBodySeeker.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("seek request body before doing first request: %w", err)
		}
		req.Body = io.NopCloser(req.Body)
		return func(r *http.Request) error {
			if _, seekErr := reqBodySeeker.Seek(reqBodySeekOffset, io.SeekStart); seekErr != nil {
				return fmt.Errorf(
					"seek request body (offset=%d, whence=%d) for retry: %w", reqBodySeekOffset, io.SeekStart, seekErr)
			}
			return nil
		}, nil
	}

	// 3) Fallback: buffer the entire body in memory to recreate readers for retries.
	bufferedReqBody, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read all request body before doing first request: %w", err)
	}
	// Set the initial buffered body for the first request
	req.Body = io.NopCloser(bytes.NewReader(bufferedReqBody))
	return func(r *http.Request) error {
		r.Body = io.NopCloser(bytes.NewReader(bufferedReqBody))
		return nil
	}, nil
}

// drainResponseBody reads and discards the entire response body to allow connection reuse.
func drainResponseBody(resp *http.Response, logger log.FieldLogger) {
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Error("failed to close previous response body between retry attempts", log.Error(closeErr))
		}
	}()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		logger.Error("failed to discard previous response body between retry attempts", log.Error(err))
	}
}

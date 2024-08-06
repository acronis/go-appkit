/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"code.cloudfoundry.org/bytefmt"
)

// RequestBodyTooLargeError represents an error that occurs
// when read number of bytes (HTTP request body) exceeds the specified limit.
type RequestBodyTooLargeError struct {
	MaxSizeBytes uint64
	Err          error
}

// Error returns a string representation of RequestBodyTooLargeError.
func (e *RequestBodyTooLargeError) Error() string {
	return e.Err.Error()
}

type maxBytesReader struct {
	io.ReadCloser
	n uint64
}

func (r *maxBytesReader) Read(p []byte) (n int, err error) {
	n, err = r.ReadCloser.Read(p)

	// If request is too large now Read() method of http.maxBytesReader doesn't return specific error,
	// so we have to check error message directly.
	// There is an open issue regarding turning this: https://github.com/golang/go/issues/30715.
	if err != nil && err.Error() == "http: request body too large" {
		err = &RequestBodyTooLargeError{r.n, err}
	}

	return
}

// SetRequestMaxBodySize wraps request body with a reader which limit the number of bytes to read.
// RequestBodyTooLargeError will be returned when maxSizeBytes is exceeded.
func SetRequestMaxBodySize(w http.ResponseWriter, r *http.Request, maxSizeBytes uint64) {
	r.Body = &maxBytesReader{ReadCloser: http.MaxBytesReader(w, r.Body, int64(maxSizeBytes)), n: maxSizeBytes}
}

// MalformedRequestError is an error that occurs in case of incorrect request.
type MalformedRequestError struct {
	HTTPStatusCode int
	Message        string
}

// Error returns a string representation of MalformedRequestError.
func (e *MalformedRequestError) Error() string {
	return e.Message
}

// NewTooLargeMalformedRequestError creates a new MalformedRequestError for case when request body is too large.
func NewTooLargeMalformedRequestError(maxSizeBytes uint64) *MalformedRequestError {
	return &MalformedRequestError{
		http.StatusRequestEntityTooLarge,
		fmt.Sprintf("Request body must not be larger than %s.", bytefmt.ByteSize(maxSizeBytes)),
	}
}

// DecodeRequestJSONStrict tries to read and validate request fields in body and decode it as JSON.
func DecodeRequestJSONStrict(r *http.Request, dst interface{}, disallowUnknownFields bool) error {
	reqContentType := r.Header.Get("Content-Type")
	if reqContentType != "" {
		contentType, _, err := mime.ParseMediaType(reqContentType)
		if err != nil {
			return &MalformedRequestError{
				http.StatusUnsupportedMediaType,
				fmt.Sprintf("failed to parse Content-Type header for request: %s", err),
			}
		}
		if contentType != ContentTypeAppJSON && contentType != ContentTypeAppSCIMJSON {
			return &MalformedRequestError{
				http.StatusUnsupportedMediaType,
				fmt.Sprintf("Content-Type %q is not supported.", contentType),
			}
		}
	}

	decoder := json.NewDecoder(r.Body)
	if disallowUnknownFields {
		decoder.DisallowUnknownFields()
	}

	return decodeRequest(decoder, dst)
}

// DecodeRequestJSON tries to read request body and decode it as JSON.
func DecodeRequestJSON(r *http.Request, dst interface{}) error {
	return DecodeRequestJSONStrict(r, dst, false)
}

func decodeRequest(decoder *json.Decoder, dst interface{}) error {
	if err := decoder.Decode(&dst); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalTypeErr *json.UnmarshalTypeError
		var tooLargeErr *RequestBodyTooLargeError

		switch {
		case errors.Is(err, io.EOF):
			return &MalformedRequestError{
				http.StatusBadRequest,
				"Request body must not be empty.",
			}

		case errors.Is(err, io.ErrUnexpectedEOF):
			return &MalformedRequestError{
				http.StatusBadRequest,
				"Request body contains badly-formed JSON.",
			}

		case errors.As(err, &syntaxErr):
			return &MalformedRequestError{
				http.StatusBadRequest,
				fmt.Sprintf("Request body contains badly-formed JSON (at position %d).", syntaxErr.Offset),
			}

		case errors.As(err, &unmarshalTypeErr):
			if unmarshalTypeErr.Field != "" {
				return &MalformedRequestError{
					http.StatusBadRequest,
					fmt.Sprintf("Request body contains an invalid value for the %q field (at position %d).",
						unmarshalTypeErr.Field, unmarshalTypeErr.Offset),
				}
			}
			return &MalformedRequestError{
				http.StatusBadRequest,
				fmt.Sprintf("Request body contains an invalid value of type %q for the field of type %s.",
					unmarshalTypeErr.Value, unmarshalTypeErr.Type.String()),
			}

		case errors.As(err, &tooLargeErr):
			return NewTooLargeMalformedRequestError(tooLargeErr.MaxSizeBytes)

		case strings.HasPrefix(err.Error(), "json: unknown field"):
			return &MalformedRequestError{
				http.StatusBadRequest,
				"Payload does not match the scheme",
			}

		default:
			return err
		}
	}

	// Decoder is designed to decode streams of JSON objects, but we need to prevent this behavior.
	if decoder.More() {
		return &MalformedRequestError{
			http.StatusBadRequest,
			"Request body must only contain a single JSON object.",
		}
	}

	return nil
}

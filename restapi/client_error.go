/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"errors"
	"fmt"
	"net/url"
)

// ClientError - error that can be returned in request to client
type ClientError struct {
	Message    string
	Method     string
	URL        *url.URL
	StatusCode int
	Err        error
}

func (e *ClientError) wrap(message string, err error) *ClientError {
	e.Message = message
	e.Err = err
	return e
}

// Error - impelent error interface
func (e *ClientError) Error() string {
	str := fmt.Sprintf("method: [%s] url: [%s] status: [%d] message: %s", e.Method, e.URL, e.StatusCode, e.Message)
	if e.Err != nil {
		str += fmt.Sprintf(" error: %s", e.Err.Error())
	}
	return str
}

// Is - allow check it with errors.Is
func (e *ClientError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// Unwrap - allow check it with errors.As
func (e *ClientError) Unwrap() error {
	return e.Err
}

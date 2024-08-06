/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package restapi

import (
	"net/http"
	"strings"
	"unicode"
)

// Error represents an error details.
type Error struct {
	Domain  string                 `json:"domain"`
	Code    string                 `json:"code"`
	Message string                 `json:"message,omitempty"`
	Context map[string]interface{} `json:"context,omitempty"`
	KbLink  *KBLinkInfo            `json:"kbLink,omitempty"`
	Debug   map[string]interface{} `json:"debug,omitempty"`
}

// KBLinkInfo represents an info about item in Acronis Knowledge Base.
type KBLinkInfo struct {
	LineTag string `json:"lineTag,omitempty"`
	SerCode string `json:"serCode,omitempty"`
	Version string `json:"version,omitempty"`
	Build   string `json:"build,omitempty"`
	Product string `json:"product,omitempty"`
	Os      string `json:"os,omitempty"`
}

// Error codes.
// We are using "var" here because some services may want to use different error codes.
var (
	ErrCodeInternal         = "internalError"
	ErrCodeNotFound         = "notFound"
	ErrCodeMethodNotAllowed = "methodNotAllowed"
)

// Error messages.
// We are using "var" here because some services may want to use different error messages.
var (
	ErrMessageInternal         = "Internal error."
	ErrMessageNotFound         = "Not found."
	ErrMessageMethodNotAllowed = "Method not allowed."
)

// NewError creates a new Error with specified params.
func NewError(domain, code, message string) *Error {
	return &Error{Domain: domain, Code: code, Message: message}
}

// NewInternalError creates a new internal error with specified domain.
func NewInternalError(domain string) *Error {
	return NewError(domain, ErrCodeInternal, ErrMessageInternal)
}

// AddContext adds value to error context.
func (e *Error) AddContext(field string, value interface{}) *Error {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[field] = value
	return e
}

// AddDebug adds value to debug info.
func (e *Error) AddDebug(field string, value interface{}) *Error {
	if e.Debug == nil {
		e.Debug = make(map[string]interface{})
	}
	e.Debug[field] = value
	return e
}

func httpCode2ErrorCode(httpCode int) string {
	if httpCode == http.StatusInternalServerError {
		return ErrCodeInternal
	}
	var builder strings.Builder
	capitalizeNext := false
	for _, char := range http.StatusText(httpCode) {
		if unicode.IsSpace(char) {
			capitalizeNext = true
			continue
		}
		if capitalizeNext {
			builder.WriteRune(unicode.ToTitle(char))
			capitalizeNext = false
			continue
		}
		builder.WriteRune(unicode.ToLower(char))
	}
	return builder.String()
}

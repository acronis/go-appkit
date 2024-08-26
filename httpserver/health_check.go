/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/restapi"
)

// StatusClientClosedRequest is a special HTTP status code used by Nginx to show that the client
// closed the request before the server could send a response
const StatusClientClosedRequest = 499

// HealthCheckComponentName is a type alias for component names. It's used for better readability.
type HealthCheckComponentName = string

// HealthCheckStatus is a resulting status of the health-check.
type HealthCheckStatus int

// Health-check statuses.
const (
	HealthCheckStatusOK HealthCheckStatus = iota
	HealthCheckStatusFail
)

// HealthCheckResult is a type alias for result of health-check operation. It's used for better readability.
type HealthCheckResult = map[HealthCheckComponentName]HealthCheckStatus

// HealthCheck is a type alias for health-check operation. It's used for better readability.
type HealthCheck = func() (HealthCheckResult, error)

// HealthCheckContext is a type alias for health-check operation that has access to the request Context
type HealthCheckContext = func(ctx context.Context) (HealthCheckResult, error)

type healthCheckResponseData struct {
	Components map[string]bool `json:"components"`
}

// HealthCheckHandler implements http.Handler and does health-check of a service.
type HealthCheckHandler struct {
	healthCheckFn HealthCheckContext
}

// NewHealthCheckHandler creates a new http.Handler for doing health-check.
// Passing function will be called inside handler and should return statuses of service's components.
func NewHealthCheckHandler(fn HealthCheck) *HealthCheckHandler {
	if fn == nil {
		fn = func() (HealthCheckResult, error) {
			return HealthCheckResult{}, nil
		}
	}
	return &HealthCheckHandler{func(_ context.Context) (HealthCheckResult, error) {
		return fn()
	}}
}

// NewHealthCheckHandlerContext creates a new http.Handler for doing health-check. It is able to access the request Context.
// Passing function will be called inside handler and should return statuses of service's components.
func NewHealthCheckHandlerContext(fn HealthCheckContext) *HealthCheckHandler {
	if fn == nil {
		fn = func(ctx context.Context) (HealthCheckResult, error) {
			return HealthCheckResult{}, ctx.Err()
		}
	}
	return &HealthCheckHandler{fn}
}

// ServeHTTP serves heath-check HTTP request.
func (h *HealthCheckHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	hcResult, err := h.healthCheckFn(r.Context())
	if err != nil {
		if logger := middleware.GetLoggerFromContext(r.Context()); logger != nil {
			logger.Error("error while checking health", log.Error(err))
		}
		if errors.Is(err, context.Canceled) {
			rw.WriteHeader(StatusClientClosedRequest)
			return
		}
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	hasUnhealthyComponent := false
	respData := healthCheckResponseData{Components: map[string]bool{}}
	for name, status := range hcResult {
		respData.Components[name] = status == HealthCheckStatusOK
		if status == HealthCheckStatusFail {
			hasUnhealthyComponent = true
		}
	}

	if errors.Is(r.Context().Err(), context.Canceled) {
		rw.WriteHeader(StatusClientClosedRequest)
		return
	}

	respStatus := http.StatusOK
	if hasUnhealthyComponent {
		respStatus = http.StatusServiceUnavailable
	}
	restapi.RespondCodeAndJSON(rw, respStatus, respData, middleware.GetLoggerFromContext(r.Context()))
}

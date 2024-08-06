package restapi

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHttpCode2ErrorCode(t *testing.T) {
	tests := []struct {
		httpCode    int
		wantErrCode string
	}{
		{http.StatusInternalServerError, "internalError"},
		{http.StatusNotFound, "notFound"},
		{http.StatusBadRequest, "badRequest"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusMethodNotAllowed, "methodNotAllowed"},
		{http.StatusRequestEntityTooLarge, "requestEntityTooLarge"},
	}

	for _, tt := range tests {
		t.Run(tt.wantErrCode, func(t *testing.T) {
			assert.Equal(t, tt.wantErrCode, httpCode2ErrorCode(tt.httpCode))
		})
	}
}

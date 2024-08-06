/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/log"
)

func TestGetLoggerFromContext(t *testing.T) {
	t.Run("empty logger", func(t *testing.T) {
		require.Nil(t, GetLoggerFromContext(context.Background()))
	})
	t.Run("non empty logger", func(t *testing.T) {
		logger := log.NewDisabledLogger()
		ctx := NewContextWithLogger(context.Background(), logger)
		require.Equal(t, logger, GetLoggerFromContext(ctx))
	})
}

func TestGetRequestIDFromContext(t *testing.T) {
	t.Run("empty external request ID", func(t *testing.T) {
		require.Equal(t, "", GetRequestIDFromContext(context.Background()))
	})
	t.Run("non empty external request ID", func(t *testing.T) {
		const reqID = "external-request-id"
		ctx := NewContextWithRequestID(context.Background(), reqID)
		require.Equal(t, reqID, GetRequestIDFromContext(ctx))
	})
}

func TestGetInternalRequestIDFromContext(t *testing.T) {
	t.Run("empty internal request ID", func(t *testing.T) {
		require.Equal(t, "", GetInternalRequestIDFromContext(context.Background()))
	})
	t.Run("non empty internal request ID", func(t *testing.T) {
		const reqID = "internal-request-id"
		ctx := NewContextWithInternalRequestID(context.Background(), reqID)
		require.Equal(t, reqID, GetInternalRequestIDFromContext(ctx))
	})
}

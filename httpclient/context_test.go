package httpclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestTypeContext(t *testing.T) {
	t.Run("empty request type", func(t *testing.T) {
		require.Equal(t, "", GetRequestTypeFromContext(context.Background()))
	})

	t.Run("non empty request type", func(t *testing.T) {
		const requestType = "client-request-type"
		ctx := NewContextWithRequestType(context.Background(), requestType)
		require.Equal(t, requestType, GetRequestTypeFromContext(ctx))
	})
}

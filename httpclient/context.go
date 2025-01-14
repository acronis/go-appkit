package httpclient

import "context"

type ctxKey int

const (
	ctxKeyRequestType ctxKey = iota
)

func getStringFromContext(ctx context.Context, key ctxKey) string {
	value := ctx.Value(key)
	if value == nil {
		return ""
	}
	return value.(string)
}

// NewContextWithRequestType creates a new context with request type.
func NewContextWithRequestType(ctx context.Context, requestType string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestType, requestType)
}

// GetRequestTypeFromContext extracts request type from the context.
func GetRequestTypeFromContext(ctx context.Context) string {
	return getStringFromContext(ctx, ctxKeyRequestType)
}

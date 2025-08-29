package httpclient

import "context"

type ctxKey int

const (
	ctxKeyRequestType ctxKey = iota
	ctxKeyIdempotentHint
)

func getStringFromContext(ctx context.Context, key ctxKey) string {
	value := ctx.Value(key)
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

// NewContextWithRequestType creates a new context with request type.
func NewContextWithRequestType(ctx context.Context, requestType string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestType, requestType)
}

// GetRequestTypeFromContext extracts request type from the context.
func GetRequestTypeFromContext(ctx context.Context) string {
	return getStringFromContext(ctx, ctxKeyRequestType)
}

// NewContextWithIdempotentHint returns a derived context that carries an "idempotent request" hint.
// When set to true, the request is considered idempotent even if it's not a GET/HEAD/OPTIONS request.
// Currently, this hint is used by RetryableRoundTripper in the DefaultCheckRetry function to decide
// whether it's safe to retry unsafe methods like POST and PATCH on retriable server errors.
func NewContextWithIdempotentHint(ctx context.Context, isIdempotent bool) context.Context {
	return context.WithValue(ctx, ctxKeyIdempotentHint, isIdempotent)
}

// GetIdempotentHintFromContext extracts the "idempotent request" hint from context.
// Returns false when the key is not present. See NewContextWithIdempotentHint for details.
func GetIdempotentHintFromContext(ctx context.Context) bool {
	value := ctx.Value(ctxKeyIdempotentHint)
	if value == nil {
		return false
	}
	b, ok := value.(bool)
	return ok && b
}

# grpcserver/interceptor

[![GoDoc Widget]][GoDoc]

A collection of gRPC server interceptors for logging, metrics collection, panic recovery, request ID handling, rate limiting, and in-flight limiting. These interceptors enhance gRPC services with observability, reliability, and traffic control features.

See complete working example of using these interceptors in the [Echo Service Example](./../examples/echo-service).

## Table of Contents

- [Available Interceptors](#available-interceptors)
  - [Logging Interceptor](#logging-interceptor)
  - [Metrics Interceptor](#metrics-interceptor)
  - [Recovery Interceptor](#recovery-interceptor)
  - [Request ID Interceptor](#request-id-interceptor)
  - [Rate Limiting Interceptor](#rate-limiting-interceptor)
  - [In-Flight Limiting Interceptor](#in-flight-limiting-interceptor)
- [Context Utilities](#context-utilities)
  - [Call Start Time](#call-start-time)
  - [Request IDs](#request-ids)
  - [Trace ID](#trace-id)
  - [Logger](#logger)
  - [Logging Parameters](#logging-parameters)
- [Stream Utilities](#stream-utilities)
  - [WrappedServerStream](#wrappedserverstream)
- [Integration Example](#integration-example)
- [Best Practices](#best-practices)

## Available Interceptors

### Logging Interceptor

Provides comprehensive logging for gRPC calls with configurable options.

**Features:**
- Call start and finish logging
- Slow call detection
- Custom header logging
- Method exclusion
- Custom logger providers
- Request ID and trace ID integration

**Usage:**
```go
import "github.com/acronis/go-appkit/grpcserver/interceptor"

// Basic usage
unaryInterceptor := interceptor.LoggingUnaryInterceptor(logger)
streamInterceptor := interceptor.LoggingStreamInterceptor(logger)

// With options
unaryInterceptor := interceptor.LoggingUnaryInterceptor(logger,
    interceptor.WithLoggingCallStart(true),
    interceptor.WithLoggingSlowCallThreshold(2*time.Second),
    interceptor.WithLoggingExcludedMethods("/health/check", "/metrics"),
    interceptor.WithLoggingCallHeaders(map[string]string{
        "x-tenant-id": "tenant_id",
        "x-user-id": "user_id",
    }),
)
```

**Configuration Options:**
- `WithLoggingCallStart(bool)`: Enable/disable call start logging
- `WithLoggingSlowCallThreshold(time.Duration)`: Set slow call threshold
- `WithLoggingExcludedMethods(...string)`: Exclude specific methods from logging
- `WithLoggingCallHeaders(map[string]string)`: Log custom headers
- `WithLoggingUnaryCustomLoggerProvider(func)`: Custom logger provider for unary calls
- `WithLoggingStreamCustomLoggerProvider(func)`: Custom logger provider for stream calls

### Metrics Interceptor

Collects Prometheus metrics for gRPC calls.

**Features:**
- Call duration histograms
- In-flight call gauges
- Method and service labels
- User agent type classification
- Custom label support

**Usage:**
```go
// Create metrics collector
metrics := interceptor.NewPrometheusMetrics(
    interceptor.WithPrometheusNamespace("myapp"),
    interceptor.WithPrometheusDurationBuckets([]float64{0.1, 0.5, 1.0, 5.0}),
    interceptor.WithPrometheusConstLabels(prometheus.Labels{"version": "1.0"}),
)

// Register metrics
metrics.MustRegister()

// Create interceptors
unaryInterceptor := interceptor.MetricsUnaryInterceptor(metrics)
streamInterceptor := interceptor.MetricsStreamInterceptor(metrics)

// With user agent type provider
unaryInterceptor := interceptor.MetricsUnaryInterceptor(metrics,
    interceptor.WithMetricsUnaryUserAgentTypeProvider(func(ctx context.Context, info *grpc.UnaryServerInfo) string {
        // Return user agent type based on context
        return "c2c_agent" // or "mobile", "api", etc.
    }),
)
```

**Metrics Collected:**
- `grpc_call_duration_seconds`: Histogram of call durations
- `grpc_calls_in_flight`: Gauge of currently active calls

**Labels:**
- `grpc_service`: Service name
- `grpc_method`: Method name
- `grpc_method_type`: "unary" or "stream"
- `grpc_code`: gRPC status code
- `user_agent_type`: Classification of user agent (if provided)

### Recovery Interceptor

Recovers from panics and returns proper gRPC errors.

**Features:**
- Panic recovery with stack trace logging
- Configurable stack size
- Proper gRPC error responses

**Usage:**
```go
// Basic usage
unaryInterceptor := interceptor.RecoveryUnaryInterceptor()
streamInterceptor := interceptor.RecoveryStreamInterceptor()

// With custom stack size
unaryInterceptor := interceptor.RecoveryUnaryInterceptor(
    interceptor.WithRecoveryStackSize(16384),
)
```

**Configuration Options:**
- `WithRecoveryStackSize(int)`: Set stack trace size for logging (default: 8192)

### Request ID Interceptor

Manages request IDs for call tracing and correlation.

**Features:**
- Extracts request ID from incoming headers
- Generates new request IDs if missing
- Generates internal request IDs
- Sets response headers
- Context integration

**Usage:**
```go
// Basic usage
unaryInterceptor := interceptor.RequestIDUnaryInterceptor()
streamInterceptor := interceptor.RequestIDStreamInterceptor()

// With custom ID generators
unaryInterceptor := interceptor.RequestIDUnaryInterceptor(
    interceptor.WithRequestIDGenerator(func() string {
        return uuid.New().String()
    }),
    interceptor.WithInternalRequestIDGenerator(func() string {
        return fmt.Sprintf("internal-%d", time.Now().UnixNano())
    }),
)
```

**Headers:**
- `x-request-id`: External request ID (extracted or generated)
- `x-int-request-id`: Internal request ID (always generated)

### Rate Limiting Interceptor

Provides comprehensive rate limiting for gRPC calls with multiple algorithms and extensive customization options.

**Features:**
- Multiple rate limiting algorithms (leaky bucket, sliding window)
- Per-client rate limiting with custom key extraction
- Backlog support for queuing requests
- Dry run mode for testing without enforcement
- Customizable callbacks for rejection and error handling
- Retry-After header support
- Comprehensive logging and error handling

**Usage:**
```go
import "github.com/acronis/go-appkit/grpcserver/interceptor"

// Basic usage with 10 requests per second
rate := interceptor.Rate{Count: 10, Duration: time.Second}
unaryInterceptor, err := interceptor.RateLimitUnaryInterceptor(rate)
streamInterceptor, err := interceptor.RateLimitStreamInterceptor(rate)

// With advanced configuration
unaryInterceptor, err := interceptor.RateLimitUnaryInterceptor(rate,
    // Algorithm selection
    interceptor.WithRateLimitAlg(interceptor.RateLimitAlgLeakyBucket),
    interceptor.WithRateLimitMaxBurst(20),
    
    // Per-client rate limiting
    interceptor.WithRateLimitUnaryGetKey(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
        if md, ok := metadata.FromIncomingContext(ctx); ok {
            if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
                return clientIDs[0], false, nil
            }
        }
        return "", true, nil // bypass if no client-id
    }),
    
    // Backlog configuration
    interceptor.WithRateLimitBacklogLimit(100),
    interceptor.WithRateLimitBacklogTimeout(5*time.Second),
    
    // Dry run mode
    interceptor.WithRateLimitDryRun(true),
    
    // Custom callbacks
    interceptor.WithRateLimitUnaryOnReject(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.RateLimitParams) (interface{}, error) {
        // Custom rejection logic
        return nil, status.Error(codes.ResourceExhausted, "Rate limit exceeded: "+params.Key)
    }),
)
```

**Rate Limiting Algorithms:**
- `RateLimitAlgLeakyBucket`: Token bucket algorithm with burst support
- `RateLimitAlgSlidingWindow`: Sliding window algorithm

**Key Extraction Examples:**
```go
// Rate limit by client IP
func rateLimitByIP(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
    if p, ok := peer.FromContext(ctx); ok {
        if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
            return host, false, nil
        }
        return p.Addr.String(), false, nil
    }
    return "", true, nil // bypass if no peer info
}

// Rate limit by user ID from JWT
func rateLimitByUserID(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
    userID := getUserIDFromJWT(ctx)
    if userID != "" {
        return userID, false, nil
    }
    return "", true, nil // bypass for unauthenticated requests
}

// Rate limit by tenant ID from metadata
func rateLimitByTenant(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
    if md, ok := metadata.FromIncomingContext(ctx); ok {
        if tenantIDs := md.Get("x-tenant-id"); len(tenantIDs) > 0 {
            return tenantIDs[0], false, nil
        }
    }
    return "", true, nil // bypass if no tenant
}
```

**Error Responses:**
- Rate limit exceeded: `codes.ResourceExhausted` with "Too many requests" message
- Rate limiting errors: `codes.Internal` with "Internal server error" message
- Custom responses: Configurable via callback functions

**Headers:**
- `retry-after`: Number of seconds to wait before retrying (set automatically)

### In-Flight Limiting Interceptor

Controls the number of concurrent requests being processed, preventing server overload by limiting in-flight requests.

**Features:**
- Maximum concurrent request limiting
- Per-client in-flight limiting with custom key extraction
- Backlog support for queuing requests when limit is reached
- Dry run mode for testing without enforcement
- Customizable callbacks for rejection and error handling
- Retry-After header support
- Comprehensive logging and error handling

**Usage:**
```go
import "github.com/acronis/go-appkit/grpcserver/interceptor"

// Basic usage with maximum 10 concurrent requests
unaryInterceptor, err := interceptor.InFlightLimitUnaryInterceptor(10)
streamInterceptor, err := interceptor.InFlightLimitStreamInterceptor(10)

// With advanced configuration
unaryInterceptor, err := interceptor.InFlightLimitUnaryInterceptor(10,
    // Per-client in-flight limiting
    interceptor.WithInFlightLimitUnaryGetKey(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
        if md, ok := metadata.FromIncomingContext(ctx); ok {
            if clientIDs := md.Get("client-id"); len(clientIDs) > 0 {
                return clientIDs[0], false, nil
            }
        }
        return "", true, nil // bypass if no client-id
    }),
    
    // Backlog configuration
    interceptor.WithInFlightLimitBacklogLimit(50),
    interceptor.WithInFlightLimitBacklogTimeout(30*time.Second),
    
    // Dry run mode
    interceptor.WithInFlightLimitDryRun(true),
    
    // Custom rejection handler
    interceptor.WithInFlightLimitUnaryOnReject(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler, params interceptor.InFlightLimitParams) (interface{}, error) {
        // Custom rejection logic
        return nil, status.Error(codes.ResourceExhausted, "Too many concurrent requests for key: "+params.Key)
    }),
    
    // Retry-After calculation
    interceptor.WithInFlightLimitUnaryGetRetryAfter(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) time.Duration {
        return 30 * time.Second
    }),
)
```

**Key Extraction Examples:**
```go
// In-flight limit by client IP
func inFlightLimitByIP(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
    if p, ok := peer.FromContext(ctx); ok {
        if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
            return host, false, nil
        }
        return p.Addr.String(), false, nil
    }
    return "", true, nil // bypass if no peer info
}

// In-flight limit by user ID from JWT
func inFlightLimitByUserID(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
    userID := getUserIDFromJWT(ctx)
    if userID != "" {
        return userID, false, nil
    }
    return "", true, nil // bypass for unauthenticated requests
}

// In-flight limit by tenant ID from metadata
func inFlightLimitByTenant(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
    if md, ok := metadata.FromIncomingContext(ctx); ok {
        if tenantIDs := md.Get("x-tenant-id"); len(tenantIDs) > 0 {
            return tenantIDs[0], false, nil
        }
    }
    return "", true, nil // bypass if no tenant
}
```

**Configuration Options:**
- `WithInFlightLimitUnaryGetKey(func)`: Extract key from unary requests for per-client limiting
- `WithInFlightLimitStreamGetKey(func)`: Extract key from stream requests for per-client limiting
- `WithInFlightLimitMaxKeys(int)`: Maximum number of keys to track (default: 10000)
- `WithInFlightLimitDryRun(bool)`: Enable dry run mode for testing
- `WithInFlightLimitBacklogLimit(int)`: Number of requests to queue when limit is reached
- `WithInFlightLimitBacklogTimeout(time.Duration)`: Maximum time to wait in backlog
- `WithInFlightLimitUnaryOnReject(func)`: Custom handler for rejected unary requests
- `WithInFlightLimitStreamOnReject(func)`: Custom handler for rejected stream requests
- `WithInFlightLimitUnaryOnRejectInDryRun(func)`: Custom handler for dry run rejections
- `WithInFlightLimitStreamOnRejectInDryRun(func)`: Custom handler for dry run rejections
- `WithInFlightLimitUnaryOnError(func)`: Custom handler for in-flight limiting errors
- `WithInFlightLimitStreamOnError(func)`: Custom handler for in-flight limiting errors
- `WithInFlightLimitUnaryGetRetryAfter(func)`: Calculate retry-after value for unary requests
- `WithInFlightLimitStreamGetRetryAfter(func)`: Calculate retry-after value for stream requests

**Error Responses:**
- In-flight limit exceeded: `codes.ResourceExhausted` with "Too many in-flight requests" message
- In-flight limiting errors: `codes.Internal` with "Internal server error" message
- Custom responses: Configurable via callback functions

**Headers:**
- `retry-after`: Number of seconds to wait before retrying (set automatically)

**Differences from Rate Limiting:**
- **In-flight limiting**: Controls concurrent processing, focuses on server capacity
- **Rate limiting**: Controls request frequency over time, focuses on traffic shaping
- **Use together**: Rate limiting for traffic control, in-flight limiting for overload protection

## Context Utilities

The package provides utilities for working with context values:

### Call Start Time
```go
// Get call start time
startTime := interceptor.GetCallStartTimeFromContext(ctx)

// Set call start time
ctx = interceptor.NewContextWithCallStartTime(ctx, time.Now())
```

### Request IDs
```go
// Get request ID
requestID := interceptor.GetRequestIDFromContext(ctx)

// Get internal request ID
internalRequestID := interceptor.GetInternalRequestIDFromContext(ctx)

// Set request IDs
ctx = interceptor.NewContextWithRequestID(ctx, "req-123")
ctx = interceptor.NewContextWithInternalRequestID(ctx, "int-456")
```

### Trace ID
```go
// Get trace ID (from OpenTelemetry or similar)
traceID := interceptor.GetTraceIDFromContext(ctx)
```

### Logger
```go
// Get logger from context
logger := interceptor.GetLoggerFromContext(ctx)

// Set logger in context
ctx = interceptor.NewContextWithLogger(ctx, logger)
```

### Logging Parameters
```go
// Get logging parameters
params := interceptor.GetLoggingParamsFromContext(ctx)

// Add custom log fields
params.AddFields(log.String("custom", "value"))

// Set logging parameters
ctx = interceptor.NewContextWithLoggingParams(ctx, params)
```

## Stream Utilities

### WrappedServerStream
A utility for wrapping gRPC server streams with custom context:

```go
wrappedStream := &interceptor.WrappedServerStream{
    ServerStream: originalStream,
    Ctx:          customContext,
}
```

## Integration Example

Here's how to use multiple interceptors together:

```go
func createGRPCServer(logger log.FieldLogger) *grpc.Server {
    // Create metrics collector
    metrics := interceptor.NewPrometheusMetrics(
        interceptor.WithPrometheusNamespace("myapp"),
    )
    metrics.MustRegister()
    
    // Create rate limiting interceptor
    rate := interceptor.Rate{Count: 100, Duration: time.Second}
    rateLimitUnary, err := interceptor.RateLimitUnaryInterceptor(rate,
        interceptor.WithRateLimitAlg(interceptor.RateLimitAlgLeakyBucket),
        interceptor.WithRateLimitMaxBurst(100),
        interceptor.WithRateLimitUnaryGetKey(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
            // Rate limit by client IP
            if p, ok := peer.FromContext(ctx); ok {
                if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
                    return host, false, nil
                }
            }
            return "", true, nil
        }),
    )
    if err != nil {
        panic(err)
    }
    
    rateLimitStream, err := interceptor.RateLimitStreamInterceptor(rate,
        interceptor.WithRateLimitAlg(interceptor.RateLimitAlgLeakyBucket),
        interceptor.WithRateLimitMaxBurst(100),
        interceptor.WithRateLimitStreamGetKey(func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
            // Rate limit by client IP
            if p, ok := peer.FromContext(ss.Context()); ok {
                if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
                    return host, false, nil
                }
            }
            return "", true, nil
        }),
    )
    if err != nil {
        panic(err)
    }
    
    // Create in-flight limiting interceptor
    inFlightUnary, err := interceptor.InFlightLimitUnaryInterceptor(50,
        interceptor.WithInFlightLimitUnaryGetKey(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
            // In-flight limit by client IP
            if p, ok := peer.FromContext(ctx); ok {
                if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
                    return host, false, nil
                }
            }
            return "", true, nil
        }),
        interceptor.WithInFlightLimitBacklogLimit(100),
        interceptor.WithInFlightLimitBacklogTimeout(30*time.Second),
    )
    if err != nil {
        panic(err)
    }
    
    inFlightStream, err := interceptor.InFlightLimitStreamInterceptor(50,
        interceptor.WithInFlightLimitStreamGetKey(func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo) (string, bool, error) {
            // In-flight limit by client IP
            if p, ok := peer.FromContext(ss.Context()); ok {
                if host, _, err := net.SplitHostPort(p.Addr.String()); err == nil {
                    return host, false, nil
                }
            }
            return "", true, nil
        }),
        interceptor.WithInFlightLimitBacklogLimit(100),
        interceptor.WithInFlightLimitBacklogTimeout(30*time.Second),
    )
    if err != nil {
        panic(err)
    }
    
    // Create interceptor chain
    unaryInterceptors := []grpc.UnaryServerInterceptor{
        // Set call start time first
        func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
            ctx = interceptor.NewContextWithCallStartTime(ctx, time.Now())
            return handler(ctx, req)
        },
        // Request ID handling
        interceptor.RequestIDUnaryInterceptor(),
        // Logging
        interceptor.LoggingUnaryInterceptor(logger,
            interceptor.WithLoggingCallStart(true),
            interceptor.WithLoggingSlowCallThreshold(time.Second),
        ),
        // Recovery
        interceptor.RecoveryUnaryInterceptor(),
        // Metrics
        interceptor.MetricsUnaryInterceptor(metrics),
        // Rate limiting
        rateLimitUnary,
        // In-flight limiting
        inFlightUnary,
    }
    
    streamInterceptors := []grpc.StreamServerInterceptor{
        // Set call start time first
        func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
            ctx := interceptor.NewContextWithCallStartTime(ss.Context(), time.Now())
            wrappedStream := &interceptor.WrappedServerStream{ServerStream: ss, Ctx: ctx}
            return handler(srv, wrappedStream)
        },
        // Request ID handling
        interceptor.RequestIDStreamInterceptor(),
        // Logging
        interceptor.LoggingStreamInterceptor(logger,
            interceptor.WithLoggingCallStart(true),
            interceptor.WithLoggingSlowCallThreshold(time.Second),
        ),
        // Recovery
        interceptor.RecoveryStreamInterceptor(),
        // Metrics
        interceptor.MetricsStreamInterceptor(metrics),
        // In-flight limiting
        inFlightStream,
        // Rate limiting
        rateLimitStream,
    }
    
    return grpc.NewServer(
        grpc.ChainUnaryInterceptor(unaryInterceptors...),
        grpc.ChainStreamInterceptor(streamInterceptors...),
    )
}
```

## Best Practices

1. **Interceptor Order**: Place interceptors in the correct order:
   - Call start time (first)
   - Request ID
   - Logging
   - Recovery
   - Metrics
   - In-flight limiting
   - Rate limiting
   - Custom interceptors (last)

2. **Rate Limiting**:
   - Use appropriate algorithms: leaky bucket for burst tolerance, sliding window for precise limits
   - Implement per-client rate limiting using IP, user ID, or client ID as keys
   - Use dry run mode to test rate limits in production without enforcement
   - Configure appropriate backlog limits and timeouts based on your service capacity
   - Monitor rate limiting metrics and adjust limits based on traffic patterns

3. **In-Flight Limiting**:
   - Set limits based on your server's actual processing capacity and available resources
   - Use per-client limiting to prevent single clients from consuming all capacity
   - Configure appropriate backlog limits to handle temporary spikes without rejecting requests
   - Use dry run mode to test limits in production before enforcement
   - Combine with rate limiting: rate limiting for traffic shaping, in-flight limiting for overload protection
   - Monitor in-flight counts and adjust limits based on server performance metrics

4. **Performance**: Use method exclusion for high-frequency health checks and metrics endpoints

5. **Security**: Be careful with custom header logging to avoid logging sensitive information

6. **Metrics**: Use appropriate duration buckets for your use case

7. **Error Handling**: Always use the recovery interceptor to prevent panics from crashing the server

8. **Context**: Use the provided context utilities for consistent data access across interceptors

9. **Rate Limiting Key Selection**:
   - Choose keys that provide fair distribution (e.g., client IP, user ID)
   - Avoid keys that could lead to hot spots (e.g., constant values)
   - Consider using multiple levels of rate limiting (global + per-client)
   - Implement bypass logic for internal or privileged clients

[GoDoc]: https://pkg.go.dev/github.com/acronis/go-appkit/grpcserver/interceptor
[GoDoc Widget]: https://godoc.org/github.com/acronis/go-appkit?status.svg
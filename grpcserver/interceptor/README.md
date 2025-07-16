# grpcserver/interceptor

A collection of gRPC interceptors for logging, metrics collection, panic recovery, and request ID handling. These interceptors enhance gRPC services with observability and reliability features.

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
    }
    
    streamInterceptors := []grpc.StreamServerInterceptor{
        // Similar chain for stream interceptors...
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
   - Custom interceptors (last)

2. **Performance**: Use method exclusion for high-frequency health checks and metrics endpoints

3. **Security**: Be careful with custom header logging to avoid logging sensitive information

4. **Metrics**: Use appropriate duration buckets for your use case

5. **Error Handling**: Always use the recovery interceptor to prevent panics from crashing the server

6. **Context**: Use the provided context utilities for consistent data access across interceptors
# gRPC Interceptor for API Throttling

The package provides comprehensive throttling for gRPC services with both rate limiting and in-flight request limiting capabilities. It supports multiple algorithms, flexible configuration, and extensive customization options.

The throttling is implemented as standard gRPC interceptors and can be configured from code or from JSON/YAML configuration files.

Please see [testable examples](#examples) to understand how to configure and use the interceptors.

## Features

### Dual Throttling Types
1. **Rate Limiting**: Controls the frequency of requests over time using leaky bucket or sliding window algorithms
2. **In-Flight Limiting**: Controls the number of concurrent requests being processed

### Key Capabilities
- **Multiple Rate Limiting Algorithms**: Leaky bucket (with burst support) and sliding window
- **Flexible Key Extraction**: Global, per-client IP, per-header value, or custom identity-based throttling
- **Service Method Patterns**: Support for exact matches and wildcard patterns (`/package.Service/*`)
- **Request Backlogging**: Queue requests when limits are reached with configurable timeouts
- **Dry-Run Mode**: Test throttling configurations without enforcement
- **Comprehensive Metrics**: Prometheus metrics for monitoring and alerting
- **Tag-Based Rules**: Apply different throttling rules to different interceptor instances
- **Key Filtering**: Include/exclude specific keys with glob pattern support

## Throttling Configuration

The throttling configuration consists of three main parts:
1. **rateLimitZones**: Each zone defines rate limiting parameters (rate, burst, algorithm, etc.)
2. **inFlightLimitZones**: Each zone defines in-flight limiting parameters (concurrency limit, backlog, etc.)
3. **rules**: Each rule maps gRPC service/method patterns to specific throttling zones

### Basic Usage

```go
import (
    "github.com/acronis/go-appkit/grpcserver/interceptor/throttle"
)

// Load configuration from file
cfg := throttle.NewConfig()
configLoader := config.NewDefaultLoader("throttle")
if err := configLoader.LoadFromFile("throttle-config.yaml", cfg); err != nil {
    panic(err)
}

// Create interceptors
unaryInterceptor, err := throttle.UnaryInterceptor(cfg)
if err != nil {
    panic(err)
}

streamInterceptor, err := throttle.StreamInterceptor(cfg)
if err != nil {
    panic(err)
}

// Use with gRPC server
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(unaryInterceptor),
    grpc.ChainStreamInterceptor(streamInterceptor),
)
```

## Configuration Examples

### Global Rate Limiting

Global rate limiting applies to all traffic from all sources:

```yaml
rateLimitZones:
  global_rate_limit:
    rateLimit: 1000/s
    burstLimit: 2000
    responseRetryAfter: "5s"

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: global_rate_limit
    alias: global_api_limit
```

### Per-Client Rate Limiting by IP Address

```yaml
rateLimitZones:
  per_client_ip:
    rateLimit: 100/s
    burstLimit: 200
    responseRetryAfter: auto
    key:
      type: remote_addr
    maxKeys: 10000

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: per_client_ip
    alias: per_ip_limit
```

### Per-Client Rate Limiting by Custom Identity

```yaml
rateLimitZones:
  per_user:
    rateLimit: 50/s
    burstLimit: 100
    responseRetryAfter: auto
    key:
      type: identity
    maxKeys: 50000

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: per_user
    alias: per_user_limit
```

Usage with custom identity extraction:

```go
unaryInterceptor, err := throttle.UnaryInterceptor(cfg,
    throttle.WithUnaryGetKeyIdentity(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo) (string, bool, error) {
        // Extract user ID from JWT token or metadata
        if md, ok := metadata.FromIncomingContext(ctx); ok {
            if userIDs := md.Get("x-user-id"); len(userIDs) > 0 {
                return userIDs[0], false, nil
            }
        }
        return "", true, nil // bypass if no user ID
    }),
)
```

### Per-Client Rate Limiting by Header Value

```yaml
rateLimitZones:
  per_tenant:
    rateLimit: 200/s
    burstLimit: 400
    responseRetryAfter: auto
    key:
      type: header
      headerName: "x-tenant-id"
      noBypassEmpty: true
    maxKeys: 1000

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: per_tenant
    alias: per_tenant_limit
```

### In-Flight Limiting

Control concurrent request processing:

```yaml
inFlightLimitZones:
  concurrent_limit:
    inFlightLimit: 100
    backlogLimit: 200
    backlogTimeout: 30s
    responseRetryAfter: 60s

rules:
  - serviceMethods:
      - "/myservice.MyService/ExpensiveOperation"
    inFlightLimits:
      - zone: concurrent_limit
    alias: expensive_ops
```

### Combined Rate and In-Flight Limiting

```yaml
rateLimitZones:
  api_rate_limit:
    rateLimit: 1000/s
    burstLimit: 2000
    responseRetryAfter: auto

inFlightLimitZones:
  api_concurrency_limit:
    inFlightLimit: 500
    backlogLimit: 1000
    backlogTimeout: 30s
    responseRetryAfter: 60s

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: api_rate_limit
    inFlightLimits:
      - zone: api_concurrency_limit
    alias: comprehensive_throttling
```

### Sliding Window Rate Limiting

```yaml
rateLimitZones:
  sliding_window_limit:
    alg: sliding_window
    rateLimit: 60/m
    responseRetryAfter: auto
    key:
      type: identity
    maxKeys: 10000

rules:
  - serviceMethods:
      - "/myservice.MyService/LimitedEndpoint"
    rateLimits:
      - zone: sliding_window_limit
    alias: precise_rate_limit
```

### Method Pattern Matching

```yaml
rules:
  # Exact method match
  - serviceMethods:
      - "/myservice.MyService/SpecificMethod"
    rateLimits:
      - zone: specific_limit

  # Service wildcard match
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: service_limit

  # Multiple patterns
  - serviceMethods:
      - "/myservice.MyService/Method1"
      - "/myservice.MyService/Method2"
      - "/otherservice.Service/*"
    rateLimits:
      - zone: combined_limit

  # Exclusion patterns
  - serviceMethods:
      - "/myservice.MyService/*"
    excludedServiceMethods:
      - "/myservice.MyService/HealthCheck"
      - "/myservice.MyService/Metrics"
    rateLimits:
      - zone: api_limit
```

### Key Filtering with Patterns

```yaml
rateLimitZones:
  filtered_clients:
    rateLimit: 10/s
    key:
      type: header
      headerName: "user-agent"
    includedKeys:
      - "BadBot/*"
      - "Crawler*"
    maxKeys: 1000

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: filtered_clients
    alias: block_bad_bots
```

### Dry-Run Mode

Test throttling configurations without enforcement:

```yaml
rateLimitZones:
  test_limit:
    rateLimit: 100/s
    burstLimit: 200
    dryRun: true  # Only log, don't reject
    key:
      type: remote_addr
    maxKeys: 1000

inFlightLimitZones:
  test_concurrency:
    inFlightLimit: 50
    dryRun: true  # Only log, don't reject
    key:
      type: remote_addr
    maxKeys: 1000

rules:
  - serviceMethods:
      - "/myservice.MyService/*"
    rateLimits:
      - zone: test_limit
    inFlightLimits:
      - zone: test_concurrency
    alias: testing_limits
```

## Advanced Usage

### Custom Callbacks

```go
unaryInterceptor, err := throttle.UnaryInterceptor(cfg,
    // Custom rate limit rejection handler
    throttle.WithUnaryRateLimitOnReject(func(
        ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler, params interceptor.RateLimitParams,
    ) (interface{}, error) {
        // Custom rejection logic
        log.Printf("Rate limit exceeded for key: %s, method: %s", params.Key, info.FullMethod)
        return nil, status.Error(codes.ResourceExhausted, "Custom rate limit message")
    }),

    // Custom in-flight limit rejection handler
    throttle.WithUnaryInFlightLimitOnReject(func(
        ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler, params interceptor.InFlightLimitParams,
    ) (interface{}, error) {
        // Custom rejection logic
        log.Printf("In-flight limit exceeded for key: %s, method: %s", params.Key, info.FullMethod)
        return nil, status.Error(codes.ResourceExhausted, "Custom in-flight limit message")
    }),

    // Custom error handler
    throttle.WithUnaryRateLimitOnError(func(
        ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
        handler grpc.UnaryHandler, err error,
    ) (interface{}, error) {
        log.Printf("Rate limiting error for method %s: %v", info.FullMethod, err)
        return nil, status.Error(codes.Internal, "Throttling service temporarily unavailable")
    }),
)
```

### Multiple Interceptor Instances with Tags

```yaml
rules:
  - serviceMethods:
      - "/myservice.MyService/PublicMethod"
    rateLimits:
      - zone: public_rate_limit
    tags: ["public"]

  - serviceMethods:
      - "/myservice.MyService/AuthenticatedMethod"
    rateLimits:
      - zone: authenticated_rate_limit
    tags: ["authenticated"]
```

```go
// Create separate interceptors for different rule sets
publicInterceptor, err := throttle.UnaryInterceptor(cfg,
    throttle.WithUnaryTags([]string{"public"}),
)

authenticatedInterceptor, err := throttle.UnaryInterceptor(cfg,
    throttle.WithUnaryTags([]string{"authenticated"}),
    throttle.WithUnaryGetKeyIdentity(extractUserID),
)

// Use in interceptor chain
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        publicInterceptor,
        authenticateInterceptor, // Your authentication interceptor
        authenticatedInterceptor,
    ),
)
```

### Prometheus Metrics

```go
// Create metrics collector
metrics := throttle.NewPrometheusMetrics(
    throttle.WithPrometheusNamespace("myapp"),
    throttle.WithPrometheusConstLabels(prometheus.Labels{
        "service": "myservice",
        "version": "1.0",
    }),
)

// Register with Prometheus
prometheus.MustRegister(metrics)

// Use with interceptor
unaryInterceptor, err := throttle.UnaryInterceptor(cfg,
    throttle.WithUnaryMetricsCollector(metrics),
)
```

**Collected Metrics:**
- `myapp_grpc_throttle_rate_limit_rejects_total`: Counter of rate limit rejections
- `myapp_grpc_throttle_in_flight_limit_rejects_total`: Counter of in-flight limit rejections

**Labels:**
- `rule`: Rule name or alias
- `dry_run`: Whether the rejection was in dry-run mode
- `backlogged`: Whether the in-flight request was backlogged (in-flight limits only)

## Configuration Reference

### Rate Limit Zone Configuration

```yaml
rateLimitZones:
  zone_name:
    # Algorithm: "leaky_bucket" (default) or "sliding_window"
    alg: leaky_bucket
    
    # Rate limit (required)
    rateLimit: 100/s  # Format: <count>/<duration>
    
    # Burst limit (optional, only for leaky_bucket)
    burstLimit: 200
    
    # Backlog configuration (optional)
    backlogLimit: 100
    backlogTimeout: 30s
    
    # Retry-After header value
    responseRetryAfter: auto  # or specific duration like "5s"
    
    # Key configuration
    key:
      type: identity  # "identity", "header", "remote_addr", or empty string for global throttling
      headerName: "x-client-id"  # Required for "header" type
      noBypassEmpty: true  # Don't bypass empty header values
    
    # Key management
    maxKeys: 10000  # Maximum number of keys to track
    includedKeys: ["pattern1", "pattern2"]  # Only these keys
    excludedKeys: ["pattern3", "pattern4"]  # Exclude these keys
    
    # Testing
    dryRun: false  # Enable dry-run mode
```

### In-Flight Limit Zone Configuration

```yaml
inFlightLimitZones:
  zone_name:
    # In-flight limit (required)
    inFlightLimit: 100
    
    # Backlog configuration (optional)
    backlogLimit: 200
    backlogTimeout: 30s
    
    # Retry-After header value
    responseRetryAfter: 60s

    # Key configuration
    key:
      type: identity  # "identity", "header", "remote_addr", or empty string for global throttling
      headerName: "x-client-id"  # Required for "header" type
      noBypassEmpty: true  # Don't bypass empty header values

    # Key management
    maxKeys: 10000  # Maximum number of keys to track
    includedKeys: ["pattern1", "pattern2"]  # Only these keys
    excludedKeys: ["pattern3", "pattern4"]  # Exclude these keys
    
    # Testing
    dryRun: false
```

### Rule Configuration

```yaml
rules:
  - # Service method patterns (required)
    serviceMethods:
      - "/package.Service/Method"     # Exact match
      - "/package.Service/*"          # Wildcard match
    
    # Excluded methods (optional)
    excludedServiceMethods:
      - "/package.Service/HealthCheck"
    
    # Rate limiting zones to apply
    rateLimits:
      - zone: zone_name
    
    # In-flight limiting zones to apply
    inFlightLimits:
      - zone: zone_name
    
    # Rule identification
    alias: rule_name  # Used in metrics and logs
    
    # Tags for filtering rules
    tags: ["public", "authenticated"]
```

## Rate Limit Value Formats

- `100/s` - 100 requests per second
- `60/m` - 60 requests per minute
- `1000/h` - 1000 requests per hour
- `10000/d` - 10000 requests per day

## Key Types

### `identity`
Extracts keys using custom identity function. Requires `WithUnaryGetKeyIdentity` or `WithStreamGetKeyIdentity` option.

### `header`
Extracts keys from gRPC metadata headers. Requires `headerName` configuration.

### `remote_addr`
Extracts keys from client IP addresses.

## Error Responses

### Rate Limiting
- **Status**: `codes.ResourceExhausted`
- **Message**: "Too many requests"
- **Header**: `retry-after` with seconds to wait

### In-Flight Limiting
- **Status**: `codes.ResourceExhausted`
- **Message**: "Too many in-flight requests"
- **Header**: `retry-after` with seconds to wait

### Throttling Errors
- **Status**: `codes.Internal`
- **Message**: "Internal server error"

## License

Copyright © 2025 Acronis International GmbH.

Licensed under [MIT License](./../../../../LICENSE).
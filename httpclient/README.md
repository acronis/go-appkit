# httpclient

This package provides a collection of HTTP round trippers that can be chained together to enhance HTTP clients with reliability, observability, and traffic control features.

Additionally, it includes a factory function to create HTTP clients with pre-configured round trippers based on YAML/JSON configuration files.

## Table of Contents

- [Available Round Trippers](#available-round-trippers)
  - [Authentication Bearer Round Tripper](#authentication-bearer-round-tripper)
  - [Logging Round Tripper](#logging-round-tripper)
  - [Metrics Round Tripper](#metrics-round-tripper)
  - [Rate Limiting Round Tripper](#rate-limiting-round-tripper)
  - [Request ID Round Tripper](#request-id-round-tripper)
  - [Retryable Round Tripper](#retryable-round-tripper)
  - [User Agent Round Tripper](#user-agent-round-tripper)
- [Client Factory and Configuration](#client-factory-and-configuration)

## Available Round Trippers

### Authentication Bearer Token Round Tripper

Automatically handles Bearer token authentication with token refresh capabilities.

**Features:**
- Automatic token injection
- Token refresh on authentication failures
- Cache invalidation with minimum intervals
- Request body rewinding for retries after token refresh

**Usage:**
```go
// Create auth provider
tokenProvider := httpclient.AuthProviderFunc(func(ctx context.Context, scope ...string) (string, error) {
    // Your token acquisition logic here
    return "your-access-token", nil
})

// Basic usage
transport := httpclient.NewAuthBearerRoundTripper(http.DefaultTransport, tokenProvider)

// With custom options
transport := httpclient.NewAuthBearerRoundTripperWithOpts(
    http.DefaultTransport,
    tokenProvider,
    httpclient.AuthBearerRoundTripperOpts{
        TokenScope: []string{"my-service:read", "my-service:write"},
        MinInvalidationInterval: 15 * time.Minute,
        ShouldRefreshTokenAndRetry: func(ctx context.Context, resp *http.Response) bool {
			// Custom condition to trigger token refresh and retry
        },
    },
)

client := &http.Client{Transport: transport}
```

**Configuration Options:**
- `TokenScope`: OAuth scopes for token requests
- `MinInvalidationInterval`: Minimum time between auth token provider cache invalidations
- `ShouldRefreshTokenAndRetry`: Custom condition for token refresh and retry
- `LoggerProvider`: Context-specific logger function

### Logging Round Tripper

Provides comprehensive HTTP request and response logging.

**Features:**
- Request/response logging
- Slow request detection
- Configurable logging modes (all/failed)
- Client type identification

**Usage:**
```go
// Basic logging
transport := httpclient.NewLoggingRoundTripper(http.DefaultTransport)

// With custom options
transport := httpclient.NewLoggingRoundTripperWithOpts(
    http.DefaultTransport,
    httpclient.LoggingRoundTripperOpts{
        ClientType:           "my-api-client",
        Mode:                 httpclient.LoggingModeAll,
        SlowRequestThreshold: 2 * time.Second,
        LoggerProvider: func(ctx context.Context) log.FieldLogger {
			// Extract logger from your context
        },
    },
)

client := &http.Client{Transport: transport}
```

**Logging Modes:**
- `LoggingModeAll`: Log all requests and responses (default)
- `LoggingModeFailed`: Log only failed or slow requests

**Configuration Options:**
- `ClientType`: Type identifier for the client (used in logs and metrics)
- `Mode`: Logging mode - all requests or only failed/slow requests
- `SlowRequestThreshold`: Duration threshold for identifying slow requests
- `LoggerProvider`: Context-specific logger function

### Metrics Round Tripper

Collects Prometheus metrics for HTTP client requests.

**Features:**
- Request duration histograms
- Request classification
- Client type labeling
- Customizable metrics collection

**Usage:**
```go
// Create metrics collector
collector := httpclient.NewPrometheusMetricsCollector("myapp")
collector.MustRegister()

// Create transport
transport := httpclient.NewMetricsRoundTripperWithOpts(
    http.DefaultTransport,
    collector,
    httpclient.MetricsRoundTripperOpts{
        ClientType: "api-client",
        ClassifyRequest: func(r *http.Request, clientType string) string {
			// Custom request classification logic
        },
    },
)

client := &http.Client{Transport: transport}
```

**Metrics Collected:**
- `http_client_request_duration_seconds`: Histogram of request durations (suitable for Prometheus alerts on response times or codes)

**Labels:**
- `client_type`: Type of client making the request
- `remote_address`: Target server address
- `summary`: Request classification summary
- `status`: HTTP response status code
- `request_type`: Type of request (from context)

**Configuration Options:**
- `ClientType`: Type identifier for the client (included in metrics labels)
- `ClassifyRequest`: Function to classify requests for metrics summary

### User Agent Round Tripper

Manages User-Agent headers in outgoing requests.

**Features:**
- Multiple update strategies
- Conditional User-Agent setting
- Header composition

**Usage:**
```go
// Basic usage - set User-Agent if empty
transport := httpclient.NewUserAgentRoundTripper(http.DefaultTransport, "myapp/1.0")

// With custom strategy
transport := httpclient.NewUserAgentRoundTripperWithOpts(
    http.DefaultTransport,
    "myapp/1.0",
    httpclient.UserAgentRoundTripperOpts{
        UpdateStrategy: httpclient.UserAgentUpdateStrategyAppend,
    },
)

client := &http.Client{Transport: transport}
```

**Configuration Options:**
- `UpdateStrategy`: Strategy for updating the User-Agent header

**Update Strategies:**
- `UserAgentUpdateStrategySetIfEmpty`: Set only if User-Agent is empty (default)
- `UserAgentUpdateStrategyAppend`: Append to existing User-Agent
- `UserAgentUpdateStrategyPrepend`: Prepend to existing User-Agent

### Rate Limiting Round Tripper

Controls outbound request rate to prevent overwhelming target services.

**Features:**
- Token bucket rate limiting
- Adaptive rate limiting based on response headers
- Configurable burst capacity
- Wait timeout control

**Usage:**
```go
// Basic rate limiting - 10 requests per second
transport, err := httpclient.NewRateLimitingRoundTripper(http.DefaultTransport, 10)

// With custom options
transport, err := httpclient.NewRateLimitingRoundTripperWithOpts(
    http.DefaultTransport, 10,
    httpclient.RateLimitingRoundTripperOpts{
        Burst:       20,
        WaitTimeout: 5 * time.Second,
        Adaptation: httpclient.RateLimitingRoundTripperAdaptation{
            ResponseHeaderName: "X-RateLimit-Remaining",
            SlackPercent:       10,
        },
    },
)

client := &http.Client{Transport: transport}
```

**Configuration Options:**
- `Burst`: Allow temporary request bursts (default: 1)
- `WaitTimeout`: Maximum wait time for rate limiting (default: 15s)
- `Adaptation`: Adaptive rate limiting based on response headers

### Request ID Round Tripper

Automatically adds X-Request-ID headers to outgoing requests.

**Features:**
- Automatic request ID generation
- Context-aware request ID extraction
- Request correlation support

**Usage:**
```go
// Basic usage
transport := httpclient.NewRequestIDRoundTripper(http.DefaultTransport)

// With custom provider
transport := httpclient.NewRequestIDRoundTripperWithOpts(
    http.DefaultTransport,
    httpclient.RequestIDRoundTripperOpts{
        RequestIDProvider: func(ctx context.Context) string {
            // Extract request ID from your context
        },
    },
)

client := &http.Client{Transport: transport}
```

**Configuration Options:**
- `RequestIDProvider`: Function to extract request ID from context

### Retryable Round Tripper

Provides automatic retry functionality for failed HTTP requests with configurable backoff policies.

**Features:**
- Configurable retry attempts (default: 10)
- Multiple backoff policies (exponential, constant)
- Retry-After header support
- Idempotent request detection
- Body rewinding for safe retries

**Usage:**
```go
// Basic usage with default settings
transport, err := httpclient.NewRetryableRoundTripper(http.DefaultTransport)

// With custom options
transport, err := httpclient.NewRetryableRoundTripperWithOpts(
    http.DefaultTransport,
    httpclient.RetryableRoundTripperOpts{
        MaxRetryAttempts: 5,
        CheckRetryFunc: func(ctx context.Context, resp *http.Response, err error, attempts int) (bool, error) {
            // Custom retry logic, e.g., retry on 5xx status codes or network errors for idempotent methods, etc.
        },
        BackoffPolicy: retry.PolicyFunc(func() backoff.BackOff {
			// Custom backoff policy
        }),
    },
)

client := &http.Client{Transport: transport}
```

**Configuration:**
- `MaxRetryAttempts`: Maximum number of retry attempts (default: 10)
- `CheckRetryFunc`: Custom retry condition function
- `IgnoreRetryAfter`: Ignore Retry-After response headers
- `BackoffPolicy`: Backoff strategy for retry delays

## Client Factory and Configuration

The package provides factory functions to create HTTP clients with pre-configured round trippers. Configuration can be loaded from YAML files, environment variables, or defined programmatically.

**Configuration can be loaded from:**
- YAML/JSON files using unmarshalling
- YAML/JSON files and/or environment variables using `config.Loader`
- Direct struct initialization

### Creating Client from YAML Configuration

**config.yaml:**
```yaml
httpclient:
  timeout: 30s
  retries:
    enabled: true
    maxAttempts: 5
    policy: exponential
    exponentialBackoff:
      initialInterval: 1s
      multiplier: 2.0
  rateLimits:
    enabled: true
    limit: 100
    burst: 20
    waitTimeout: 5s
  log:
    enabled: true
    mode: all
    slowRequestThreshold: 2s
  metrics:
    enabled: true
```

**Go code:**
```go
import (
    "os"

    "gopkg.in/yaml.v3"

    "github.com/acronis/go-appkit/httpclient"
)

func createClientFromYAML() (*http.Client, error) {
    data, err := os.ReadFile("config.yaml")
    if err != nil {
        return nil, err
    }

    var config struct {
        HTTPClient httpclient.Config `yaml:"httpclient"`
    }
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, err
    }
    
    // Create auth provider
    authProvider := httpclient.AuthProviderFunc(func(ctx context.Context, scope ...string) (string, error) {
        // Your token acquisition logic here
        return "your-access-token", nil
    })
    
    // Create metrics collector
    metricsCollector := httpclient.NewPrometheusMetricsCollector("my_service")
    metricsCollector.MustRegister()
    
    // Create client with loaded configuration
    return httpclient.NewWithOpts(&config.HTTPClient, httpclient.Opts{
        UserAgent:        "my-service/1.0.0",
        ClientType:       "external-api",
        AuthProvider:     authProvider,
        MetricsCollector: metricsCollector,
        LoggerProvider: func(ctx context.Context) log.FieldLogger {
            return middleware.GetLoggerFromContext(ctx)
        },
        RequestIDProvider: func(ctx context.Context) string {
            return middleware.GetRequestIDFromContext(ctx)
        },
    })
}
```
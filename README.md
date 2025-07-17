# Common Go packages for writing applications, services, and tools

[![GoDoc Widget]][GoDoc]

The project includes the following packages:

+ [config](./config) - loading configuration from environment variables, files, and `io.Reader`. YAML and JSON formats are supported out of the box.
+ [grpcserver/interceptor](./grpcserver/interceptor) - collection of gRPC interceptors for logging, metrics collection, panic recovery, request-id tracing, etc.
+ [httpclient](./httpclient) - helpers and a set of `http.RoundTripper` implementations for simplifying typical HTTP client operations (e.g. retries, client-side throttling, setting any header for each request, etc.).
+ [httpserver](./httpserver) - configurable HTTP server (wrapper around `http.Server`) that includes graceful shutdown support, panic recovery, metrics collection, and logging.
+ [httpserver/middleware](./httpserver/middleware) - collection of middlewares for HTTP server (e.g. request logging, metrics collection, panic recovery, in-flight request limiting, rate limiting, request-id tracing, etc.).
+ [httpserver/middleware/throttle](./httpserver/middleware/throttle) - ready-to-use middleware for server-side throttling that can be flexibly configured via JSON or YAML. The package has its own [README](./httpserver/middleware/throttle/README.md), so check it out for more details.
+ [log](./log) - unified interface for structured logging with included configurable adapter for tiny, fast, and memory-efficient (zero-allocation) [logf](https://github.com/ssgreg/logf) logger.
+ [lrucache](./lrucache) - in-memory LRU cache with collecting Prometheus metrics.
+ [netutil](./netutil) - utilities for working with network.
+ [profserver](./profserver) - profiling HTTP server (pprof).
+ [restapi](./restapi) - set of simple functions for doing requests and sending responses in the REST API.
+ [retry](./restapi) - helper functions for doing retryable operations.
+ [service](./service) - ready-to-use primitives for creating services and managing their lifecycle.
+ [testutil](./testutil) - helpers for writing tests.

## Installation

```
go get -u github.com/acronis/go-appkit
```

## Examples

### Simple service that provides HTTP API

The following example demonstrates how to create a simple service with
+ Configuration loading from environment variables and a yaml file.
+ HTTP server with request logging, metrics collection, primitive tracing and versioned API endpoints.
+ Profiling server (pprof) for debugging purposes running on a separate port.
+ Graceful shutdown on SIGTERM and SIGINT signals.

```go
package main

import (
	"fmt"
	golog "log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/httpserver"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/profserver"
	"github.com/acronis/go-appkit/restapi"
	"github.com/acronis/go-appkit/service"
)

func main() {
	if err := runApp(); err != nil {
		golog.Fatal(err)
	}
}

func runApp() error {
	cfg, err := loadConfigFromFile("config.yaml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, closeFn := log.NewLogger(cfg.Log)
	defer closeFn()

	var serviceUnits []service.Unit

	// Create HTTP server that provides /healthz, /metrics, and /api/{service-name}/v{number}/* endpoints.
	httpServer, err := makeHTTPServer(cfg.Server, logger)
	if err != nil {
		return err
	}
	serviceUnits = append(serviceUnits, httpServer)

	if cfg.ProfServer.Enabled {
		// Create HTTP server for profiling (pprof is used under the hood).
		serviceUnits = append(serviceUnits, profserver.New(cfg.ProfServer, logger))
	}

	return service.New(logger, service.NewCompositeUnit(serviceUnits...)).Start()
}

func makeHTTPServer(cfg *httpserver.Config, logger log.FieldLogger) (*httpserver.HTTPServer, error) {
	const errorDomain = "MyService" // Error domain is useful for distinguishing errors from different services (e.g. proxies).

	apiRoutes := map[httpserver.APIVersion]httpserver.APIRoute{
		1: func(router chi.Router) {
			router.Get("/hello", v1HelloHandler())
		},
		2: func(router chi.Router) {
			router.Get("/hi", v2HiHandler(errorDomain))
		},
	}

	opts := httpserver.Opts{
		ServiceNameInURL: "my-service",
		ErrorDomain:      errorDomain,
		APIRoutes:        apiRoutes,
		HealthCheck: func() (httpserver.HealthCheckResult, error) {
			// 503 status code will be returned if any of the components is unhealthy.
			return map[httpserver.HealthCheckComponentName]httpserver.HealthCheckStatus{
				"component-a": httpserver.HealthCheckStatusOK,
				"component-b": httpserver.HealthCheckStatusOK,
			}, nil
		},
	}

	httpServer, err := httpserver.New(cfg, logger, opts)
	if err != nil {
		return nil, err
	}

	// Custom routes can be added using chi.Router directly.
	httpServer.HTTPRouter.Get("/custom-route", customRouteHandler)

	return httpServer, nil
}

func loadConfigFromFile(filePath string) (*AppConfig, error) {
	cfg := NewAppConfig()
	err := config.NewDefaultLoader("my_service").LoadFromFile(filePath, config.DataTypeYAML, cfg)
	return cfg, err
}

func v1HelloHandler() func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		logger := middleware.GetLoggerFromContext(r.Context())
		restapi.RespondJSON(rw, map[string]string{"message": "Hello from v1"}, logger)
	}
}

func v2HiHandler(errorDomain string) func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		logger := middleware.GetLoggerFromContext(r.Context())
		name := r.URL.Query().Get("name")
		if len(name) < 3 {
			apiErr := restapi.NewError(errorDomain, "invalidName", "Name must be at least 3 characters long")
			restapi.RespondError(rw, http.StatusBadRequest, apiErr, middleware.GetLoggerFromContext(r.Context()))
			return
		}
		restapi.RespondJSON(rw, map[string]string{"message": fmt.Sprintf("Hi %s from v2", name)}, logger)
	}
}

func customRouteHandler(rw http.ResponseWriter, r *http.Request) {
	logger := middleware.GetLoggerFromContext(r.Context())
	if _, err := rw.Write([]byte("Content from the custom route")); err != nil {
		logger.Error("error while writing response body", log.Error(err))
	}
}

type AppConfig struct {
	Server     *httpserver.Config
	ProfServer *profserver.Config
	Log        *log.Config
}

func NewAppConfig() *AppConfig {
	return &AppConfig{
		Server:     httpserver.NewConfig(),
		ProfServer: profserver.NewConfig(),
		Log:        log.NewConfig(),
	}
}

func (c *AppConfig) SetProviderDefaults(dp config.DataProvider) {
	config.CallSetProviderDefaultsForFields(c, dp)
}

func (c *AppConfig) Set(dp config.DataProvider) error {
	return config.CallSetForFields(c, dp)
}
```

Configuration file `config.yaml`:

```yaml
server:
  address: ":8888"
  timeouts:
    write: 1m
    read: 15s
    readHeader: 10s
    idle: 1m
    shutdown: 5s
  limits:
    maxBodySize: 1M
  log:
    requestStart: true
profServer:
  enabled: true
  address: ":8889"
log:
  level: info
  format: json
  output: stdout
```

Run the service:

```shell
$ go run main.go
```

Check the service API:

```shell
$ curl -w "\nHTTP code: %{http_code}\n" localhost:8888/api/my-service/v1/hello                                                                                                                                                               [130]
{"message":"Hello"}
HTTP code: 200

$ curl -w "\nHTTP code: %{http_code}\n" 'localhost:8888/api/my-service/v2/hi?name='
{"error":{"domain":"MyService","code":"invalidName","message":"Name must be at least 3 characters long"}}
HTTP code: 400

$ curl -w "\nHTTP code: %{http_code}\n" 'localhost:8888/api/my-service/v2/hi?name=Alice'
{"message":"Hi Alice"}
HTTP code: 200
```

Check the service health and metrics:
```shell

$ curl -w "\nHTTP code: %{http_code}\n" localhost:8888/healthz
{"components":{"component-a":true,"component-b":true}}
HTTP code: 200

$ curl localhost:8888/metrics
...
# HELP http_request_duration_seconds A histogram of the HTTP request durations.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{method="GET",route_pattern="/api/my-service/v1/hello",status_code="200",user_agent_type="http-client",le="0.01"} 1
...
http_request_duration_seconds_bucket{method="GET",route_pattern="/api/my-service/v1/hello",status_code="200",user_agent_type="http-client",le="600"} 1
http_request_duration_seconds_bucket{method="GET",route_pattern="/api/my-service/v1/hello",status_code="200",user_agent_type="http-client",le="+Inf"} 1
http_request_duration_seconds_sum{method="GET",route_pattern="/api/my-service/v1/hello",status_code="200",user_agent_type="http-client"} 0.000184125
http_request_duration_seconds_count{method="GET",route_pattern="/api/my-service/v1/hello",status_code="200",user_agent_type="http-client"} 1
...
http_request_duration_seconds_bucket{method="GET",route_pattern="/api/my-service/v2/hi",status_code="400",user_agent_type="http-client",le="0.01"} 1
...
http_request_duration_seconds_bucket{method="GET",route_pattern="/api/my-service/v2/hi",status_code="400",user_agent_type="http-client",le="600"} 1
http_request_duration_seconds_bucket{method="GET",route_pattern="/api/my-service/v2/hi",status_code="400",user_agent_type="http-client",le="+Inf"} 1
http_request_duration_seconds_sum{method="GET",route_pattern="/api/my-service/v2/hi",status_code="400",user_agent_type="http-client"} 0.000326041
http_request_duration_seconds_count{method="GET",route_pattern="/api/my-service/v2/hi",status_code="400",user_agent_type="http-client"} 1
...
# HELP http_requests_in_flight Current number of HTTP requests being served.
# TYPE http_requests_in_flight gauge
http_requests_in_flight{method="GET",route_pattern="/api/my-service/v1/hello",user_agent_type="http-client"} 0
http_requests_in_flight{method="GET",route_pattern="/api/my-service/v2/hi",user_agent_type="http-client"} 0
...
```

Service logs:
```
{"level":"info","time":"2024-06-04T20:22:01.862351+03:00","msg":"starting application HTTP server...","pid":8455,"address":":8888","write_timeout":"1m0s","read_timeout":"15s","read_header_timeout":"10s","idle_timeout":"1m0s","shutdown_timeout":"
5s"}
{"level":"info","time":"2024-06-04T20:22:01.862516+03:00","msg":"starting profiling HTTP server...","pid":8455,"address":":8889"}
...
{"level":"info","time":"2024-06-04T20:22:08.075376+03:00","msg":"request started","pid":8455,"request_id":"cpfkqg3juspi21pmber0","int_request_id":"cpfkqg3juspi21pmberg","trace_id":"","method":"GET","uri":"/api/my-service/v1/hello","remote_addr":"[::1]:59994","content_length":0,"user_agent":"curl/8.6.0","remote_addr_ip":"::1","remote_addr_port":59994}
{"level":"info","time":"2024-06-04T20:22:08.075518+03:00","msg":"response completed in 0.000s","pid":8455,"request_id":"cpfkqg3juspi21pmber0","int_request_id":"cpfkqg3juspi21pmberg","trace_id":"","method":"GET","uri":"/api/my-service/v1/hello","remote_addr":"[::1]:59994","content_length":0,"user_agent":"curl/8.6.0","remote_addr_ip":"::1","remote_addr_port":59994,"duration_ms":0,"duration":184,"status":200,"bytes_sent":19}
...
{"level":"info","time":"2024-06-04T20:31:14.002993+03:00","msg":"service got signal","pid":8455,"signal":"interrupt"}
{"level":"info","time":"2024-06-04T20:31:14.003051+03:00","msg":"closing profiling HTTP server...","pid":8455}
{"level":"info","time":"2024-06-04T20:31:14.00321+03:00","msg":"profiling HTTP served closed","pid":8455,"address":":8889"}
{"level":"info","time":"2024-06-04T20:31:14.003254+03:00","msg":"shutting down application HTTP server...","pid":8455,"timeout":"5s"}
{"level":"info","time":"2024-06-04T20:31:14.003365+03:00","msg":"application HTTP server closed","pid":8455,"address":":8888","write_timeout":"1m0s","read_timeout":"15s","read_header_timeout":"10s","idle_timeout":"1m0s","shutdown_timeout":"5s"}
{"level":"info","time":"2024-06-04T20:31:14.003412+03:00","msg":"application HTTP server shut down","pid":8455}
```

## License

Copyright © 2024 Acronis International GmbH.

Licensed under [MIT License](./LICENSE).

[GoDoc]: https://pkg.go.dev/github.com/acronis/go-appkit
[GoDoc Widget]: https://godoc.org/github.com/acronis/go-appkit?status.svg

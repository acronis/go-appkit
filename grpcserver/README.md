# grpcserver

[![GoDoc Widget]][GoDoc]

A gRPC server implementation with YAML/JSON based configuration, built-in interceptors, and observability features.

## Features

- **Configuration**: Flexible gRPC server setup with configuration from YAML/JSON files
- **Interceptors**: Built-in interceptors for observability and reliability
- **Metrics**: Prometheus metrics collection

## Usage

```go
package main

import (
    "github.com/acronis/go-appkit/config"
    "github.com/acronis/go-appkit/grpcserver"
    "github.com/acronis/go-appkit/log"
    "github.com/acronis/go-appkit/service"
)

func main () {
    // Load configuration from YAML/JSON file
    cfgLoader := config.NewDefaultLoader("my_service")
    cfg := &AppConfig{
        Server: grpcserver.NewConfig(),
        Log:    log.NewConfig(),
    }
    err := cfgLoader.LoadFromFile("config.yml", config.DataTypeYAML, cfg)

    // Create logger from config
    logger, loggerClose := log.NewLogger(cfg.Log)
    defer loggerClose()

    // Create gRPC server with configuration
    grpcServer, err := grpcserver.New(cfg.Server, logger)
    if err != nil {
        return err
    }

    // Register your services
    pb.RegisterYourServiceServer(grpcServer.GRPCServer, &yourService{})

    // Start server using service micro-framework
    service.New(logger, grpcServer).Start()
}
```

## Documentation

- [Interceptors](interceptor/README.md) - Logging, metrics, recovery, throttling, and request ID interceptors
- [Configurable Throttling](interceptor/throttle/README.md) - Interceptors for gRPC throttling that can be flexible configured from YAML/JSON files
- [Echo Service Example](examples/echo-service/README.md) - Complete working example with testing instructions

## Configuration

The server can be easily configured from JSON/YAML files with options including:
- Network address and port
- TLS configuration
- Timeout settings
- Graceful shutdown parameters

See the examples directory for complete configuration examples.

[GoDoc]: https://pkg.go.dev/github.com/acronis/go-appkit/grpcserver
[GoDoc Widget]: https://godoc.org/github.com/acronis/go-appkit?status.svg
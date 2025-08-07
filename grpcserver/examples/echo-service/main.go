package main

import (
	"context"
	"fmt"
	"io"
	golog "log"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/grpcserver"
	"github.com/acronis/go-appkit/grpcserver/examples/echo-service/pb"
	"github.com/acronis/go-appkit/grpcserver/interceptor/throttle"
	"github.com/acronis/go-appkit/httpserver"
	"github.com/acronis/go-appkit/log"
	"github.com/acronis/go-appkit/service"
)

func main() {
	if err := runApp(); err != nil {
		golog.Fatal(err)
	}
}

func runApp() error {
	cfg, err := loadAppConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Create logger from config
	logger, loggerClose := log.NewLogger(cfg.Log)
	defer loggerClose()

	var serviceUnits []service.Unit

	// Create gRPC server with health check, reflection, and throttling
	grpcServer, err := makeGRPCServer(cfg.Server, cfg.Throttle, logger)
	if err != nil {
		return err
	}
	serviceUnits = append(serviceUnits, grpcServer)

	// Create HTTP server for metrics
	serviceUnits = append(serviceUnits, httpserver.NewWithHandler(cfg.MetricsServer, logger, promhttp.Handler()))

	// Create and start the service
	return service.New(logger, service.NewCompositeUnit(serviceUnits...)).Start()
}

func makeGRPCServer(cfg *grpcserver.Config, throttleCfg *throttle.Config, logger log.FieldLogger) (*grpcserver.GRPCServer, error) {
	// Create and register Prometheus metrics for throttling
	throttleMetrics := throttle.NewPrometheusMetrics()
	throttleMetrics.MustRegister()

	// Create throttling interceptors with metrics collection
	unaryThrottleInterceptor, err := throttle.UnaryInterceptor(throttleCfg,
		throttle.WithUnaryMetricsCollector(throttleMetrics))
	if err != nil {
		return nil, fmt.Errorf("create unary throttle interceptor: %w", err)
	}
	streamThrottleInterceptor, err := throttle.StreamInterceptor(throttleCfg,
		throttle.WithStreamMetricsCollector(throttleMetrics))
	if err != nil {
		return nil, fmt.Errorf("create stream throttle interceptor: %w", err)
	}

	// Create gRPC server with logging, metrics, and throttling
	grpcServer, err := grpcserver.New(cfg, logger,
		grpcserver.WithUnaryInterceptors(unaryThrottleInterceptor),
		grpcserver.WithStreamInterceptors(streamThrottleInterceptor))
	if err != nil {
		return nil, fmt.Errorf("create gRPC server: %w", err)
	}
	pb.RegisterEchoServiceServer(grpcServer.GRPCServer, &EchoService{})

	// Register health check service
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer.GRPCServer, healthServer)

	// Register reflection service for debugging
	reflection.Register(grpcServer.GRPCServer)

	return grpcServer, nil
}

func loadAppConfig() (*AppConfig, error) {
	cfgLoader := config.NewDefaultLoader("my_service")
	cfg := NewAppConfig()
	err := cfgLoader.LoadFromFile("config.yml", config.DataTypeYAML, cfg)
	return cfg, err
}

type AppConfig struct {
	Server        *grpcserver.Config
	Log           *log.Config
	MetricsServer *httpserver.Config
	Throttle      *throttle.Config
}

func NewAppConfig() *AppConfig {
	return &AppConfig{
		Server:        grpcserver.NewConfig(),
		Log:           log.NewConfig(),
		MetricsServer: httpserver.NewConfig(httpserver.WithKeyPrefix("metricsServer")),
		Throttle:      throttle.NewConfig(throttle.WithKeyPrefix("throttle")),
	}
}

func (c *AppConfig) SetProviderDefaults(dp config.DataProvider) {
	config.CallSetProviderDefaultsForFields(c, dp)
}

func (c *AppConfig) Set(dp config.DataProvider) error {
	return config.CallSetForFields(c, dp)
}

type EchoService struct {
	pb.UnimplementedEchoServiceServer
}

func (s *EchoService) Echo(ctx context.Context, req *pb.EchoRequest) (*pb.EchoResponse, error) {
	return &pb.EchoResponse{
		Payload: req.Payload,
	}, nil
}

func (s *EchoService) EchoStream(stream pb.EchoService_EchoStreamServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		resp := &pb.EchoResponse{
			Payload: req.Payload,
		}

		if err = stream.Send(resp); err != nil {
			return err
		}
	}
}

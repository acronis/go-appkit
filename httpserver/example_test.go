/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver_test

import (
	"fmt"
	golog "log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"

	"git.acronis.com/abc/go-libs/v2/config"
	"git.acronis.com/abc/go-libs/v2/httpserver"
	"git.acronis.com/abc/go-libs/v2/httpserver/middleware"
	"git.acronis.com/abc/go-libs/v2/log"
	"git.acronis.com/abc/go-libs/v2/profserver"
	"git.acronis.com/abc/go-libs/v2/restapi"
	"git.acronis.com/abc/go-libs/v2/service"
)

/*
Add "// Output:" in the end of Example() function and run:

	$ go test ./httpserver -v -run Example

Application and pprof servers will be ready to handle HTTP requests:

	$ curl localhost:8888/healthz
	{"components":{"component-a":true,"component-b":false}}

	$ curl localhost:8888/metrics
	# Metrics in Prometheus format

	$ curl localhost:8888/api/my-service/v1/hello
	{"message":"Hello"}

	$ curl 'localhost:8888/api/my-service/v2/hi?name=Alice'
	{"message":"Hi Alice"}
*/

func Example() {
	if err := runApp(); err != nil {
		golog.Fatal(err)
	}
}

func runApp() error {
	cfg, err := loadAppConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, loggerClose := log.NewLogger(cfg.Log)
	defer loggerClose()

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

func loadAppConfig() (*AppConfig, error) {
	// Environment variables may be used to configure the server as well.
	// Variable name is built from the service name and path to the configuration parameter separated by underscores.
	_ = os.Setenv("MY_SERVICE_SERVER_TIMEOUTS_SHUTDOWN", "10s")
	_ = os.Setenv("MY_SERVICE_LOG_LEVEL", "info")

	// Configuration may be read from a file or io.Reader. YAML and JSON formats are supported.
	cfgReader := strings.NewReader(`
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
  level: warn
  format: json
  output: stdout
`)

	cfgLoader := config.NewDefaultLoader("my_service")
	cfg := NewAppConfig()
	err := cfgLoader.LoadFromReader(cfgReader, config.DataTypeYAML, cfg)
	return cfg, err
}

func v1HelloHandler() func(rw http.ResponseWriter, r *http.Request) {
	return func(rw http.ResponseWriter, r *http.Request) {
		logger := middleware.GetLoggerFromContext(r.Context())
		restapi.RespondJSON(rw, map[string]string{"message": "Hello"}, logger)
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
		restapi.RespondJSON(rw, map[string]string{"message": fmt.Sprintf("Hi %s", name)}, logger)
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

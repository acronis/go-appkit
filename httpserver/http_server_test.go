/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/httpserver/middleware"
	"git.acronis.com/abc/go-libs/v2/log"
	"git.acronis.com/abc/go-libs/v2/log/logtest"
	"git.acronis.com/abc/go-libs/v2/restapi"
	"git.acronis.com/abc/go-libs/v2/testutil"
)

func generateCertificate(certFilePath, privKeyPath string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Hosting.com"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Encode and write the certificate to cert.pem
	certOut, err := os.Create(certFilePath)
	if err != nil {
		return fmt.Errorf("failed to create %#q for writing: %w", certFilePath, err)
	}
	defer func() { _ = certOut.Close() }()
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err != nil {
		return fmt.Errorf("failed to write data to %#q: %w", certFilePath, err)
	}

	// Encode and write the private key to key.pem
	keyOut, err := os.Create(privKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create %#q for writing: %w", privKeyPath, err)
	}
	defer func() { _ = keyOut.Close() }()

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	if err != nil {
		return fmt.Errorf("failed to write data to %#q: %w", privKeyPath, err)
	}
	return nil
}

func TestHTTPServer_Start_SecureServer(t *testing.T) {
	var (
		dirName      = "testdata/tls"
		certFilePath = filepath.Join(dirName, "cert.pem")
		privKeyFile  = filepath.Join(dirName, "key.pem")
	)
	addr := testutil.GetLocalAddrWithFreeTCPPort()
	fatalErr := make(chan error, 1)
	err := os.MkdirAll(dirName, 0755)
	require.NoError(t, err)

	err = generateCertificate(certFilePath, privKeyFile)
	require.NoError(t, err)

	cfg := TLSConfig{Enabled: true, Certificate: certFilePath, Key: privKeyFile}
	httpServer, err := New(&Config{Address: addr, TLS: cfg}, logtest.NewLogger(), Opts{})
	require.NoError(t, err)

	go httpServer.Start(fatalErr)
	require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))

	port := httpServer.GetPort()
	require.Greater(t, port, 0)
	require.Equal(t, addr, fmt.Sprintf("127.0.0.1:%d", httpServer.GetPort()))

	defer func() {
		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	client := buildClient(cfg.Certificate)
	resp, err := client.Get(httpServer.URL + "/healthz")
	defer func() { require.NoError(t, resp.Body.Close()) }()

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func buildClient(certPath string) *http.Client {
	// Set up our own certificate pool
	tlsConfig := &tls.Config{RootCAs: x509.NewCertPool()}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: transport}

	// Load our trusted certificate path
	pemData, err := os.ReadFile(certPath)
	if err != nil {
		panic(err)
	}
	ok := tlsConfig.RootCAs.AppendCertsFromPEM(pemData)
	if !ok {
		panic("Couldn't load PEM data")
	}

	return client
}

func TestHTTPServer_StartWithStaticPort(t *testing.T) {
	testHTTPServerStart(t, "127.0.0.1", testutil.GetLocalFreeTCPPort(), "")
}

func TestHTTPServer_StartWithDynamicPort(t *testing.T) {
	testHTTPServerStart(t, "127.0.0.1", 0, "")
}

func TestHTTPServer_Start_UnixSocket(t *testing.T) {
	testHTTPServerStart(t, "", 0, t.TempDir()+"/s.sock")
}

func testHTTPServerStart(t *testing.T, host string, port int, unixSocketPath string) {
	newClient := func(addr string) *http.Client {
		if addr != "" {
			return &http.Client{Timeout: time.Second}
		}
		c := &http.Client{Timeout: time.Second}
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", unixSocketPath)
		}
		c.Transport = tr
		return c
	}

	var serverConfigAddress string
	if host != "" {
		serverConfigAddress = fmt.Sprintf("%s:%d", host, port)
	}
	httpServer, err := New(&Config{Address: serverConfigAddress, UnixSocketPath: unixSocketPath}, logtest.NewLogger(), Opts{})
	require.NoError(t, err)
	fatalErr := make(chan error, 1)
	go httpServer.Start(fatalErr)
	var actualAddr = ""
	var serverURL = ""
	if host != "" {
		actualPort, err := testutil.WaitPortAndListeningServer(host, func() int { return httpServer.GetPort() },
			time.Second*3)
		require.NoError(t, err)
		require.Greater(t, actualPort, 0)
		actualAddr = fmt.Sprintf("%s:%d", host, actualPort)
		if httpServer.TLS.Enabled {
			serverURL = fmt.Sprintf("https://%s", actualAddr)
		} else {
			serverURL = fmt.Sprintf("http://%s", actualAddr)
		}
	} else {
		serverURL = httpServer.URL
		require.NoError(t, testutil.WaitListeningServerWithUnixSocket(unixSocketPath, time.Second*3))
		require.Equal(t, 0, httpServer.GetPort())
	}
	defer func() {
		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	var resp *http.Response
	var respBody []byte

	client := newClient(actualAddr)

	resp, err = client.Get(serverURL + "/metrics")
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, len(respBody) > 0)

	resp, err = client.Get(serverURL + "/healthz")
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	testutil.RequireStringJSONInResponse(t, resp, `{"components":{}}`)
}

func TestHTTPServer_Start_WithAPI(t *testing.T) {
	apiRoutes := map[APIVersion]APIRoute{
		1: func(router chi.Router) {
			router.Get("/hello", func(rw http.ResponseWriter, r *http.Request) {
				logger := middleware.GetLoggerFromContext(r.Context())
				restapi.RespondJSON(rw, map[string]string{"message": "hello from v1"}, logger)
			})
			router.Post("/panic", func(rw http.ResponseWriter, r *http.Request) {
				panic("PANIC!!!")
			})
		},
		2: func(router chi.Router) {
			router.Get("/hello", func(rw http.ResponseWriter, r *http.Request) {
				logger := middleware.GetLoggerFromContext(r.Context())
				restapi.RespondJSON(rw, map[string]string{"message": "hello from v2"}, logger)
			})
		},
	}
	const errDomain = "MyService"
	opts := Opts{ServiceNameInURL: "my-service", ErrorDomain: errDomain, APIRoutes: apiRoutes}

	addr := testutil.GetLocalAddrWithFreeTCPPort()

	httpServer, err := New(&Config{Address: addr}, logtest.NewLogger(), opts)
	require.NoError(t, err)
	fatalErr := make(chan error, 1)
	go httpServer.Start(fatalErr)
	require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))
	defer func() {
		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	var resp *http.Response

	resp, err = http.Get(httpServer.URL + "/api/my-service/v1/hello")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.RequireStringJSONInResponse(t, resp, `{"message":"hello from v1"}`)
	require.NoError(t, resp.Body.Close())

	resp, err = http.Post(httpServer.URL+"/api/my-service/v1/panic", restapi.ContentTypeAppJSON, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	testutil.RequireErrorInResponse(t, resp, http.StatusInternalServerError, errDomain, restapi.ErrCodeInternal)
	require.NoError(t, resp.Body.Close())

	resp, err = http.Post(httpServer.URL+"/api/my-service/v2/hello", restapi.ContentTypeAppJSON, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp, err = http.Get(httpServer.URL + "/api/my-service/v2/hello")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.RequireStringJSONInResponse(t, resp, `{"message":"hello from v2"}`)
	require.NoError(t, resp.Body.Close())
}

func TestHTTPServer_Stop(t *testing.T) {
	apiRoutes := map[APIVersion]APIRoute{
		1: func(router chi.Router) {
			router.Get("/sleep", func(rw http.ResponseWriter, r *http.Request) {
				time.Sleep(time.Second * 1) // Long operation.
				logger := middleware.GetLoggerFromContext(r.Context())
				restapi.RespondJSON(rw, map[string]string{"message": "long operation is finished!"}, logger)
			})
		},
	}
	opts := Opts{ServiceNameInURL: "my-service", ErrorDomain: "", APIRoutes: apiRoutes}

	t.Run("with gracefully shutdown", func(t *testing.T) {
		addr := testutil.GetLocalAddrWithFreeTCPPort()

		httpServer, err := New(&Config{Address: addr, Timeouts: TimeoutsConfig{Shutdown: time.Second * 3}}, logtest.NewLogger(), opts)
		require.NoError(t, err)
		fatalErr := make(chan error, 1)
		go httpServer.Start(fatalErr)
		require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))

		done := make(chan bool, 1)
		go func() {
			defer func() { done <- true }()
			c := http.Client{Timeout: time.Second * 5}
			startedAt := time.Now()
			resp, err := c.Get(httpServer.URL + "/api/my-service/v1/sleep")
			if err == nil {
				defer func() { require.NoError(t, resp.Body.Close()) }()
			}
			require.NoError(t, err,
				"server should wait until all HTTP requests are served and only after this close TCP connection")
			require.WithinDuration(t, startedAt.Add(time.Second), time.Now(), time.Millisecond*100)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			testutil.RequireStringJSONInResponse(t, resp, `{"message":"long operation is finished!"}`)
		}()

		time.Sleep(time.Millisecond * 500) // Give time to send request.

		require.NoError(t, httpServer.Stop(true))
		testutil.RequireNoErrorInChannel(t, fatalErr)

		<-done
	})

	t.Run("w/o gracefully shutdown", func(t *testing.T) {
		addr := testutil.GetLocalAddrWithFreeTCPPort()

		httpServer, err := New(&Config{Address: addr, Timeouts: TimeoutsConfig{Shutdown: time.Second * 3}}, logtest.NewLogger(), opts)
		require.NoError(t, err)
		fatalErr := make(chan error, 1)
		go httpServer.Start(fatalErr)
		require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))

		done := make(chan bool, 1)
		go func() {
			defer func() { done <- true }()
			c := http.Client{Timeout: time.Second * 5}
			startedAt := time.Now()
			resp, err := c.Get(httpServer.URL + "/api/my-service/v1/sleep")
			if err == nil {
				defer func() { require.NoError(t, resp.Body.Close()) }()
			}
			require.WithinDuration(t, startedAt.Add(time.Millisecond*500), time.Now(), time.Millisecond*100)
			require.Error(t, err, "server should close TCP connection immediately")
		}()

		time.Sleep(time.Millisecond * 500) // Give time to send request.

		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)

		<-done
	})
}

func TestHTTPServer_Stop_Without_Start(t *testing.T) {
	apiRoutes := map[APIVersion]APIRoute{
		1: func(router chi.Router) {
			router.Get("/sleep", func(rw http.ResponseWriter, r *http.Request) {
				time.Sleep(time.Second * 1) // Long operation.
				logger := middleware.GetLoggerFromContext(r.Context())
				restapi.RespondJSON(rw, map[string]string{"message": "long operation is finished!"}, logger)
			})
		},
	}
	opts := Opts{ServiceNameInURL: "my-service", ErrorDomain: "", APIRoutes: apiRoutes}

	t.Run("with graceful shutdown", func(t *testing.T) {
		addr := testutil.GetLocalAddrWithFreeTCPPort()
		httpServer, err := New(&Config{Address: addr, Timeouts: TimeoutsConfig{Shutdown: time.Second * 3}}, logtest.NewLogger(), opts)
		require.NoError(t, err)

		require.NoError(t, httpServer.Stop(true))
	})

	t.Run("w/o graceful shutdown", func(t *testing.T) {
		addr := testutil.GetLocalAddrWithFreeTCPPort()
		httpServer, err := New(&Config{Address: addr, Timeouts: TimeoutsConfig{Shutdown: time.Second * 3}}, logtest.NewLogger(), opts)
		require.NoError(t, err)

		require.NoError(t, httpServer.Stop(false))
	})
}

func TestHTTPServer_MetricsHandler(t *testing.T) {
	addr := testutil.GetLocalAddrWithFreeTCPPort()

	wrapperNewValues := []byte("input new values")
	metricWrapper := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write(wrapperNewValues)
			require.NoError(t, err)

			h.ServeHTTP(w, r)
		})
	}

	httpServer, err := New(&Config{Address: addr}, logtest.NewLogger(), Opts{MetricsHandler: metricWrapper(promhttp.Handler())})
	require.NoError(t, err)
	fatalErr := make(chan error, 1)
	go httpServer.Start(fatalErr)
	require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))
	defer func() {
		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	var resp *http.Response
	var respBody []byte

	resp, err = http.Get(httpServer.URL + "/metrics")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	respBody, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.True(t, bytes.Contains(respBody, wrapperNewValues))

	resp, err = http.Get(httpServer.URL + "/healthz")
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)
	testutil.RequireStringJSONInResponse(t, resp, `{"components":{}}`)
	require.NoError(t, resp.Body.Close())
}

func TestHTTPServer_Logging(t *testing.T) {
	apiRoutes := map[APIVersion]APIRoute{
		1: func(router chi.Router) {
			router.Get("/hello", func(rw http.ResponseWriter, r *http.Request) {
				logger := middleware.GetLoggerFromContext(r.Context())
				restapi.RespondJSON(rw, map[string]string{"message": "hello from v1"}, logger)
			})
			router.Post("/panic", func(rw http.ResponseWriter, r *http.Request) {
				panic("PANIC!!!")
			})
		},
	}
	const errDomain = "MyService"
	opts := Opts{ServiceNameInURL: "my-service", ErrorDomain: errDomain, APIRoutes: apiRoutes}

	addr := testutil.GetLocalAddrWithFreeTCPPort()

	logger := logtest.NewRecorder()
	logConfig := LogConfig{
		RequestStart:      true,
		RequestHeaders:    []string{"X-Custom-Header1", "X-Custom-Header2"},
		ExcludedEndpoints: []string{"/metrics", "/healthz"},
		SecretQueryParams: []string{"token", "sign"},
	}

	httpServer, err := New(&Config{Address: addr, Log: logConfig}, logger, opts)
	require.NoError(t, err)
	fatalErr := make(chan error, 1)
	go httpServer.Start(fatalErr)
	require.NoError(t, testutil.WaitListeningServer(addr, time.Second*3))
	defer func() {
		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	var resp *http.Response

	req, err := http.NewRequest(http.MethodGet,
		httpServer.URL+"/api/my-service/v1/hello?token=secretToken&sign=secretSign&foo=bar", nil)
	require.NoError(t, err)
	req.Header.Set("X-Custom-Header1", "value1")
	req.Header.Set("X-Custom-Header2", "value2")
	req.Header.Set("X-Custom-Header3", "value3")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	for _, logMsg := range []string{"request started", "response completed"} {
		logEntry, found := logger.FindEntryByFilter(func(entry logtest.RecordedEntry) bool {
			return strings.Contains(entry.Text, logMsg)
		})
		require.True(t, found, "%q should be logged", logMsg)

		var logField *log.Field

		// Check custom headers (only that were specified in the config) are logged.
		logField, found = logEntry.FindField("req_header_x_custom_header1")
		require.True(t, found)
		require.Equal(t, "value1", string(logField.Bytes))
		logField, found = logEntry.FindField("req_header_x_custom_header2")
		require.True(t, found)
		require.Equal(t, "value2", string(logField.Bytes))
		_, found = logEntry.FindField("req_header_x_custom_header3")
		require.False(t, found)

		// Check secret query parameters are hidden.
		logField, found = logEntry.FindField("uri")
		require.True(t, found)
		var parsedLoggedURL *url.URL
		parsedLoggedURL, err = url.Parse(string(logField.Bytes))
		require.NoError(t, err)
		require.Equal(t, "bar", parsedLoggedURL.Query().Get("foo"))
		require.Equal(t, middleware.LoggingSecretQueryPlaceholder, parsedLoggedURL.Query().Get("token"))
		require.Equal(t, middleware.LoggingSecretQueryPlaceholder, parsedLoggedURL.Query().Get("sign"))
	}

	logger.Reset()

	// Check requests for excluded endpoints are not logged.
	for _, endpoint := range []string{"/metrics", "/healthz"} {
		resp, err = http.Get(httpServer.URL + endpoint)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		for _, logMsg := range []string{"request started", "response completed"} {
			_, found := logger.FindEntryByFilter(func(entry logtest.RecordedEntry) bool {
				return strings.Contains(entry.Text, logMsg)
			})
			require.False(t, found, "%q should NOT be logged", logMsg)
		}
	}
}

func TestHTTPServer_MaxRequestsLimiting(t *testing.T) {
	httpServerCfg := &Config{Address: testutil.GetLocalAddrWithFreeTCPPort(), Limits: LimitsConfig{MaxRequests: 1}}

	httpServer, err := New(httpServerCfg, logtest.NewLogger(), Opts{})
	require.NoError(t, err)

	httpServer.HTTPRouter.Get("/slow", func(rw http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		rw.WriteHeader(http.StatusOK)
	})
	fatalErr := make(chan error, 1)
	go httpServer.Start(fatalErr)
	require.NoError(t, testutil.WaitListeningServer(httpServerCfg.Address, time.Second*3))
	defer func() {
		require.NoError(t, httpServer.Stop(false))
		testutil.RequireNoErrorInChannel(t, fatalErr)
	}()

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		slowResp, getErr := http.Get(httpServer.URL + "/slow")
		if getErr != nil {
			errCh <- getErr
			return
		}
		if slowResp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("unexpected status code, expected: %d, got: %d", http.StatusOK, slowResp.StatusCode)
			return
		}
		errCh <- slowResp.Body.Close()
	}()

	time.Sleep(time.Millisecond * 500) // Give time to send request.

	resp, err := http.Get(httpServer.URL + "/slow")
	require.NoError(t, err)
	require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// System endpoints (/healthz, /metrics) should not be limited.

	resp, err = http.Get(httpServer.URL + "/healthz")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp, err = http.Get(httpServer.URL + "/metrics")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.NoError(t, <-errCh)
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle_test

import (
	"bytes"
	"fmt"
	stdlog "log"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"time"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/httpserver/middleware/throttle"
)

const apiErrDomain = "MyService"

func Example() {
	configReader := bytes.NewReader([]byte(`
rateLimitZones:
  rl_zone1:
    rateLimit: 1/s
    burstLimit: 0
    responseStatusCode: 503
    responseRetryAfter: auto
    dryRun: false

  rl_zone2:
    rateLimit: 5/m
    burstLimit: 0
    responseStatusCode: 429
    responseRetryAfter: auto
    key:
      type: "identity"
    dryRun: false

inFlightLimitZones:
  ifl_zone1:
    inFlightLimit: 1
    backlogLimit: 0
    backlogTimeout: 15s
    responseStatusCode: 503
    dryRun: false

rules:
  - routes:
    - path: "/hello-world"
      methods: GET
    excludedRoutes:
      - path: "/healthz"
    rateLimits:
      - zone: rl_zone1
    tags: all_reqs

  - routes:
    - path: "= /long-work"
      methods: POST
    inFlightLimits:
      - zone: ifl_zone1
    tags: all_reqs

  - routes:
    - path: ~^/api/2/tenants/([\w\-]{36})/?$
      methods: PUT
    rateLimits:
      - zone: rl_zone2
    tags: authenticated_reqs
`))
	configLoader := config.NewLoader(config.NewViperAdapter())
	cfg := &throttle.Config{}
	if err := configLoader.LoadFromReader(configReader, config.DataTypeYAML, cfg); err != nil {
		stdlog.Fatal(err)
		return
	}

	const longWorkDelay = time.Second

	srv := makeExampleTestServer(cfg, longWorkDelay)
	defer srv.Close()

	// Rate limiting.
	// 1st request finished successfully.
	resp1, _ := http.Get(srv.URL + "/hello-world")
	_ = resp1.Body.Close()
	fmt.Println("[1] GET /hello-world " + strconv.Itoa(resp1.StatusCode))
	// 2nd request is throttled.
	resp2, _ := http.Get(srv.URL + "/hello-world")
	_ = resp2.Body.Close()
	fmt.Println("[2] GET /hello-world " + strconv.Itoa(resp2.StatusCode))

	// In-flight limiting.
	// 3rd request finished successfully.
	resp3code := make(chan int)
	go func() {
		resp3, _ := http.Post(srv.URL+"/long-work", "", nil)
		_ = resp3.Body.Close()
		resp3code <- resp3.StatusCode
	}()
	time.Sleep(longWorkDelay / 2)
	// 4th request is throttled.
	resp4, _ := http.Post(srv.URL+"/long-work", "", nil)
	_ = resp4.Body.Close()
	fmt.Println("[3] POST /long-work " + strconv.Itoa(<-resp3code))
	fmt.Println("[4] POST /long-work " + strconv.Itoa(resp4.StatusCode))

	// Unmatched (unspecified) routes are not limited.
	resp5code := make(chan int)
	go func() {
		resp5, _ := http.Post(srv.URL+"/long-work-without-limits", "", nil)
		_ = resp5.Body.Close()
		resp5code <- resp5.StatusCode
	}()
	time.Sleep(longWorkDelay / 2)
	resp6, _ := http.Post(srv.URL+"/long-work", "", nil)
	_ = resp6.Body.Close()
	fmt.Println("[5] POST /long-work-without-limits " + strconv.Itoa(<-resp5code))
	fmt.Println("[6] POST /long-work-without-limits " + strconv.Itoa(resp6.StatusCode))

	// Throttle authenticated requests by username from basic auth.
	const tenantPath = "/api/2/tenants/446507ba-2f9b-4347-adbc-63581383ba25"
	doReqWithBasicAuth := func(username string) *http.Response {
		req, _ := http.NewRequest(http.MethodPut, srv.URL+tenantPath, http.NoBody)
		req.SetBasicAuth(username, username+"-password")
		resp, _ := http.DefaultClient.Do(req)
		return resp
	}
	// 7th request is not throttled.
	resp7 := doReqWithBasicAuth("ba27afb7-ad60-4077-956e-366e77358b92")
	_ = resp7.Body.Close()
	fmt.Printf("[7] PUT %s %d\n", tenantPath, resp7.StatusCode)
	// 8th request is throttled (the same username as in the previous request, and it's rate-limited).
	resp8 := doReqWithBasicAuth("ba27afb7-ad60-4077-956e-366e77358b92")
	_ = resp8.Body.Close()
	fmt.Printf("[8] PUT %s %d\n", tenantPath, resp8.StatusCode)
	// 9th request is not throttled (the different username is used).
	resp9 := doReqWithBasicAuth("97d8d1e6-948d-4c41-91d6-495dcc8c7b1a")
	_ = resp9.Body.Close()
	fmt.Printf("[9] PUT %s %d\n", tenantPath, resp9.StatusCode)

	// Output:
	// [1] GET /hello-world 200
	// [2] GET /hello-world 503
	// [3] POST /long-work 200
	// [4] POST /long-work 503
	// [5] POST /long-work-without-limits 200
	// [6] POST /long-work-without-limits 200
	// [7] PUT /api/2/tenants/446507ba-2f9b-4347-adbc-63581383ba25 204
	// [8] PUT /api/2/tenants/446507ba-2f9b-4347-adbc-63581383ba25 429
	// [9] PUT /api/2/tenants/446507ba-2f9b-4347-adbc-63581383ba25 204
}

func makeExampleTestServer(cfg *throttle.Config, longWorkDelay time.Duration) *httptest.Server {
	promMetrics := throttle.NewPrometheusMetrics()
	promMetrics.MustRegister()
	defer promMetrics.Unregister()

	// Configure middleware that should do global throttling ("all_reqs" tag says about that).
	allReqsThrottleMiddleware := throttle.MiddlewareWithOpts(cfg, apiErrDomain, promMetrics, throttle.MiddlewareOpts{
		Tags: []string{"all_reqs"}})

	// Configure middleware that should do per-client throttling based on the username from basic auth ("authenticated_reqs" tag says about that).
	authenticatedReqsThrottleMiddleware := throttle.MiddlewareWithOpts(cfg, apiErrDomain, promMetrics, throttle.MiddlewareOpts{
		Tags: []string{"authenticated_reqs"},
		GetKeyIdentity: func(r *http.Request) (key string, bypass bool, err error) {
			username, _, ok := r.BasicAuth()
			if !ok {
				return "", true, fmt.Errorf("no basic auth")
			}
			return username, false, nil
		},
	})

	restoreTenantPathRegExp := regexp.MustCompile(`^/api/2/tenants/([\w-]{36})/?$`)
	return httptest.NewServer(allReqsThrottleMiddleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/long-work":
			if r.Method != http.MethodPost {
				rw.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			time.Sleep(longWorkDelay) // Emulate long work.
			rw.WriteHeader(http.StatusOK)
			return

		case "/hello-world":
			if r.Method != http.MethodGet {
				rw.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte("Hello world!"))
			return

		case "/long-work-without-limits":
			if r.Method != http.MethodPost {
				rw.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			time.Sleep(longWorkDelay) // Emulate long work.
			rw.WriteHeader(http.StatusOK)
			return
		}

		if restoreTenantPathRegExp.MatchString(r.URL.Path) {
			if r.Method != http.MethodPut {
				rw.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			authenticatedReqsThrottleMiddleware(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				rw.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(rw, r)
			return
		}

		rw.WriteHeader(http.StatusNotFound)
	})))
}

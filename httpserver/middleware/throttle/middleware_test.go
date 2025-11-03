/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/httpserver/middleware"
	"github.com/acronis/go-appkit/log"
)

const testErrDomain = "TestService"

func TestLimitHandler_ServeHTTP(t *testing.T) {
	matchedPrefixedRoutes := []string{"POST /aaa", "PUT /aaa", "DELETE /aaa", "POST /aaa/", "PUT /aaa/bbb", "DELETE /aaa/b/c"}
	matchedExactRoutes := []string{"GET /bbb", "POST /bbb"}
	var matchedRoutes []string
	matchedRoutes = append(matchedRoutes, matchedPrefixedRoutes...)
	matchedRoutes = append(matchedRoutes, matchedExactRoutes...)

	unmatchedPrefixedRoutes := []string{"GET /aaa", "HEAD /aaa", "GET /aaa/b"}
	unmatchedExactRoutes := []string{"GET /bbb/", "POST /bbb/", "GET /bbb/a"}
	unmatchedOtherRoutes := []string{"POST /a", "PUT /b", "DELETE /c"}
	var unmatchedRoutes []string
	unmatchedRoutes = append(unmatchedRoutes, unmatchedPrefixedRoutes...)
	unmatchedRoutes = append(unmatchedRoutes, unmatchedExactRoutes...)
	unmatchedRoutes = append(unmatchedRoutes, unmatchedOtherRoutes...)

	tests := []struct {
		Name    string
		CfgData string
		Func    func(t *testing.T, cfg *Config)
	}{
		{
			Name: "rate limiting, leaky bucket",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				const burst = 10

				// Prefixed path matching.
				reqsGen := makeReqsGenerator(matchedPrefixedRoutes)
				checkRateLimiting(t, cfg, reqsGen, burst+1, 30, 503, time.Second*5)

				// Prefixed path unmatching.
				reqsGen = makeReqsGenerator(unmatchedPrefixedRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)

				// Exact path matching.
				reqsGen = makeReqsGenerator(matchedExactRoutes)
				checkRateLimiting(t, cfg, reqsGen, burst+1, 30, 503, time.Second*5)

				// Exact path unmatching.
				reqsGen = makeReqsGenerator(unmatchedExactRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)

				// Other endpoints should NOT be throttled.
				reqsGen = makeReqsGenerator(unmatchedOtherRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)

				// Paths with dotes are normalised before throttling.
				reqsGen = makeReqsGenerator([]string{"GET /bbb/.", "GET /bbb/cc/..", "GET /bbb/cc/../cc/..", "GET /bbb/cc/../././."})
				checkRateLimiting(t, cfg, reqsGen, burst+1, 30, 503, time.Second*5)
			},
		},
		{
			Name: "rate limiting, sliding window",
			CfgData: `
rateLimitZones:
  rl_zone:
    alg: sliding_window
    rateLimit: 10/m
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				const ratePerMinute = 10

				// Prefixed path matching.
				reqsGen := makeReqsGenerator(matchedPrefixedRoutes)
				checkRateLimiting(t, cfg, reqsGen, ratePerMinute, 30, 503, time.Second*5)

				// Prefixed path unmatching.
				reqsGen = makeReqsGenerator(unmatchedPrefixedRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)

				// Exact path matching.
				reqsGen = makeReqsGenerator(matchedExactRoutes)
				checkRateLimiting(t, cfg, reqsGen, ratePerMinute, 30, 503, time.Second*5)

				// Exact path unmatching.
				reqsGen = makeReqsGenerator(unmatchedExactRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)

				// Other endpoints should NOT be throttled.
				reqsGen = makeReqsGenerator(unmatchedOtherRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)

				// Paths with dotes are normalised before throttling.
				reqsGen = makeReqsGenerator([]string{"GET /bbb/.", "GET /bbb/cc/..", "GET /bbb/cc/../cc/..", "GET /bbb/cc/../././."})
				checkRateLimiting(t, cfg, reqsGen, ratePerMinute, 30, 503, time.Second*5)
			},
		},
		{
			Name: "rate limiting, leaky bucket, backlogLimit > 0",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/s
    backlogLimit: 3
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				reqsGen := makeReqsGenerator(matchedPrefixedRoutes)
				checkRateLimiting(t, cfg, reqsGen, 3, 3, 503, time.Second*5)
			},
		},
		{
			Name: "rate limiting, leaky bucket, dry-run mode",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 503
    responseRetryAfter: 5s
    dryRun: true
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				checkRateLimitingInDryRun(t, cfg, makeReqsGenerator(matchedRoutes), 10, 30)
				checkNoRateLimiting(t, cfg, makeReqsGenerator(unmatchedRoutes), 30)
			},
		},
		{
			Name: "rate limiting, leaky bucket, by http header",
			CfgData: `
rateLimitZones:
  rl_zone:
    key:
      type: header
      headerName: x-client-id
      noBypassEmpty: true
    excludedKeys: ["good-client1", "good-client2", "very-good-client*"]
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 429
    responseRetryAfter: 30s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				const burst = 10

				// Many requests with the same X-Client-ID. Should be throttled.
				reqsGen := makeReqsGenerator(matchedRoutes)
				reqsGenWithHeader := func() *http.Request {
					r := reqsGen()
					r.Header.Set("X-Client-ID", "client-id")
					return r
				}
				checkRateLimiting(t, cfg, reqsGenWithHeader, burst+1, 30, 429, time.Second*30)

				// Many requests with missing X-Client-ID. Should be throttled since noBypassEmpty is true.
				checkRateLimiting(t, cfg, makeReqsGenerator(matchedRoutes), burst+1, 30, 429, time.Second*30)

				// Many requests with the different X-Client-ID.
				reqsGen = makeReqsGenerator(matchedRoutes)
				clientIDsGen := makeFmtGenerator("client-%d")
				reqsGenWithHeader = func() *http.Request {
					r := reqsGen()
					r.Header.Set("X-Client-ID", clientIDsGen())
					return r
				}
				checkNoRateLimiting(t, cfg, reqsGenWithHeader, 100)

				// Excluded clients should NOT be throttled.
				reqsGen = makeReqsGenerator(matchedRoutes)
				clientIDsGen = makeStrsGenerator([]string{"good-client1", "good-client2", "very-good-client1", "very-good-client777"})
				reqsGenWithHeader = func() *http.Request {
					r := reqsGen()
					r.Header.Set("X-Client-ID", clientIDsGen())
					return r
				}
				checkNoRateLimiting(t, cfg, reqsGenWithHeader, 100)
			},
		},
		{
			Name: "rate limiting, leaky bucket, by identity",
			CfgData: `
rateLimitZones:
  rl_zone:
    key:
      type: identity
    includedKeys: ["bad-user1", "bad-user2", "very-bad-user*"]
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 429
    responseRetryAfter: 60s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				const burst = 10

				// Included clients should be throttled.
				for _, identity := range []string{"bad-user1", "bad-user2", "very-bad-user1", "very-bad-user777"} {
					reqsGen := makeReqsGenerator(matchedRoutes)
					reqsGenWithBasicAuth := func() *http.Request {
						r := reqsGen()
						r.SetBasicAuth(identity, identity+"-password")
						return r
					}
					checkRateLimiting(t, cfg, reqsGenWithBasicAuth, burst+1, 30, 429, time.Second*60)
				}

				// Other clients should NOT be throttled.
				reqsGen := makeReqsGenerator(matchedRoutes)
				reqsGenWithBasicAuth := func() *http.Request {
					r := reqsGen()
					r.SetBasicAuth("good-user", "good-user-password")
					return r
				}
				checkNoRateLimiting(t, cfg, reqsGenWithBasicAuth, 30)
			},
		},
		{
			Name: "rate limiting, leaky bucket, by remote addr",
			CfgData: `
rateLimitZones:
  rl_zone:
    key:
      type: remote_addr
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 429
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    rateLimits:
      - zone: rl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				const burst = 10

				// Prefixed path matching.
				reqsGen := makeReqsGenerator(matchedPrefixedRoutes)
				checkRateLimiting(t, cfg, reqsGen, burst+1, 30, 429, time.Second*5)

				// Other endpoints should NOT be throttled.
				reqsGen = makeReqsGenerator(unmatchedOtherRoutes)
				checkNoRateLimiting(t, cfg, reqsGen, 30)
			},
		},
		{
			Name: "in-flight limiting",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    inFlightLimit: 5
    backlogLimit: 5
    backlogTimeout: 30s
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    inFlightLimits:
      - zone: ifl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				checkInFlightLimiting(t, cfg, checkInFlightLimitingParams{
					reqsGen:        makeReqsGenerator(matchedRoutes),
					totalLimit:     10,
					reqsNum:        20,
					unblockDelay:   time.Second,
					wantRespCode:   503,
					wantRetryAfter: time.Second * 5,
				})
				checkNoInFlightLimiting(t, cfg, makeReqsGenerator(unmatchedRoutes), 20, time.Second)
			},
		},
		{
			Name: "in-flight limiting, build handler at init",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    inFlightLimit: 5
    backlogLimit: 5
    backlogTimeout: 30s
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    inFlightLimits:
      - zone: ifl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				checkInFlightLimiting(t, cfg, checkInFlightLimitingParams{
					reqsGen:            makeReqsGenerator(matchedRoutes),
					totalLimit:         10,
					reqsNum:            20,
					unblockDelay:       time.Second,
					wantRespCode:       503,
					wantRetryAfter:     time.Second * 5,
					buildHandlerAtInit: true,
				})
			},
		},
		{
			Name: "in-flight limiting, dry-run mode",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    inFlightLimit: 10
    backlogTimeout: 30s
    responseStatusCode: 503
    responseRetryAfter: 5s
    dryRun: true
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    inFlightLimits:
      - zone: ifl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				checkInFlightLimitingInDryRun(t, cfg, makeReqsGenerator(matchedRoutes), 10, 20, time.Second)
				checkNoInFlightLimiting(t, cfg, makeReqsGenerator(unmatchedRoutes), 20, time.Second)
			},
		},
		{
			Name: "in-flight limiting by http header",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    key:
      type: header
      headerName: x-client-id
    excludedKeys: ["good-client1", "good-client2", "very-good-client*"]
    inFlightLimit: 5
    backlogLimit: 5
    backlogTimeout: 30s
    responseStatusCode: 429
    responseRetryAfter: 30s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    inFlightLimits:
      - zone: ifl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				// Many requests with the same X-Client-ID. Should be throttled.
				reqsGen := makeReqsGenerator(matchedRoutes)
				reqsGenWithHeader := func() *http.Request {
					r := reqsGen()
					r.Header.Set("X-Client-ID", "client-id")
					return r
				}
				checkInFlightLimiting(t, cfg, checkInFlightLimitingParams{
					reqsGen:        reqsGenWithHeader,
					totalLimit:     10,
					reqsNum:        20,
					unblockDelay:   time.Second,
					wantRespCode:   429,
					wantRetryAfter: time.Second * 30,
				})

				// Many requests with missing X-Client-ID should NOT be throttled since noBypassEmpty is false (by default).
				checkNoInFlightLimiting(t, cfg, makeReqsGenerator(matchedRoutes), 20, time.Second)

				// Many requests with the different X-Client-ID should NOT be throttled.
				reqsGen = makeReqsGenerator(matchedRoutes)
				clientIDsGen := makeFmtGenerator("client-%d")
				reqsGenWithHeader = func() *http.Request {
					r := reqsGen()
					r.Header.Set("X-Client-ID", clientIDsGen())
					return r
				}
				checkNoInFlightLimiting(t, cfg, reqsGenWithHeader, 20, time.Second)

				// Excluded clients should NOT be throttled.
				reqsGen = makeReqsGenerator(matchedRoutes)
				clientIDsGen = makeStrsGenerator([]string{"good-client1", "good-client2", "very-good-client1", "very-good-client777"})
				reqsGenWithHeader = func() *http.Request {
					r := reqsGen()
					r.Header.Set("X-Client-ID", clientIDsGen())
					return r
				}
				checkNoInFlightLimiting(t, cfg, reqsGenWithHeader, 40, time.Second)
			},
		},
		{
			Name: "in-flight limiting by identity",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    key:
      type: identity
      noBypassEmpty: true
    includedKeys: ["", "bad-user1", "bad-user2", "very-bad-user*"]
    inFlightLimit: 5
    backlogLimit: 5
    backlogTimeout: 30s
    responseStatusCode: 429
    responseRetryAfter: 60s
rules:
  - routes:
    - path: "/aaa"
      methods: POST,PUT,DELETE
    - path: "= /bbb"
    inFlightLimits:
      - zone: ifl_zone
`,
			Func: func(t *testing.T, cfg *Config) {
				// Included clients should be throttled.
				for _, identity := range []string{"bad-user1", "bad-user2", "very-bad-user1", "very-bad-user777"} {
					reqsGen := makeReqsGenerator(matchedRoutes)
					reqsGenWithBasicAuth := func() *http.Request {
						r := reqsGen()
						r.SetBasicAuth(identity, identity+"-password")
						return r
					}
					checkInFlightLimiting(t, cfg, checkInFlightLimitingParams{
						reqsGen:        reqsGenWithBasicAuth,
						totalLimit:     10,
						reqsNum:        30,
						unblockDelay:   time.Second,
						wantRespCode:   429,
						wantRetryAfter: time.Second * 60,
					})
				}

				// Other clients should NOT be throttled.
				reqsGen := makeReqsGenerator(matchedRoutes)
				reqsGenWithBasicAuth := func() *http.Request {
					r := reqsGen()
					r.SetBasicAuth("good-user", "good-user-password")
					return r
				}
				checkNoInFlightLimiting(t, cfg, reqsGenWithBasicAuth, 30, time.Second)
			},
		},
		{
			Name: "tags usage",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST, PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone
    tags: tag_a,tag_b
`,
			Func: func(t *testing.T, cfg *Config) {
				const burst = 10

				reqsMatchGen := makeReqsGenerator(matchedPrefixedRoutes)
				reqsMismatchGen := makeReqsGenerator(unmatchedPrefixedRoutes)

				// No tags, all rules should be used.
				checkRateLimiting(t, cfg, reqsMatchGen, burst+1, 30, 503, time.Second*5)

				// Tags match.
				checkRateLimiting(t, cfg, reqsMatchGen, burst+1, 30, 503, time.Second*5, "tag_a", "tag_c")
				checkRateLimiting(t, cfg, reqsMatchGen, burst+1, 30, 503, time.Second*5, "tag_c", "tag_b")

				// Tags mismatch.
				checkNoRateLimiting(t, cfg, reqsMatchGen, 30, "tag_c", "tag_d")

				// Tags match, routes mismatch.
				checkNoRateLimiting(t, cfg, reqsMismatchGen, 30)
				checkNoRateLimiting(t, cfg, reqsMismatchGen, 30, "tag_a")
			},
		},
		{
			Name: "tags usage (zone level)",
			CfgData: `
rateLimitZones:
  rl_zone1:
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 503
    responseRetryAfter: 5s
  rl_zone2:
    rateLimit: 2/m
    burstLimit: 5
    responseStatusCode: 429
    responseRetryAfter: 10s
rules:
  - routes:
    - path: "/aaa"
      methods: POST, PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone1
        tags: tag_a
      - zone: rl_zone2
        tags: tag_b
`,
			Func: func(t *testing.T, cfg *Config) {
				reqsMatchGen := makeReqsGenerator(matchedPrefixedRoutes)

				// Tags match only rl_zone1 (tag_a).
				checkRateLimiting(t, cfg, reqsMatchGen, 11, 30, 503, time.Second*5, "tag_a")

				// Tags match only rl_zone2 (tag_b).
				checkRateLimiting(t, cfg, reqsMatchGen, 6, 30, 429, time.Second*10, "tag_b")

				// Tags mismatch - no rate limiting should be applied.
				checkNoRateLimiting(t, cfg, reqsMatchGen, 30, "tag_c")
			},
		},
		{
			Name: "tags usage (rule-level tags take precedence)",
			CfgData: `
rateLimitZones:
  rl_zone_with_zone_tag:
    rateLimit: 1/m
    burstLimit: 10
    responseStatusCode: 503
    responseRetryAfter: 5s
rules:
  - routes:
    - path: "/aaa"
      methods: POST, PUT,DELETE
    - path: "= /bbb"
    rateLimits:
      - zone: rl_zone_with_zone_tag
        tags: tag_zone_mismatch
    tags: tag_rule
`,
			Func: func(t *testing.T, cfg *Config) {
				const burst = 10
				reqsMatchGen := makeReqsGenerator(matchedPrefixedRoutes)

				// Middleware provides "tag_rule" which matches rule-level tag.
				// Zone should be included even though its zone-level tag "tag_zone_mismatch" doesn't match.
				// This proves rule-level tag match takes precedence.
				checkRateLimiting(t, cfg, reqsMatchGen, burst+1, 30, 503, time.Second*5, "tag_rule")

				// Middleware provides "tag_zone_mismatch" which matches zone-level tag but NOT rule-level tag.
				// Zone should be included because zone tag matches.
				checkRateLimiting(t, cfg, reqsMatchGen, burst+1, 30, 503, time.Second*5, "tag_zone_mismatch")

				// Middleware provides "tag_none" which matches neither rule nor zone tags.
				// No zones should be included.
				checkNoRateLimiting(t, cfg, reqsMatchGen, 30, "tag_none")
			},
		},
	}
	configLoader := config.NewLoader(config.NewViperAdapter())
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			cfg := &Config{}
			err := configLoader.LoadFromReader(bytes.NewReader([]byte(tt.CfgData)), config.DataTypeYAML, cfg)
			require.NoError(t, err)
			tt.Func(t, cfg)
		})
	}
}

type testCounters struct {
	nextCalls atomic.Int32

	rateLimitRejects       atomic.Int32
	rateLimitDryRunRejects atomic.Int32
	rateLimitErrors        atomic.Int32

	inFlightLimitRejects       atomic.Int32 //nolint
	inFlightLimitDryRunRejects atomic.Int32 //nolint
	inFlightLimitErrors        atomic.Int32 //nolint
}

// nolint
func (c *testCounters) checkRateLimit(t *testing.T, wantRejects, wantDryRunRejects, wantErrors int) {
	require.Equal(t, wantRejects, int(c.rateLimitRejects.Load()))
	require.Equal(t, wantDryRunRejects, int(c.rateLimitDryRunRejects.Load()))
	require.Equal(t, wantErrors, int(c.rateLimitErrors.Load()))
}

// nolint
func (c *testCounters) checkInFlightLimit(t *testing.T, wantRejects, wantDryRunRejects, wantErrors int) {
	require.Equal(t, wantRejects, int(c.inFlightLimitRejects.Load()))
	require.Equal(t, wantDryRunRejects, int(c.inFlightLimitDryRunRejects.Load()))
	require.Equal(t, wantErrors, int(c.inFlightLimitErrors.Load()))
}

func makeHandlerWrappedIntoMiddleware(
	cfg *Config, blockCh chan struct{}, tags []string, buildHandlerAtInit bool,
) (http.Handler, *testCounters, error) {
	c := &testCounters{}
	mw, err := MiddlewareWithOpts(cfg, testErrDomain, NewPrometheusMetrics(), MiddlewareOpts{
		GetKeyIdentity: func(r *http.Request) (key string, bypass bool, err error) {
			username, _, ok := r.BasicAuth()
			if !ok {
				return "", false, fmt.Errorf("no basic auth")
			}
			return username, false, nil
		},
		RateLimitOnReject: func(
			rw http.ResponseWriter, r *http.Request, params middleware.RateLimitParams, next http.Handler, logger log.FieldLogger,
		) {
			c.rateLimitRejects.Inc()
			middleware.DefaultRateLimitOnReject(rw, r, params, next, logger)
		},
		RateLimitOnRejectInDryRun: func(
			rw http.ResponseWriter, r *http.Request, params middleware.RateLimitParams, next http.Handler, logger log.FieldLogger,
		) {
			c.rateLimitDryRunRejects.Inc()
			middleware.DefaultRateLimitOnRejectInDryRun(rw, r, params, next, logger)
		},
		RateLimitOnError: func(
			rw http.ResponseWriter, r *http.Request, params middleware.RateLimitParams, err error,
			next http.Handler, logger log.FieldLogger,
		) {
			c.inFlightLimitErrors.Inc()
			middleware.DefaultRateLimitOnError(rw, r, params, err, next, logger)
		},
		InFlightLimitOnReject: func(
			rw http.ResponseWriter, r *http.Request, params middleware.InFlightLimitParams, next http.Handler, logger log.FieldLogger,
		) {
			c.inFlightLimitRejects.Inc()
			middleware.DefaultInFlightLimitOnReject(rw, r, params, next, logger)
		},
		InFlightLimitOnRejectInDryRun: func(
			rw http.ResponseWriter, r *http.Request, params middleware.InFlightLimitParams, next http.Handler, logger log.FieldLogger,
		) {
			c.inFlightLimitDryRunRejects.Inc()
			middleware.DefaultInFlightLimitOnRejectInDryRun(rw, r, params, next, logger)
		},
		InFlightLimitOnError: func(
			rw http.ResponseWriter, r *http.Request, params middleware.InFlightLimitParams, err error,
			next http.Handler, logger log.FieldLogger,
		) {
			c.inFlightLimitErrors.Inc()
			middleware.DefaultInFlightLimitOnError(rw, r, params, err, next, logger)
		},
		Tags:               tags,
		BuildHandlerAtInit: buildHandlerAtInit,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create throttling middleware: %w", err)
	}
	return mw(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		c.nextCalls.Inc()
		if blockCh != nil {
			if err := waitSend(blockCh, time.Second*5); err != nil {
				rw.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		rw.WriteHeader(http.StatusOK)
	})), c, nil
}

// nolint
func checkRateLimiting(
	t *testing.T,
	cfg *Config,
	reqsGen func() *http.Request,
	wantNotThrottledReqsNum int,
	totalReqsNum int,
	wantRespCode int,
	wantRetryAfter time.Duration,
	tags ...string,
) {
	if totalReqsNum < wantNotThrottledReqsNum {
		panic("totalReqsNum should be > burst+1")
	}

	throttleHandler, counters, err := makeHandlerWrappedIntoMiddleware(cfg, nil, tags, false)
	require.NoError(t, err)

	// First N requests SHOULD NOT BE throttled.
	for i := 0; i < wantNotThrottledReqsNum; i++ {
		respRec := httptest.NewRecorder()
		throttleHandler.ServeHTTP(respRec, reqsGen())
		require.Equal(t, http.StatusOK, respRec.Code)
	}

	require.Equal(t, wantNotThrottledReqsNum, int(counters.nextCalls.Load()))
	counters.checkRateLimit(t, 0, 0, 0)

	// Next requests SHOULD BE throttled.
	for i := wantNotThrottledReqsNum; i < totalReqsNum; i++ {
		respRec := httptest.NewRecorder()
		throttleHandler.ServeHTTP(respRec, reqsGen())
		require.Equal(t, wantRespCode, respRec.Code)
		retryAfterSecs, err := strconv.Atoi(respRec.Header().Get("Retry-After"))
		require.NoError(t, err)
		require.Equal(t, wantRetryAfter, time.Duration(retryAfterSecs)*time.Second)
	}
	require.Equal(t, wantNotThrottledReqsNum, int(counters.nextCalls.Load())) // Not changed.
	counters.checkRateLimit(t, totalReqsNum-wantNotThrottledReqsNum, 0, 0)
}

func checkRateLimitingInDryRun(t *testing.T, cfg *Config, reqsGen func() *http.Request, burst, reqsNum int, tags ...string) {
	if reqsNum <= burst+1 {
		panic("reqsNum should be > burst+1")
	}
	checkNoRateLimitingOrDryRun(t, cfg, reqsGen, reqsNum, reqsNum-burst-1, tags...)
}

func checkNoRateLimiting(t *testing.T, cfg *Config, reqsGen func() *http.Request, reqsNum int, tags ...string) {
	checkNoRateLimitingOrDryRun(t, cfg, reqsGen, reqsNum, 0, tags...)
}

func checkNoRateLimitingOrDryRun(
	t *testing.T, cfg *Config, reqsGen func() *http.Request, reqsNum, wantDryRunRejects int, tags ...string,
) {
	throttleHandler, counters, err := makeHandlerWrappedIntoMiddleware(cfg, nil, tags, false)
	require.NoError(t, err)
	for i := 0; i < reqsNum; i++ {
		respRec := httptest.NewRecorder()
		throttleHandler.ServeHTTP(respRec, reqsGen())
		require.Equal(t, http.StatusOK, respRec.Code)
	}
	require.Equal(t, reqsNum, int(counters.nextCalls.Load()))
	counters.checkRateLimit(t, 0, wantDryRunRejects, 0)
}

type checkInFlightLimitingParams struct {
	reqsGen            func() *http.Request
	totalLimit         int
	reqsNum            int
	unblockDelay       time.Duration
	wantRespCode       int
	wantRetryAfter     time.Duration
	tags               []string
	buildHandlerAtInit bool
}

func checkInFlightLimiting(t *testing.T, cfg *Config, params checkInFlightLimitingParams) {
	if params.reqsNum <= params.totalLimit {
		panic("reqsNum should be > totalLimit")
	}
	blockCh := make(chan struct{})
	throttleHandler, counters, err := makeHandlerWrappedIntoMiddleware(cfg, blockCh, params.tags, params.buildHandlerAtInit)
	require.NoError(t, err)
	var okCodes, throttledCodes, unexpectedCodes, wrongRetryAfterNums atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < params.reqsNum; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			respRec := httptest.NewRecorder()
			throttleHandler.ServeHTTP(respRec, params.reqsGen())
			switch respRec.Code {
			case http.StatusOK:
				okCodes.Inc()
			case params.wantRespCode:
				throttledCodes.Inc()
				gotRetryAfterSecs, err := strconv.Atoi(respRec.Header().Get("Retry-After"))
				if err != nil || time.Duration(gotRetryAfterSecs)*time.Second != params.wantRetryAfter {
					wrongRetryAfterNums.Inc()
				}
			default:
				unexpectedCodes.Inc()
			}
		}()
	}
	time.Sleep(params.unblockDelay)
	for i := 0; i < params.totalLimit; i++ {
		require.NoError(t, waitRecv(blockCh, time.Second*5))
	}
	wg.Wait()

	require.Equal(t, params.totalLimit, int(counters.nextCalls.Load()))
	require.Equal(t, params.totalLimit, int(okCodes.Load()))
	require.Equal(t, params.reqsNum-params.totalLimit, int(throttledCodes.Load()))
	require.Equal(t, 0, int(wrongRetryAfterNums.Load()))
	require.Equal(t, 0, int(unexpectedCodes.Load()))
	counters.checkInFlightLimit(t, params.reqsNum-params.totalLimit, 0, 0)
}

func checkInFlightLimitingInDryRun(
	t *testing.T,
	cfg *Config,
	reqsGen func() *http.Request,
	totalLimit int,
	reqsNum int,
	unblockDelay time.Duration,
	tags ...string,
) {
	if reqsNum <= totalLimit {
		panic("reqsNum should be > totalLimit")
	}
	checkNoInFlightLimitingOrDryRun(t, cfg, reqsGen, reqsNum, unblockDelay, reqsNum-totalLimit, tags...)
}

//nolint:unparam
func checkNoInFlightLimiting(
	t *testing.T, cfg *Config, reqsGen func() *http.Request, reqsNum int, unblockDelay time.Duration, tags ...string,
) {
	checkNoInFlightLimitingOrDryRun(t, cfg, reqsGen, reqsNum, unblockDelay, 0, tags...)
}

func checkNoInFlightLimitingOrDryRun(
	t *testing.T,
	cfg *Config,
	reqsGen func() *http.Request,
	reqsNum int,
	unblockDelay time.Duration,
	wantDryRunRejects int,
	tags ...string,
) {
	blockCh := make(chan struct{})
	throttleHandler, counters, err := makeHandlerWrappedIntoMiddleware(cfg, blockCh, tags, false)
	require.NoError(t, err)
	var okCodes, unexpectedCodes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < reqsNum; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			respRec := httptest.NewRecorder()
			throttleHandler.ServeHTTP(respRec, reqsGen())
			switch respRec.Code {
			case http.StatusOK:
				okCodes.Inc()
			default:
				unexpectedCodes.Inc()
			}
		}()
	}
	time.Sleep(unblockDelay)
	for i := 0; i < reqsNum; i++ {
		require.NoError(t, waitRecv(blockCh, time.Second*5))
	}
	wg.Wait()

	require.Equal(t, reqsNum, int(counters.nextCalls.Load()))
	require.Equal(t, reqsNum, int(okCodes.Load()))
	require.Equal(t, 0, int(unexpectedCodes.Load()))
	counters.checkInFlightLimit(t, 0, wantDryRunRejects, 0)
}

func makeReqsGenerator(strReqs []string) func() *http.Request {
	var i atomic.Int32
	return func() *http.Request {
		j := int(i.Inc()) - 1
		reqParts := strings.SplitN(strReqs[j%len(strReqs)], " ", 2)
		return httptest.NewRequest(reqParts[0], reqParts[1], http.NoBody)
	}
}

func makeStrsGenerator(strs []string) func() string {
	var i atomic.Int32
	return func() string {
		j := int(i.Inc()) - 1
		res := strs[j%len(strs)]
		return res
	}
}

func makeFmtGenerator(format string) func() string {
	var i atomic.Int32
	return func() string {
		j := int(i.Inc()) - 1
		res := fmt.Sprintf(format, j)
		return res
	}
}

func waitRecv(ch chan struct{}, timeout time.Duration) error {
	select {
	case <-ch:
	case <-time.After(timeout):
		return fmt.Errorf("receive from channel: timeout %s exceeded", timeout)
	}
	return nil
}

func waitSend(ch chan struct{}, timeout time.Duration) error {
	select {
	case ch <- struct{}{}:
	case <-time.After(timeout):
		return fmt.Errorf("send to channel: timeout %s exceeded", timeout)
	}
	return nil
}

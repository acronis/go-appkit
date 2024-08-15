/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"github.com/acronis/go-appkit/testutil"
)

type mockHTTPRequestMetricsNextHandler struct {
	calledNum          int
	statusCodeToReturn int
}

func (h *mockHTTPRequestMetricsNextHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.calledNum++
	rw.WriteHeader(h.statusCodeToReturn)
}

func TestHttpRequestMetricsHandler_ServeHTTP(t *testing.T) {
	makeLabels := func(method, routePattern, uaType, statusCode string) prometheus.Labels {
		return prometheus.Labels{
			httpRequestMetricsLabelMethod:        method,
			httpRequestMetricsLabelRoutePattern:  routePattern,
			httpRequestMetricsLabelUserAgentType: uaType,
			httpRequestMetricsLabelStatusCode:    statusCode,
		}
	}

	getRoutePattern := func(r *http.Request) string {
		return r.URL.Path
	}

	t.Run("collect total number", func(t *testing.T) {
		tests := []struct {
			name               string
			method             string
			url                string
			userAgent          string
			statusCodeToReturn int
			reqsNum            int
			wantUserAgentType  string
			getUserAgentType   UserAgentTypeGetterFunc
			excludedEndpoints  []string
			curriedLabels      prometheus.Labels
		}{
			{
				name:               "GET request, user agent is not browser",
				method:             http.MethodGet,
				url:                "/hello",
				userAgent:          "agent1",
				statusCodeToReturn: http.StatusOK,
				reqsNum:            10,
				wantUserAgentType:  userAgentTypeHTTPClient,
			},
			{
				name:               "POST request, user agent is not browser",
				method:             http.MethodPost,
				url:                "/world",
				userAgent:          "agent2",
				statusCodeToReturn: http.StatusMethodNotAllowed,
				reqsNum:            11,
				wantUserAgentType:  userAgentTypeHTTPClient,
			},
			{
				name:               "DELETE request, user agent is browser",
				method:             http.MethodDelete,
				url:                "/admin",
				userAgent:          "Mozilla/5.0",
				statusCodeToReturn: http.StatusForbidden,
				reqsNum:            12,
				wantUserAgentType:  userAgentTypeBrowser,
			},
			{
				name:               "PUT request, custom func to parse user agent",
				method:             http.MethodPut,
				url:                "/hello",
				userAgent:          "my-service-http-client",
				statusCodeToReturn: http.StatusNoContent,
				reqsNum:            10,
				wantUserAgentType:  "my-service-agent",
				getUserAgentType: func(r *http.Request) string {
					if r.UserAgent() == "my-service-http-client" {
						return "my-service-agent"
					}
					return "http-client"
				},
			},
			{
				name:               "GET request, endpoint excluded",
				method:             http.MethodGet,
				url:                "/healthz",
				userAgent:          "k8s",
				statusCodeToReturn: http.StatusOK,
				reqsNum:            10,
				wantUserAgentType:  userAgentTypeHTTPClient,
				excludedEndpoints:  []string{"/healthz"},
			},
			{
				name:               "GET request, labels currying",
				method:             http.MethodGet,
				url:                "/hello-currying",
				userAgent:          "agent1",
				statusCodeToReturn: http.StatusOK,
				reqsNum:            10,
				wantUserAgentType:  userAgentTypeHTTPClient,
				curriedLabels:      prometheus.Labels{"extra1": "value1", "extra2": "value2"},
			},
		}
		for i := range tests {
			tt := tests[i]
			t.Run(tt.name, func(t *testing.T) {
				curriedLabelNames := make([]string, 0, len(tt.curriedLabels))
				for k := range tt.curriedLabels {
					curriedLabelNames = append(curriedLabelNames, k)
				}
				collector := NewHTTPRequestMetricsCollectorWithOpts(HTTPRequestMetricsCollectorOpts{
					CurriedLabelNames: curriedLabelNames,
				})
				collector = collector.MustCurryWith(tt.curriedLabels)
				mw := HTTPRequestMetricsWithOpts(collector, getRoutePattern, HTTPRequestMetricsOpts{
					GetUserAgentType:  tt.getUserAgentType,
					ExcludedEndpoints: tt.excludedEndpoints,
				})

				next := &mockHTTPRequestMetricsNextHandler{statusCodeToReturn: tt.statusCodeToReturn}
				h := mw(next)

				for j := 0; j < tt.reqsNum; j++ {
					req := httptest.NewRequest(tt.method, tt.url, nil)
					req.Header.Set("User-Agent", tt.userAgent)
					resp := httptest.NewRecorder()
					h.ServeHTTP(resp, req)
					assert.Equal(t, tt.statusCodeToReturn, resp.Code)
				}
				assert.Equal(t, tt.reqsNum, next.calledNum)

				labels := makeLabels(tt.method, tt.url, tt.wantUserAgentType, strconv.Itoa(tt.statusCodeToReturn))
				hist := collector.Durations.With(labels).(prometheus.Histogram)
				wantReqsNum := tt.reqsNum
				for _, exEndpoint := range tt.excludedEndpoints {
					if exEndpoint == tt.url {
						wantReqsNum = 0
						break
					}
				}
				testutil.AssertSamplesCountInHistogram(t, hist, wantReqsNum)
			})
		}
	})

	t.Run("collect 500 on panic", func(t *testing.T) {
		collector := NewHTTPRequestMetricsCollector()
		next := &mockRecoveryNextHandler{}
		req := httptest.NewRequest(http.MethodGet, "/internal-error", nil)
		resp := httptest.NewRecorder()
		h := HTTPRequestMetrics(collector, getRoutePattern)(next)
		if assert.Panics(t, func() { h.ServeHTTP(resp, req) }) {
			assert.Equal(t, 1, next.called)
			labels := makeLabels(http.MethodGet, "/internal-error", "http-client", "500")
			hist := collector.Durations.With(labels).(prometheus.Histogram)
			testutil.AssertSamplesCountInHistogram(t, hist, 1)
		}
	})
}

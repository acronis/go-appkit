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
	customLabels       map[string]string
}

func (h *mockHTTPRequestMetricsNextHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.calledNum++
	rw.WriteHeader(h.statusCodeToReturn)
	if h.customLabels != nil {
		mp := GetMetricsParamsFromContext(r.Context())
		if mp != nil {
			for k, v := range h.customLabels {
				mp.SetValue(k, v)
			}
		}
	}
}

type mockHTTPRequestMetricsDisabledHandler struct{}

func (h *mockHTTPRequestMetricsDisabledHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	DisableHTTPMetricsInContext(r.Context())
	rw.WriteHeader(http.StatusOK)
}

func TestHttpRequestMetricsHandler_ServeHTTP(t *testing.T) {
	makeLabels := func(method, routePattern, uaType, statusCode string, customLabels map[string]string) prometheus.Labels {
		labels := make(prometheus.Labels, 4+len(customLabels))
		labels[httpRequestMetricsLabelMethod] = method
		labels[httpRequestMetricsLabelRoutePattern] = routePattern
		labels[httpRequestMetricsLabelUserAgentType] = uaType
		labels[httpRequestMetricsLabelStatusCode] = statusCode
		for k, v := range customLabels {
			labels[k] = v
		}
		return labels
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
			customLabels       map[string]string
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
			{
				name:               "GET request, custom labels",
				method:             http.MethodGet,
				url:                "/hello-currying",
				userAgent:          "agent1",
				statusCodeToReturn: http.StatusOK,
				reqsNum:            10,
				wantUserAgentType:  userAgentTypeHTTPClient,
				customLabels:       map[string]string{"custom1": "value1", "custom2": "value2"},
			},
			{
				name:               "GET request, custom labels, labels currying",
				method:             http.MethodGet,
				url:                "/hello-currying",
				userAgent:          "agent1",
				statusCodeToReturn: http.StatusOK,
				reqsNum:            10,
				wantUserAgentType:  userAgentTypeHTTPClient,
				curriedLabels:      prometheus.Labels{"extra1": "value1", "extra2": "value2"},
				customLabels:       map[string]string{"custom1": "value1", "custom2": "value2"},
			},
		}
		for i := range tests {
			tt := tests[i]
			t.Run(tt.name, func(t *testing.T) {
				curriedLabelNames := make([]string, 0, len(tt.curriedLabels))
				for k := range tt.curriedLabels {
					curriedLabelNames = append(curriedLabelNames, k)
				}
				customLabelNames := make([]string, 0, len(tt.customLabels))
				for k := range tt.customLabels {
					customLabelNames = append(customLabelNames, k)
				}
				collector := NewHTTPRequestPrometheusMetricsWithOpts(HTTPRequestPrometheusMetricsOpts{
					CurriedLabelNames: curriedLabelNames,
					CustomLabelNames:  customLabelNames,
				})
				collector = collector.MustCurryWith(tt.curriedLabels)
				mw := HTTPRequestMetricsWithOpts(collector, getRoutePattern, HTTPRequestMetricsOpts{
					GetUserAgentType:  tt.getUserAgentType,
					ExcludedEndpoints: tt.excludedEndpoints,
				})

				next := &mockHTTPRequestMetricsNextHandler{statusCodeToReturn: tt.statusCodeToReturn, customLabels: tt.customLabels}
				h := mw(next)

				for j := 0; j < tt.reqsNum; j++ {
					req := httptest.NewRequest(tt.method, tt.url, nil)
					req.Header.Set("User-Agent", tt.userAgent)
					resp := httptest.NewRecorder()
					h.ServeHTTP(resp, req)
					assert.Equal(t, tt.statusCodeToReturn, resp.Code)
				}
				assert.Equal(t, tt.reqsNum, next.calledNum)

				labels := makeLabels(tt.method, tt.url, tt.wantUserAgentType, strconv.Itoa(tt.statusCodeToReturn), tt.customLabels)
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
		promMetrics := NewHTTPRequestPrometheusMetrics()
		next := &mockRecoveryNextHandler{}
		req := httptest.NewRequest(http.MethodGet, "/internal-error", nil)
		resp := httptest.NewRecorder()
		h := HTTPRequestMetrics(promMetrics, getRoutePattern)(next)
		if assert.Panics(t, func() { h.ServeHTTP(resp, req) }) {
			assert.Equal(t, 1, next.called)
			labels := makeLabels(http.MethodGet, "/internal-error", "http-client", "500", nil)
			hist := promMetrics.Durations.With(labels).(prometheus.Histogram)
			testutil.AssertSamplesCountInHistogram(t, hist, 1)
		}
	})

	t.Run("not collect if disabled", func(t *testing.T) {
		promMetrics := NewHTTPRequestPrometheusMetrics()
		next := &mockHTTPRequestMetricsDisabledHandler{}
		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		req.Header.Set("User-Agent", "http-client")
		resp := httptest.NewRecorder()
		h := HTTPRequestMetrics(promMetrics, getRoutePattern)(next)
		h.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusOK, resp.Code)
		labels := makeLabels(http.MethodGet, "/hello", "http-client", "200", nil)
		hist := promMetrics.Durations.With(labels).(prometheus.Histogram)
		testutil.AssertSamplesCountInHistogram(t, hist, 0)
	})
}

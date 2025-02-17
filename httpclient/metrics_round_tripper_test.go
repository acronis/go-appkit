/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/testutil"
)

func TestNewMetricsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	collector := NewPrometheusMetricsCollector("")
	defer collector.Unregister()

	clientType := "test-client-type"

	metricsRoundTripper := NewMetricsRoundTripperWithOpts(http.DefaultTransport, collector, MetricsRoundTripperOpts{
		ClientType: clientType,
	})
	client := &http.Client{Transport: metricsRoundTripper}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)

	labels := prometheus.Labels{
		"client_type":    clientType,
		"remote_address": strings.ReplaceAll(server.URL, "http://", ""),
		"summary":        "POST test-client-type",
		"request_type":   "",
		"status":         "418",
	}
	hist := collector.Durations.With(labels).(prometheus.Histogram)
	testutil.AssertSamplesCountInHistogram(t, hist, 1)
}

func TestNewMetricsCollectionRequiredRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	metricsRoundTripper := NewMetricsRoundTripper(http.DefaultTransport, nil)
	client := &http.Client{Transport: metricsRoundTripper}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	_, err = client.Do(req) // nolint:bodyclose
	require.Error(t, err)
	require.Contains(t, err.Error(), "metrics collector is not provided")
}

func TestNewMetricsRequestTypeRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	collector := NewPrometheusMetricsCollector("")
	defer collector.Unregister()

	clientType := "test-client-type"
	requestType := "test-request-type"

	metricsRoundTripper := NewMetricsRoundTripperWithOpts(http.DefaultTransport, collector, MetricsRoundTripperOpts{
		ClientType: clientType,
	})
	client := &http.Client{Transport: metricsRoundTripper}
	ctx := NewContextWithRequestType(context.Background(), requestType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)

	labels := prometheus.Labels{
		"client_type":    clientType,
		"remote_address": strings.ReplaceAll(server.URL, "http://", ""),
		"summary":        "POST test-client-type",
		"request_type":   requestType,
		"status":         "418",
	}
	hist := collector.Durations.With(labels).(prometheus.Histogram)
	testutil.AssertSamplesCountInHistogram(t, hist, 1)
}

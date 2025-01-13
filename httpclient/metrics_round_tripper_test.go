/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"github.com/acronis/go-appkit/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestNewMetricsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	collector := NewPrometheusMetricsCollector("")
	defer collector.Unregister()

	metricsRoundTripper := NewMetricsRoundTripperWithOpts(http.DefaultTransport, collector, MetricsRoundTripperOpts{
		ClientType: "test-client-type",
	})
	client := &http.Client{Transport: metricsRoundTripper}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)

	labels := prometheus.Labels{
		"client_type":    "test-client-type",
		"remote_address": strings.ReplaceAll(server.URL, "http://", ""),
		"summary":        "POST test-client-type",
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

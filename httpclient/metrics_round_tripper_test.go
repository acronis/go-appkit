/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewMetricsRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusTeapot)
	}))
	defer server.Close()

	collector := NewPrometheusMetricsCollector("")
	defer collector.Unregister()

	metricsRoundTripper := NewMetricsRoundTripperWithOpts(http.DefaultTransport, MetricsRoundTripperOpts{
		ReqType:   "test-request",
		Collector: collector,
	})
	client := &http.Client{Transport: metricsRoundTripper}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, nil)
	require.NoError(t, err)

	r, err := client.Do(req)
	defer func() { _ = r.Body.Close() }()
	require.NoError(t, err)

	ch := make(chan prometheus.Metric, 1)
	go func() {
		collector.Durations.Collect(ch)
		close(ch)
	}()

	var metricCount int
	for range ch {
		metricCount++
	}

	require.Equal(t, metricCount, 1)
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertSamplesCountInHistogram asserts that passed prometheus.Histogram contains the specified number of samples.
func AssertSamplesCountInHistogram(t assert.TestingT, hist prometheus.Histogram, wantSamplesCount int) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	reg := prometheus.NewPedanticRegistry()
	if !assert.NoError(t, reg.Register(hist)) {
		return false
	}
	gotMetrics, err := reg.Gather()
	if !assert.NoError(t, err) {
		return false
	}
	if !assert.Equal(t, 1, len(gotMetrics)) {
		return false
	}
	return assert.Equal(t, wantSamplesCount, int(gotMetrics[0].GetMetric()[0].Histogram.GetSampleCount()))
}

// RequireSamplesCountInHistogram calls AssertSamplesCountInHistogram and fail test immediately in case of error.
func RequireSamplesCountInHistogram(t require.TestingT, hist prometheus.Histogram, wantSamplesCount int) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if AssertSamplesCountInHistogram(t, hist, wantSamplesCount) {
		return
	}
	t.FailNow()
}

// AssertSamplesCountInCounter asserts that passed prometheus.Counter has proper value.
func AssertSamplesCountInCounter(t assert.TestingT, counter prometheus.Counter, wantCount int) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	reg := prometheus.NewPedanticRegistry()
	if !assert.NoError(t, reg.Register(counter)) {
		return false
	}
	gotMetrics, err := reg.Gather()
	if !assert.NoError(t, err) {
		return false
	}
	if !assert.Equal(t, 1, len(gotMetrics)) {
		return false
	}
	return assert.Equal(t, wantCount, int(gotMetrics[0].GetMetric()[0].GetCounter().GetValue()))
}

// RequireSamplesCountInCounter calls AssertSamplesCountInCounter and fail test immediately in case of error.
func RequireSamplesCountInCounter(t require.TestingT, counter prometheus.Counter, wantCount int) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if AssertSamplesCountInCounter(t, counter, wantCount) {
		return
	}
	t.FailNow()
}

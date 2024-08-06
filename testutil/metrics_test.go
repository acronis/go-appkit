/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestRequireSamplesCountInCounter(t *testing.T) {
	eventsCounter := prometheus.NewCounter(prometheus.CounterOpts{Name: "events"})
	eventsCounter.Add(42)

	mockT := &MockT{}
	RequireSamplesCountInCounter(mockT, eventsCounter, 41)
	require.True(t, mockT.Failed)

	mockT = &MockT{}
	RequireSamplesCountInCounter(mockT, eventsCounter, 42)
	require.False(t, mockT.Failed)
}

func TestRequireSamplesCountInHistogram(t *testing.T) {
	eventsHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{Name: "events", Buckets: []float64{1, 10, 20, 30, 40, 50}})
	eventsHistogram.Observe(42)

	mockT := &MockT{}
	RequireSamplesCountInHistogram(mockT, eventsHistogram, 0)
	require.True(t, mockT.Failed)

	mockT = &MockT{}
	RequireSamplesCountInHistogram(mockT, eventsHistogram, 1)
	require.False(t, mockT.Failed)
}

/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricsParams_SetValue(t *testing.T) {
	tests := []struct {
		name           string
		values         map[string]string
		addName        string
		addValue       string
		expectedValues map[string]string
	}{
		{
			name:           "add label to empty params",
			values:         nil,
			addName:        "service",
			addValue:       "api",
			expectedValues: map[string]string{"service": "api"},
		},
		{
			name:           "add label to existing params",
			values:         map[string]string{"env": "prod"},
			addName:        "service",
			addValue:       "api",
			expectedValues: map[string]string{"env": "prod", "service": "api"},
		},
		{
			name:           "overwrite existing label",
			values:         map[string]string{"service": "old"},
			addName:        "service",
			addValue:       "new",
			expectedValues: map[string]string{"service": "new"},
		},
		{
			name:           "add empty value",
			values:         nil,
			addName:        "service",
			addValue:       "",
			expectedValues: map[string]string{"service": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := &MetricsParams{values: tt.values}
			mp.SetValue(tt.addName, tt.addValue)
			assert.Equal(t, tt.expectedValues, mp.values)
		})
	}
}

func TestMetricsParams_MultipleOperations(t *testing.T) {
	mp := &MetricsParams{}

	// Initially empty
	assert.Nil(t, mp.values)

	// Add first label
	mp.SetValue("service", "api")
	assert.Equal(t, map[string]string{"service": "api"}, mp.values)

	// Add second label
	mp.SetValue("env", "prod")
	assert.Equal(t, map[string]string{"service": "api", "env": "prod"}, mp.values)

	// Add third label
	mp.SetValue("version", "1.0")
	assert.Equal(t, map[string]string{"service": "api", "env": "prod", "version": "1.0"}, mp.values)

	// Overwrite existing label
	mp.SetValue("env", "staging")
	assert.Equal(t, map[string]string{"service": "api", "env": "staging", "version": "1.0"}, mp.values)
}

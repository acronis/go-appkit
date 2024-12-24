/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"github.com/acronis/go-appkit/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConfigWithLoader(t *testing.T) {
	yamlData := []byte(`
retries:
  enabled: true
  maxAttempts: 30
rateLimits:
  enabled: true
  limit: 300
  burst: 3000
  waitTimeout: 3s
logger:
  enabled: true
  slowRequestThreshold: 5s
  mode: all
metrics:
  enabled: true
timeout: 30s
`)

	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.NoError(t, err, "load configuration")

	expectedConfig := &Config{
		Retries: RetriesConfig{
			Enabled:     true,
			MaxAttempts: 30,
		},
		RateLimits: RateLimitConfig{
			Enabled:     true,
			Limit:       300,
			Burst:       3000,
			WaitTimeout: 3 * time.Second,
		},
		Logger: LoggerConfig{
			Enabled:              true,
			SlowRequestThreshold: 5 * time.Second,
			Mode:                 "all",
		},
		Metrics: MetricsConfig{Enabled: true},
		Timeout: 30 * time.Second,
	}

	require.Equal(t, expectedConfig, actualConfig, "configuration does not match expected")
}

func TestConfigRateLimitInvalid(t *testing.T) {
	yamlData := []byte(`
retries:
  enabled: true
  maxAttempts: 30
rateLimits:
  enabled: true
  limit: -300
  burst: 3000
  waitTimeout: 3s
timeout: 30s
`)

	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client rate limit must be positive", err.Error())

	yamlData = []byte(`
retries:
  enabled: true
  maxAttempts: 30
rateLimits:
  enabled: true
  limit: 300
  burst: -3
  waitTimeout: 3s
timeout: 30s
`)

	actualConfig = &Config{}
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client burst must be positive", err.Error())

	yamlData = []byte(`
retries:
  enabled: true
  maxAttempts: 30
rateLimits:
  enabled: true
  limit: 300
  burst: 3
  waitTimeout: -3s
timeout: 30s
`)

	actualConfig = &Config{}
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client wait timeout must be positive", err.Error())
}

func TestConfigRetriesInvalid(t *testing.T) {
	yamlData := []byte(`
retries:
  enabled: true
  maxAttempts: -30
timeout: 30s
`)

	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client max retry attempts must be positive", err.Error())
}

func TestConfigLoggerInvalid(t *testing.T) {
	yamlData := []byte(`
retries:
  enabled: true
  maxAttempts: 30
rateLimits:
  enabled: true
  limit: 300
  burst: 3000
  waitTimeout: 3s
logger:
  enabled: true
  slowRequestThreshold: -5s
  mode: all
metrics:
  enabled: true
timeout: 30s
`)

	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client logger slow request threshold can not be negative", err.Error())

	yamlData = []byte(`
retries:
  enabled: true
  maxAttempts: 30
rateLimits:
  enabled: true
  limit: 300
  burst: 3000
  waitTimeout: 3s
logger:
  enabled: true
  slowRequestThreshold: 5s
  mode: invalid
metrics:
  enabled: true
timeout: 30s
`)

	actualConfig = &Config{}
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client logger invalid mode, choose one of: [none, all, failed]", err.Error())
}

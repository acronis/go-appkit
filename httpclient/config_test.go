/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/retry"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestConfigWithLoader(t *testing.T) {
	yamlData := testYamlData(nil)
	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.NoError(t, err, "load configuration")

	expectedConfig := &Config{
		Retries: RetriesConfig{
			Enabled:     true,
			MaxAttempts: 30,
			Policy: PolicyConfig{
				Strategy:                          RetryPolicyExponential,
				ExponentialBackoffInitialInterval: 3 * time.Second,
				ExponentialBackoffMultiplier:      2,
			},
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

func TestConfigRateLimit(t *testing.T) {
	yamlData := testYamlData([][]string{{"limit: 300", "limit: -300"}})
	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client rate limit must be positive", err.Error())

	yamlData = testYamlData([][]string{{"burst: 3000", "burst: -3"}})
	actualConfig = &Config{}
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client burst must be positive", err.Error())

	yamlData = testYamlData([][]string{{"waitTimeout: 3s", "waitTimeout: -3s"}})
	actualConfig = &Config{}
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client wait timeout must be positive", err.Error())
}

func TestConfigRetries(t *testing.T) {
	yamlData := testYamlData([][]string{{"maxAttempts: 30", "maxAttempts: -30"}})
	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client max retry attempts must be positive", err.Error())
}

func TestConfigLogger(t *testing.T) {
	yamlData := testYamlData([][]string{{"slowRequestThreshold: 5s", "slowRequestThreshold: -5s"}})
	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client logger slow request threshold can not be negative", err.Error())

	yamlData = testYamlData([][]string{{"mode: all", "mode: invalid"}})
	actualConfig = &Config{}
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client logger invalid mode, choose one of: [none, all, failed]", err.Error())
}

func TestConfigRetriesPolicy(t *testing.T) {
	yamlData := testYamlData([][]string{{"strategy: exponential", "strategy: invalid"}})
	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client retry policy must be one of: [exponential, constant]", err.Error())

	yamlData = testYamlData([][]string{
		{"exponentialBackoffInitialInterval: 3s", "exponentialBackoffInitialInterval: -1s"},
	})
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client exponential backoff initial interval must be positive", err.Error())

	yamlData = testYamlData([][]string{{"exponentialBackoffMultiplier: 2", "exponentialBackoffMultiplier: 1"}})
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client exponential backoff multiplier must be greater than 1", err.Error())

	yamlData = testYamlData([][]string{
		{"strategy: exponential", "strategy: constant"},
		{"constantBackoffInterval: 2s", "constantBackoffInterval: -3s"},
	})
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.Error(t, err)
	require.Equal(t, "client constant backoff interval must be positive", err.Error())

	yamlData = testYamlData([][]string{
		{"strategy: exponential", "strategy:"},
	})
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.NoError(t, err)
	require.Nil(t, actualConfig.Retries.GetPolicy())

	yamlData = testYamlData(nil)
	err = config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.NoError(t, err)
	require.Implements(t, (*retry.Policy)(nil), actualConfig.Retries.GetPolicy())
}

func TestConfigDisableWithLoader(t *testing.T) {
	yamlData := []byte(`
retries:
  enabled: false
rateLimits:
  enabled: false
logger:
  enabled: false
metrics:
  enabled: false
timeout: 30s
`)
	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.NoError(t, err)
}

func testYamlData(replacements [][]string) []byte {
	yamlData := `
retries:
  enabled: true
  maxAttempts: 30
  policy:
    strategy: exponential
    exponentialBackoffInitialInterval: 3s
    exponentialBackoffMultiplier: 2
    constantBackoffInterval: 2s
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
`
	for i := range replacements {
		yamlData = strings.Replace(yamlData, replacements[i][0], replacements[i][1], 1)
	}

	return []byte(yamlData)
}

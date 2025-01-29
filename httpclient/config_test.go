/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
)

type AppConfig struct {
	HTTPClient *Config `mapstructure:"httpClient" json:"httpClient" yaml:"httpClient"`
}

func TestConfig(t *testing.T) {
	expectedAppCfg := AppConfig{HTTPClient: NewDefaultConfig(WithKeyPrefix("httpClient"))}
	expectedAppCfg.HTTPClient.Retries = RetriesConfig{
		Enabled:     true,
		MaxAttempts: 30,
		Policy:      RetryPolicyExponential,
		ExponentialBackoff: ExponentialBackoffConfig{
			InitialInterval: config.TimeDuration(3 * time.Second),
			Multiplier:      2,
		},
	}
	expectedAppCfg.HTTPClient.RateLimits = RateLimitsConfig{
		Enabled:     true,
		Limit:       300,
		Burst:       3000,
		WaitTimeout: config.TimeDuration(3 * time.Second),
	}
	expectedAppCfg.HTTPClient.Log = LogConfig{
		Enabled:              true,
		SlowRequestThreshold: config.TimeDuration(5 * time.Second),
		Mode:                 "all",
	}
	expectedAppCfg.HTTPClient.Metrics = MetricsConfig{Enabled: true}
	expectedAppCfg.HTTPClient.Timeout = config.TimeDuration(30 * time.Second)

	tests := []struct {
		name        string
		cfgDataType config.DataType
		cfgData     string
	}{
		{
			name:        "yaml config",
			cfgDataType: config.DataTypeYAML,
			cfgData: `
httpClient:
  retries:
    enabled: true
    maxAttempts: 30
    policy: exponential
    exponentialBackoff:
      initialInterval: 3s
      multiplier: 2
  rateLimits:
    enabled: true
    limit: 300
    burst: 3000
    waitTimeout: 3s
  log:
    enabled: true
    slowRequestThreshold: 5s
    mode: all
  metrics:
    enabled: true
  timeout: 30s
`,
		},
		{
			name:        "json config",
			cfgDataType: config.DataTypeJSON,
			cfgData: `
{
	"httpClient": {
		"retries": {
			"enabled": true,
			"maxAttempts": 30,
			"policy": "exponential",
			"exponentialBackoff": {
				"initialInterval": "3s",
				"multiplier": 2
			}
		},
		"rateLimits": {
			"enabled": true,
			"limit": 300,
			"burst": 3000,
			"waitTimeout": "3s"
		},
		"log": {
			"enabled": true,
			"slowRequestThreshold": "5s",
			"mode": "all"
		},
		"metrics": {
			"enabled": true
		},
		"timeout": "30s"
	}
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var appCfg AppConfig

			// Load config using config.Loader.
			appCfg = AppConfig{HTTPClient: NewDefaultConfig(WithKeyPrefix("httpClient"))}
			cfgLoader := config.NewLoader(config.NewViperAdapter())
			err := cfgLoader.LoadFromReader(bytes.NewBuffer([]byte(tt.cfgData)), tt.cfgDataType, appCfg.HTTPClient)
			require.NoError(t, err)
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using viper unmarshal.
			appCfg = AppConfig{HTTPClient: NewDefaultConfig(WithKeyPrefix("httpClient"))}
			vpr := viper.New()
			vpr.SetConfigType(string(tt.cfgDataType))
			require.NoError(t, vpr.ReadConfig(bytes.NewBuffer([]byte(tt.cfgData))))
			require.NoError(t, vpr.Unmarshal(&appCfg, func(c *mapstructure.DecoderConfig) {
				c.DecodeHook = mapstructure.TextUnmarshallerHookFunc()
			}))
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using yaml/json unmarshal.
			appCfg = AppConfig{HTTPClient: NewDefaultConfig(WithKeyPrefix("httpClient"))}
			switch tt.cfgDataType {
			case config.DataTypeYAML:
				require.NoError(t, yaml.Unmarshal([]byte(tt.cfgData), &appCfg))
				require.Equal(t, expectedAppCfg, appCfg)
			case config.DataTypeJSON:
				require.NoError(t, json.Unmarshal([]byte(tt.cfgData), &appCfg))
				require.Equal(t, expectedAppCfg, appCfg)
			default:
				t.Fatalf("unsupported config data type: %s", tt.cfgDataType)
			}
		})
	}
}

func TestNewDefaultConfig(t *testing.T) {
	var cfg *Config

	// Empty config, all defaults for the data provider should be used
	cfg = NewConfig()
	require.NoError(t, config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer(nil), config.DataTypeYAML, cfg))
	require.Equal(t, NewDefaultConfig(), cfg)

	// viper.Unmarshal
	cfg = NewDefaultConfig()
	vpr := viper.New()
	vpr.SetConfigType("yaml")
	require.NoError(t, vpr.Unmarshal(&cfg))
	require.Equal(t, NewDefaultConfig(), cfg)

	// yaml.Unmarshal
	cfg = NewDefaultConfig()
	require.NoError(t, yaml.Unmarshal([]byte(""), &cfg))
	require.Equal(t, NewDefaultConfig(), cfg)

	// json.Unmarshal
	cfg = NewDefaultConfig()
	require.NoError(t, json.Unmarshal([]byte("{}"), &cfg))
	require.Equal(t, NewDefaultConfig(), cfg)
}

func TestConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		yamlData       string
		expectedErrMsg string
	}{
		{
			name: "error, retries maxAttempts must be positive",
			yamlData: `
httpClient:
  retries:
    enabled: true
    maxAttempts: -1
`,
			expectedErrMsg: `httpClient.retries.maxAttempts: must be positive`,
		},
		{
			name: "error, rateLimits limit must be positive",
			yamlData: `
httpClient:
  rateLimits:
    enabled: true
    limit: -1
`,
			expectedErrMsg: `httpClient.rateLimits.limit: must be positive`,
		},
		{
			name: "error, rateLimits burst must be positive",
			yamlData: `
httpClient:
  rateLimits:
    enabled: true
    limit: 10
    burst: -1
`,
			expectedErrMsg: `httpClient.rateLimits.burst: must be positive`,
		},
		{
			name: "error, rateLimits waitTimeout must be positive",
			yamlData: `
httpClient:
  rateLimits:
    enabled: true
    limit: 10
    waitTimeout: -1s
`,
			expectedErrMsg: `httpClient.rateLimits.waitTimeout: must be positive`,
		},
		{
			name: "error, log slowRequestThreshold can not be negative",
			yamlData: `
httpClient:
  log:
    enabled: true
    slowRequestThreshold: -1s
`,
			expectedErrMsg: `httpClient.log.slowRequestThreshold: can not be negative`,
		},
		{
			name: "error, unknown log mode",
			yamlData: `
httpClient:
  log:
    enabled: true
    mode: invalid-mode
`,
			expectedErrMsg: `httpClient.log.mode: choose one of: [all, failed]`,
		},
		{
			name: "error, unknown retries policy",
			yamlData: `
httpClient:
  retries:
    enabled: true
    policy: invalid-policy
`,
			expectedErrMsg: `httpClient.retries.policy: must be one of: [exponential, constant]`,
		},
		{
			name: "error, exponentialBackoff initialInterval must be positive",
			yamlData: `
httpClient:
  retries:
    enabled: true
    policy: exponential
    exponentialBackoff:
      initialInterval: -1s
`,
			expectedErrMsg: `httpClient.retries.exponentialBackoff.initialInterval: must be positive`,
		},
		{
			name: "error, exponentialBackoff multiplier must be greater than 1",
			yamlData: `
httpClient:
  retries:
    enabled: true
    policy: exponential
    exponentialBackoff:
      multiplier: 1
`,
			expectedErrMsg: `httpClient.retries.exponentialBackoff.multiplier: must be greater than 1`,
		},
		{
			name: "error, constantBackoff interval must be positive",
			yamlData: `
httpClient:
  retries:
    enabled: true
    policy: constant
    constantBackoff:
      interval: -1s
`,
			expectedErrMsg: `httpClient.retries.constantBackoff.interval: must be positive`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(WithKeyPrefix("httpClient"))
			err := config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer([]byte(tt.yamlData)), config.DataTypeYAML, cfg)
			require.EqualError(t, err, tt.expectedErrMsg)
		})
	}
}

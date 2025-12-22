/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
)

type AppConfig struct {
	Server *Config `mapstructure:"server" json:"server" yaml:"server"`
}

func TestConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfgDataType config.DataType
		cfgData     string
		expectedCfg func() *Config
	}{
		{
			name:        "yaml config",
			cfgDataType: config.DataTypeYAML,
			cfgData: `
server:
  address: "127.0.0.1:8080"
  timeouts:
    write: 1h
    read: 7m
    readHeader: 1m
    idle: 20m
    shutdown: 30s
  limits:
    maxRequests: 10
    maxBodySize: 1M
  log:
    requestStart: true
    slowRequestThreshold: 2s
    timeSlotsThreshold: 500ms
  tls:
    enabled: true
    cert: "/test/path"
    key: "/test/path"
`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Address = "127.0.0.1:8080"
				cfg.Timeouts.Write = config.TimeDuration(time.Hour)
				cfg.Timeouts.Read = config.TimeDuration(time.Minute * 7)
				cfg.Timeouts.ReadHeader = config.TimeDuration(time.Minute)
				cfg.Timeouts.Idle = config.TimeDuration(time.Minute * 20)
				cfg.Timeouts.Shutdown = config.TimeDuration(time.Second * 30)
				cfg.Limits.MaxRequests = 10
				cfg.Limits.MaxBodySizeBytes = 1024 * 1024
				cfg.Log.RequestStart = true
				cfg.Log.SlowRequestThreshold = config.TimeDuration(2 * time.Second)
				cfg.Log.TimeSlotsThreshold = config.TimeDuration(500 * time.Millisecond)
				cfg.TLS.Enabled = true
				cfg.TLS.Certificate = "/test/path"
				cfg.TLS.Key = "/test/path"
				return cfg
			},
		},
		{
			name:        "json config",
			cfgDataType: config.DataTypeJSON,
			cfgData: `
{
	"server": {
		"address": "127.0.0.1:8080",
		"timeouts": {
			"write": "1h",
			"read": "7m",
			"readHeader": "1m",
			"idle": "20m",
			"shutdown": "30s"
		},
		"limits": {
			"maxRequests": 10,
			"maxBodySize": "1M"
		},
		"log": {
			"requestStart": true
		},
		"tls": {
			"enabled": true,
			"cert": "/test/path",
			"key": "/test/path"
		}
	}
}`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Address = "127.0.0.1:8080"
				cfg.Timeouts.Write = config.TimeDuration(time.Hour)
				cfg.Timeouts.Read = config.TimeDuration(time.Minute * 7)
				cfg.Timeouts.ReadHeader = config.TimeDuration(time.Minute)
				cfg.Timeouts.Idle = config.TimeDuration(time.Minute * 20)
				cfg.Timeouts.Shutdown = config.TimeDuration(time.Second * 30)
				cfg.Limits.MaxRequests = 10
				cfg.Limits.MaxBodySizeBytes = 1024 * 1024
				cfg.Log.RequestStart = true
				cfg.TLS.Enabled = true
				cfg.TLS.Certificate = "/test/path"
				cfg.TLS.Key = "/test/path"
				return cfg
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load config using config.Loader.
			appCfg := AppConfig{Server: NewDefaultConfig()}
			expectedAppCfg := AppConfig{Server: tt.expectedCfg()}
			cfgLoader := config.NewLoader(config.NewViperAdapter())
			err := cfgLoader.LoadFromReader(bytes.NewBuffer([]byte(tt.cfgData)), tt.cfgDataType, appCfg.Server)
			require.NoError(t, err)
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using viper unmarshal.
			appCfg = AppConfig{Server: NewDefaultConfig()}
			expectedAppCfg = AppConfig{Server: tt.expectedCfg()}
			vpr := viper.New()
			vpr.SetConfigType(string(tt.cfgDataType))
			require.NoError(t, vpr.ReadConfig(bytes.NewBuffer([]byte(tt.cfgData))))
			require.NoError(t, vpr.Unmarshal(&appCfg, func(c *mapstructure.DecoderConfig) {
				c.DecodeHook = mapstructure.TextUnmarshallerHookFunc()
			}))
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using yaml/json unmarshal.
			appCfg = AppConfig{Server: NewDefaultConfig()}
			expectedAppCfg = AppConfig{Server: tt.expectedCfg()}
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

func TestWithKeyPrefix(t *testing.T) {
	t.Run("custom key prefix", func(t *testing.T) {
		cfgData := `
customServer:
  address: "127.0.0.1:9999"
`
		expectedCfg := NewDefaultConfig(WithKeyPrefix("customServer"))
		expectedCfg.Address = "127.0.0.1:9999"

		cfg := NewConfig(WithKeyPrefix("customServer"))
		err := config.NewLoader(config.NewViperAdapter()).LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
		require.NoError(t, err)
		require.Equal(t, expectedCfg, cfg)
	})

	t.Run("default key prefix, empty struct initialization", func(t *testing.T) {
		cfgData := `
server:
  address: "127.0.0.1:9999"
`
		cfg := &Config{}
		err := config.NewLoader(config.NewViperAdapter()).LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1:9999", cfg.Address)
	})
}

func TestConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		yamlData       string
		expectedErrMsg string
	}{
		{
			name: "error, invalid address",
			yamlData: `
server:
  address: []
`,
			expectedErrMsg: `server.address: unable to cast`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			err := config.NewLoader(config.NewViperAdapter()).LoadFromReader(bytes.NewBuffer([]byte(tt.yamlData)), config.DataTypeYAML, cfg)
			require.ErrorContains(t, err, tt.expectedErrMsg)
		})
	}
}

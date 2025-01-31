/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package profserver

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
)

type AppConfig struct {
	ProfServer *Config `mapstructure:"profServer" json:"profServer" yaml:"profServer"`
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
profServer:
  enabled: false
  address: "0.0.0.0:6060"
`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Enabled = false
				cfg.Address = "0.0.0.0:6060"
				return cfg
			},
		},
		{
			name:        "json config",
			cfgDataType: config.DataTypeJSON,
			cfgData: `
{
	"profServer": {
		"enabled": false,
		"address": "0.0.0.0:6060"
	}
}`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Enabled = false
				cfg.Address = "0.0.0.0:6060"
				return cfg
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load config using config.Loader.
			appCfg := AppConfig{ProfServer: NewDefaultConfig()}
			expectedAppCfg := AppConfig{ProfServer: tt.expectedCfg()}
			cfgLoader := config.NewLoader(config.NewViperAdapter())
			err := cfgLoader.LoadFromReader(bytes.NewBuffer([]byte(tt.cfgData)), tt.cfgDataType, appCfg.ProfServer)
			require.NoError(t, err)
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using viper unmarshal.
			appCfg = AppConfig{ProfServer: NewDefaultConfig()}
			expectedAppCfg = AppConfig{ProfServer: tt.expectedCfg()}
			vpr := viper.New()
			vpr.SetConfigType(string(tt.cfgDataType))
			require.NoError(t, vpr.ReadConfig(bytes.NewBuffer([]byte(tt.cfgData))))
			require.NoError(t, vpr.Unmarshal(&appCfg))
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using yaml/json unmarshal.
			appCfg = AppConfig{ProfServer: NewDefaultConfig()}
			expectedAppCfg = AppConfig{ProfServer: tt.expectedCfg()}
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
	cfgData := `
customProfServer:
  enabled: true
  address: "127.0.0.1:7070"
`
	expectedCfg := NewDefaultConfig(WithKeyPrefix("customProfServer"))
	expectedCfg.Enabled = true
	expectedCfg.Address = "127.0.0.1:7070"

	cfg := NewConfig(WithKeyPrefix("customProfServer"))
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
	require.NoError(t, err)
	require.Equal(t, expectedCfg, cfg)
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
profServer:
  address: ""
`,
			expectedErrMsg: `profServer.address: cannot be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			err := config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer([]byte(tt.yamlData)), config.DataTypeYAML, cfg)
			require.EqualError(t, err, tt.expectedErrMsg)
		})
	}
}

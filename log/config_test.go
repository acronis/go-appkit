/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
)

type AppConfig struct {
	Log *Config `mapstructure:"log" json:"log" yaml:"log"`
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
log:
  level: warn
  format: text
  output: file
  file:
    path: my-service.log
    rotation:
      compress: true
      maxSize: 100M
      maxBackups: 42
  addCaller: true
  error:
    noVerbose: true
    verboseSuffix: test-suffix
`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Level = LevelWarn
				cfg.Format = FormatText
				cfg.Output = OutputFile
				cfg.File.Path = "my-service.log"
				cfg.File.Rotation.MaxSize = 100 * 1024 * 1024
				cfg.File.Rotation.MaxBackups = 42
				cfg.File.Rotation.Compress = true
				cfg.AddCaller = true
				cfg.Error.NoVerbose = true
				cfg.Error.VerboseSuffix = "test-suffix"
				return cfg
			},
		},
		{
			name:        "default yaml config with masking rules",
			cfgDataType: config.DataTypeYAML,
			cfgData: `
log:
  masking:
    enabled: true
    rules:
      - field: "api_key"
        formats: ["http_header", "json", "urlencoded"]
        masks:
          - regexp: "<api_key>.+?</api_key>"
            mask: "<api_key>***</api_key>"
`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Masking.Enabled = true
				cfg.Masking.Rules = []MaskingRuleConfig{
					{
						Field:   "api_key",
						Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader, FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
						Masks: []MaskConfig{
							{
								RegExp: "<api_key>.+?</api_key>",
								Mask:   "<api_key>***</api_key>",
							},
						},
					},
				}
				return cfg
			},
		},
		{
			name:        "json config",
			cfgDataType: config.DataTypeJSON,
			cfgData: `
{
	"log": {
		"level": "error",
		"format": "text",
		"output": "file",
		"file": {
			"path": "my-service.log",
			"rotation": {
				"compress": true,
				"maxSize": "100M",
				"maxBackups": 42
			}
		},
		"addCaller": true,
		"error": {
			"noVerbose": true,
			"verboseSuffix": "test-suffix"
		}
	}
}`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Level = LevelError
				cfg.Format = FormatText
				cfg.Output = OutputFile
				cfg.File.Path = "my-service.log"
				cfg.File.Rotation.MaxSize = 100 * 1024 * 1024
				cfg.File.Rotation.MaxBackups = 42
				cfg.File.Rotation.Compress = true
				cfg.AddCaller = true
				cfg.Error.NoVerbose = true
				cfg.Error.VerboseSuffix = "test-suffix"
				return cfg
			},
		},
		{
			name:        "default json config with masking rules",
			cfgDataType: config.DataTypeJSON,
			cfgData: `
{
	"log": {
		"masking": {
			"enabled": true,
			"rules": [
				{
					"field": "api_key",
					"formats": ["http_header", "json", "urlencoded"],
					"masks": [
						{
							"regexp": "<api_key>.+?</api_key>",
							"mask": "<api_key>***</api_key>"
						}
					]
				}
			]
		}
	}
}`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Masking.Enabled = true
				cfg.Masking.Rules = []MaskingRuleConfig{
					{
						Field:   "api_key",
						Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader, FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
						Masks: []MaskConfig{
							{
								RegExp: "<api_key>.+?</api_key>",
								Mask:   "<api_key>***</api_key>",
							},
						},
					},
				}
				return cfg
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load config using config.Loader.
			appCfg := AppConfig{Log: NewDefaultConfig()}
			expectedAppCfg := AppConfig{Log: tt.expectedCfg()}
			cfgLoader := config.NewLoader(config.NewViperAdapter())
			err := cfgLoader.LoadFromReader(bytes.NewBuffer([]byte(tt.cfgData)), tt.cfgDataType, appCfg.Log)
			require.NoError(t, err)
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using viper unmarshal.
			appCfg = AppConfig{Log: NewDefaultConfig()}
			expectedAppCfg = AppConfig{Log: tt.expectedCfg()}
			vpr := viper.New()
			vpr.SetConfigType(string(tt.cfgDataType))
			require.NoError(t, vpr.ReadConfig(bytes.NewBuffer([]byte(tt.cfgData))))
			require.NoError(t, vpr.Unmarshal(&appCfg, func(c *mapstructure.DecoderConfig) {
				c.DecodeHook = mapstructure.TextUnmarshallerHookFunc()
			}))
			require.Equal(t, expectedAppCfg, appCfg)

			// Load config using yaml/json unmarshal.
			appCfg = AppConfig{Log: NewDefaultConfig()}
			expectedAppCfg = AppConfig{Log: tt.expectedCfg()}
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

func TestConfigWithKeyPrefix(t *testing.T) {
	t.Run("custom key prefix", func(t *testing.T) {
		cfgData := `
customLog:
  level: debug
  format: text
`
		expectedCfg := NewDefaultConfig(WithKeyPrefix("customLog"))
		expectedCfg.Level = LevelDebug
		expectedCfg.Format = FormatText

		cfg := NewConfig(WithKeyPrefix("customLog"))
		err := config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
		require.NoError(t, err)
		require.Equal(t, expectedCfg, cfg)
	})

	t.Run("default key prefix, empty struct initialization", func(t *testing.T) {
		cfgData := `
log:
  level: debug
  format: text
`
		cfg := &Config{}
		err := config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
		require.NoError(t, err)
		require.Equal(t, LevelDebug, cfg.Level)
		require.Equal(t, FormatText, cfg.Format)
	})
}

func TestConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name           string
		yamlData       string
		expectedErrMsg string
	}{
		{
			name: "error, unknown log level",
			yamlData: `
log:
  level: invalid-level
`,
			expectedErrMsg: `log.level: unknown value "invalid-level", should be one of [error warn info debug]`,
		},
		{
			name: "error, unknown log format",
			yamlData: `
log:
  format: invalid-format
`,
			expectedErrMsg: `log.format: unknown value "invalid-format", should be one of [json text]`,
		},
		{
			name: "error, unknown log output",
			yamlData: `
log:
  output: invalid-output
`,
			expectedErrMsg: `log.output: unknown value "invalid-output", should be one of [stdout stderr file]`,
		},
		{
			name: "error, file output without path",
			yamlData: `
log:
  output: file
`,
			expectedErrMsg: `log.file.path: cannot be empty when "file" output is used`,
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

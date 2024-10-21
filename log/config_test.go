/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/config"
)

func TestConfig(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		cfgData := bytes.NewBuffer(nil)
		cfg := Config{}
		err := config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.NoError(t, err)
		require.Equal(t, LevelInfo, cfg.Level)
		require.Equal(t, FormatJSON, cfg.Format)
		require.Equal(t, OutputStdout, cfg.Output)
		require.Equal(t, "", cfg.File.Path)
		require.Equal(t, DefaultFileRotationMaxSizeBytes, int(cfg.File.Rotation.MaxSize))
		require.Equal(t, DefaultFileRotationMaxBackups, cfg.File.Rotation.MaxBackups)
		require.False(t, cfg.File.Rotation.Compress)
		require.False(t, cfg.ErrorNoVerbose)
		require.Equal(t, "_verbose", cfg.ErrorVerboseSuffix)
	})

	t.Run("read values", func(t *testing.T) {
		cfgData := bytes.NewBuffer([]byte(`
log:
  level: warn
  format: text
  output: file
  file:
    path: my-service.log
    rotation:
      compress: true
      maxsize: 100M
      maxbackups: 42
  addcaller: true
  error:
    noverbose: true
    verbosesuffix: test-suffix
`))
		cfg := Config{}
		err := config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.NoError(t, err)
		require.Equal(t, LevelWarn, cfg.Level)
		require.Equal(t, FormatText, cfg.Format)
		require.Equal(t, OutputFile, cfg.Output)
		require.Equal(t, "my-service.log", cfg.File.Path)
		require.True(t, cfg.ErrorNoVerbose)
		require.Equal(t, "test-suffix", cfg.ErrorVerboseSuffix)
		require.Equal(t, 100*1024*1024, int(cfg.File.Rotation.MaxSize))
		require.Equal(t, 42, cfg.File.Rotation.MaxBackups)
		require.True(t, cfg.File.Rotation.Compress)
		require.True(t, cfg.AddCaller)
		require.False(t, cfg.Masking.Enabled)
		require.True(t, cfg.Masking.UseDefaultRules)
		require.Nil(t, cfg.Masking.Rules)
	})

	t.Run("errors", func(t *testing.T) {
		var cfgData *bytes.Buffer
		var cfg Config
		var err error

		cfgData = bytes.NewBuffer([]byte(`
log:
  level: invalid-level
`))
		cfg = Config{}
		err = config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.EqualError(t, err, `log.level: unknown value "invalid-level", should be one of [error warn info debug]`)

		cfgData = bytes.NewBuffer([]byte(`
log:
  format: invalid-format
`))
		cfg = Config{}
		err = config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.EqualError(t, err, `log.format: unknown value "invalid-format", should be one of [json text]`)

		cfgData = bytes.NewBuffer([]byte(`
log:
  output: invalid-output
`))
		cfg = Config{}
		err = config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.EqualError(t, err, `log.output: unknown value "invalid-output", should be one of [stdout stderr file]`)

		cfgData = bytes.NewBuffer([]byte(`
log:
  output: file
`))
		cfg = Config{}
		err = config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.EqualError(t, err, `log.file.path: cannot be empty when "file" output is used`)
	})
}

func TestMaskingConfig(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cfg     string
		masking MaskingConfig
	}{
		{
			name: "enable default",
			cfg: `
log:
  masking:
    enabled: true`,
			masking: MaskingConfig{Enabled: true, UseDefaultRules: true, Rules: nil},
		},
		{
			name: "default with custom rules",
			cfg: `
log:
  masking:
    enabled: true
    useDefaultRules: true
    rules:
      - field: "api_key"
        formats: ["http_header", "json", "urlencoded"]`,
			masking: MaskingConfig{
				Enabled: true, UseDefaultRules: true, Rules: []MaskingRuleConfig{
					{
						Field:   "api_key",
						Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader, FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
					},
				},
			},
		},
		{
			name: "ultimate",
			cfg: `
log:
  masking:
    enabled: true
    useDefaultRules: false
    rules:
      - field: "api_key"
        formats: ["http_header", "json", "urlencoded"]
        masks:
          - regexp: "<api_key>.+?</api_key>"
            mask: "<api_key>***</api_key>"`,
			masking: MaskingConfig{
				Enabled: true, UseDefaultRules: false, Rules: []MaskingRuleConfig{
					{
						Field:   "api_key",
						Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader, FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
						Masks:   []MaskConfig{{RegExp: "<api_key>.+?</api_key>", Mask: "<api_key>***</api_key>"}},
					},
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{}
			err := config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer([]byte(tc.cfg)), config.DataTypeYAML, &cfg)
			require.NoError(t, err)
			require.Equal(t, tc.masking, cfg.Masking)
		})
	}
}

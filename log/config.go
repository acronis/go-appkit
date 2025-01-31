/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"fmt"
	"strings"

	"code.cloudfoundry.org/bytefmt"

	"github.com/acronis/go-appkit/config"
)

const cfgDefaultKeyPrefix = "log"

const (
	cfgKeyLevel                        = "level"
	cfgKeyFormat                       = "format"
	cfgKeyOutput                       = "output"
	cfgKeyNoColor                      = "nocolor"
	cfgKeyFilePath                     = "file.path"
	cfgKeyFileRotationCompress         = "file.rotation.compress"
	cfgKeyFileRotationMaxSize          = "file.rotation.maxSize"
	cfgKeyFileRotationMaxBackups       = "file.rotation.maxBackups"
	cfgKeyFileRotationMaxAgeDays       = "file.rotation.maxAgeDays"
	cfgKeyFileRotationLocalTimeInNames = "file.rotation.localTimeInNames"
	cfgKeyAddCaller                    = "addCaller"
	cfgKeyErrorNoVerbose               = "error.noVerbose"
	cfgKeyErrorVerboseSuffix           = "error.verboseSuffix"
	cfgKeyMaskingEnabled               = "masking.enabled"
	cfgKeyMaskingUseDefaultRules       = "masking.useDefaultRules"
	cfgKeyMaskingRules                 = "masking.rules"
)

// Default and restriction values.
const (
	DefaultFileRotationMaxSizeBytes = 1024 * 1024 * 250
	MinFileRotationMaxSizeBytes     = 1024 * 1024

	DefaultFileRotationMaxBackups = 10
	MinFileRotationMaxBackups     = 1

	defaultErrorVerboseSuffix = "_verbose"
)

// Config represents a set of configuration parameters for logging.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	Level   Level            `mapstructure:"level" yaml:"level" json:"level"`
	Format  Format           `mapstructure:"format" yaml:"format" json:"format"`
	Output  Output           `mapstructure:"output" yaml:"output" json:"output"`
	NoColor bool             `mapstructure:"nocolor" yaml:"nocolor" json:"nocolor"`
	File    FileOutputConfig `mapstructure:"file" yaml:"file" json:"file"`

	Error ErrorConfig `mapstructure:"error" yaml:"error" json:"error"`

	// AddCaller determines whether the caller (in package/file:line format) will be added to each logged message.
	//
	// Example of log with caller:
	// 	{"level":"info","time":"...","msg":"starting application HTTP server...","caller":"httpserver/http_server.go:98","address":":8888"}
	AddCaller bool `mapstructure:"addCaller" yaml:"addCaller" json:"addCaller"`

	Masking MaskingConfig `mapstructure:"masking" yaml:"masking" json:"masking"`

	keyPrefix string
}

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

// ConfigOption is a type for functional options for the Config.
type ConfigOption func(*configOptions)

type configOptions struct {
	keyPrefix string
}

// WithKeyPrefix returns a ConfigOption that sets a key prefix for parsing configuration parameters.
// This prefix will be used by config.Loader.
func WithKeyPrefix(keyPrefix string) ConfigOption {
	return func(o *configOptions) {
		o.keyPrefix = keyPrefix
	}
}

// NewConfig creates a new instance of the Config.
func NewConfig(options ...ConfigOption) *Config {
	var opts = configOptions{keyPrefix: cfgDefaultKeyPrefix} // cfgDefaultKeyPrefix is used here for backward compatibility
	for _, opt := range options {
		opt(&opts)
	}
	return &Config{keyPrefix: opts.keyPrefix}
}

// NewConfigWithKeyPrefix creates a new instance of the Config with a key prefix.
// This prefix will be used by config.Loader.
// Deprecated: use NewConfig with WithKeyPrefix instead.
func NewConfigWithKeyPrefix(keyPrefix string) *Config {
	if keyPrefix != "" {
		keyPrefix += "."
	}
	keyPrefix += cfgDefaultKeyPrefix // cfgDefaultKeyPrefix is added here for backward compatibility
	return &Config{keyPrefix: keyPrefix}
}

// NewDefaultConfig creates a new instance of the Config with default values.
func NewDefaultConfig(options ...ConfigOption) *Config {
	opts := configOptions{keyPrefix: cfgDefaultKeyPrefix}
	for _, opt := range options {
		opt(&opts)
	}
	return &Config{
		keyPrefix: opts.keyPrefix,
		Level:     LevelInfo,
		Format:    FormatJSON,
		Output:    OutputStdout,
		File: FileOutputConfig{
			Rotation: FileRotationConfig{
				MaxSize:    DefaultFileRotationMaxSizeBytes,
				MaxBackups: DefaultFileRotationMaxBackups,
			},
		},
		Error: ErrorConfig{
			VerboseSuffix: defaultErrorVerboseSuffix,
		},
		Masking: MaskingConfig{
			UseDefaultRules: true,
		},
	}
}

// SetProviderDefaults sets default configuration values for logger in config.DataProvider.
// Implements config.Config interface.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyLevel, string(LevelInfo))
	dp.SetDefault(cfgKeyFormat, string(FormatJSON))
	dp.SetDefault(cfgKeyOutput, string(OutputStdout))
	dp.SetDefault(cfgKeyErrorVerboseSuffix, defaultErrorVerboseSuffix)
	dp.SetDefault(cfgKeyFileRotationMaxSize, bytefmt.ByteSize(DefaultFileRotationMaxSizeBytes))
	dp.SetDefault(cfgKeyFileRotationMaxBackups, DefaultFileRotationMaxBackups)
	dp.SetDefault(cfgKeyMaskingUseDefaultRules, true)
}

// Level defines possible values for log levels.
type Level string

// Logging levels.
const (
	LevelError Level = "error"
	LevelWarn  Level = "warn"
	LevelInfo  Level = "info"
	LevelDebug Level = "debug"
)

// Format defines possible values for log formats.
type Format string

// Logging formats.
const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

// Output defines possible values for log outputs.
type Output string

// Logging outputs.
const (
	OutputStdout Output = "stdout"
	OutputStderr Output = "stderr"
	OutputFile   Output = "file"
)

// FieldMaskFormat defines possible values for field mask formats.
type FieldMaskFormat string

// Field mask formats.
const (
	FieldMaskFormatHTTPHeader FieldMaskFormat = "http_header"
	FieldMaskFormatJSON       FieldMaskFormat = "json"
	FieldMaskFormatURLEncoded FieldMaskFormat = "urlencoded"
)

// FileOutputConfig is a configuration for file log output.
type FileOutputConfig struct {
	Path     string             `mapstructure:"path" yaml:"path" json:"path"`
	Rotation FileRotationConfig `mapstructure:"rotation" yaml:"rotation" json:"rotation"`
}

// FileRotationConfig is a configuration for file log rotation.
type FileRotationConfig struct {
	Compress         bool              `mapstructure:"compress" yaml:"compress" json:"compress"`
	MaxSize          config.BytesCount `mapstructure:"maxSize" yaml:"maxSize" json:"maxSize"`
	MaxBackups       int               `mapstructure:"maxBackups" yaml:"maxBackups" json:"maxBackups"`
	MaxAgeDays       int               `mapstructure:"maxAgeDays" yaml:"maxAgeDays" json:"maxAgeDays"`
	LocalTimeInNames bool              `mapstructure:"localTimeInNames" yaml:"localTimeInNames" json:"localTimeInNames"`
}

type ErrorConfig struct {
	// NoVerbose determines whether the verbose error message will be added to each logged error message.
	// If true, or if the verbose error message is equal to the plain error message (err.Error()),
	// no verbose error message will be added.
	// Otherwise, if the logged error implements the fmt.Formatter interface,
	// the verbose error message will be added as a separate field with the key "error" + VerboseSuffix.
	NoVerbose     bool   `mapstructure:"noVerbose" yaml:"noVerbose" json:"noVerbose"`
	VerboseSuffix string `mapstructure:"verboseSuffix" yaml:"verboseSuffix" json:"verboseSuffix"`
}

// MaskingConfig is a configuration for log field masking.
type MaskingConfig struct {
	Enabled         bool                `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	UseDefaultRules bool                `mapstructure:"useDefaultRules" yaml:"useDefaultRules" json:"useDefaultRules"`
	Rules           []MaskingRuleConfig `mapstructure:"rules" yaml:"rules" json:"rules"`
}

// MaskingRuleConfig is a configuration for a single masking rule.
type MaskingRuleConfig struct {
	Field   string            `mapstructure:"field" yaml:"field" json:"field"`
	Formats []FieldMaskFormat `mapstructure:"formats" yaml:"formats" json:"formats"`
	Masks   []MaskConfig      `mapstructure:"masks" yaml:"masks" json:"masks"`
}

// MaskConfig is a configuration for a single mask.
type MaskConfig struct {
	RegExp string `mapstructure:"regexp" yaml:"regexp" json:"regexp"`
	Mask   string `mapstructure:"mask" yaml:"mask" json:"mask"`
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
// Implements config.KeyPrefixProvider interface.
func (c *Config) KeyPrefix() string {
	if c.keyPrefix == "" {
		return cfgDefaultKeyPrefix
	}
	return c.keyPrefix
}

var (
	availableLevels  = []string{string(LevelError), string(LevelWarn), string(LevelInfo), string(LevelDebug)}
	availableFormats = []string{string(FormatJSON), string(FormatText)}
	availableOutputs = []string{string(OutputStdout), string(OutputStderr), string(OutputFile)}
)

// Set sets logger configuration values from config.DataProvider.
// Implements config.Config interface.
func (c *Config) Set(dp config.DataProvider) error {
	var err error

	var levelStr string
	if levelStr, err = dp.GetStringFromSet(cfgKeyLevel, availableLevels, true); err != nil {
		return err
	}
	c.Level = Level(strings.ToLower(levelStr))

	var formatStr string
	if formatStr, err = dp.GetStringFromSet(cfgKeyFormat, availableFormats, true); err != nil {
		return err
	}
	c.Format = Format(strings.ToLower(formatStr))

	var outputStr string
	if outputStr, err = dp.GetStringFromSet(cfgKeyOutput, availableOutputs, true); err != nil {
		return err
	}
	c.Output = Output(strings.ToLower(outputStr))

	if outputCfgErr := c.setFileOutputConfig(dp); outputCfgErr != nil {
		return outputCfgErr
	}

	if c.AddCaller, err = dp.GetBool(cfgKeyAddCaller); err != nil {
		return err
	}

	if c.NoColor, err = dp.GetBool(cfgKeyNoColor); err != nil {
		return err
	}

	if c.Error.NoVerbose, err = dp.GetBool(cfgKeyErrorNoVerbose); err != nil {
		return err
	}
	if c.Error.VerboseSuffix, err = dp.GetString(cfgKeyErrorVerboseSuffix); err != nil {
		return err
	}

	if err := c.setMaskingConfig(dp); err != nil {
		return err
	}
	return nil
}

func (c *Config) setFileOutputConfig(dp config.DataProvider) error {
	var err error

	if c.File.Path, err = dp.GetString(cfgKeyFilePath); err != nil {
		return err
	}
	if c.File.Path == "" && c.Output == OutputFile {
		return dp.WrapKeyErr(
			cfgKeyFilePath, fmt.Errorf("cannot be empty when %q output is used", OutputFile))
	}

	if c.File.Rotation.Compress, err = dp.GetBool(cfgKeyFileRotationCompress); err != nil {
		return err
	}

	if c.File.Rotation.MaxSize, err = dp.GetBytesCount(cfgKeyFileRotationMaxSize); err != nil {
		return err
	}
	if c.File.Rotation.MaxSize < MinFileRotationMaxSizeBytes {
		return dp.WrapKeyErr(cfgKeyFileRotationMaxSize,
			fmt.Errorf("should be >= %s", bytefmt.ByteSize(MinFileRotationMaxSizeBytes)))
	}

	if c.File.Rotation.MaxBackups, err = dp.GetInt(cfgKeyFileRotationMaxBackups); err != nil {
		return err
	}
	if c.File.Rotation.MaxBackups < MinFileRotationMaxBackups {
		return dp.WrapKeyErr(
			cfgKeyFileRotationMaxBackups, fmt.Errorf("should be >= %d", MinFileRotationMaxBackups))
	}

	if c.File.Rotation.MaxAgeDays, err = dp.GetInt(cfgKeyFileRotationMaxAgeDays); err != nil {
		return err
	}
	if c.File.Rotation.MaxAgeDays < 0 {
		return dp.WrapKeyErr(cfgKeyFileRotationMaxAgeDays, fmt.Errorf("should be >= 0"))
	}

	if c.File.Rotation.LocalTimeInNames, err = dp.GetBool(cfgKeyFileRotationLocalTimeInNames); err != nil {
		return err
	}

	return nil
}

func (c *Config) setMaskingConfig(dp config.DataProvider) (err error) {
	if c.Masking.Enabled, err = dp.GetBool(cfgKeyMaskingEnabled); err != nil {
		return err
	}
	if c.Masking.UseDefaultRules, err = dp.GetBool(cfgKeyMaskingUseDefaultRules); err != nil {
		return err
	}
	if err := dp.UnmarshalKey(cfgKeyMaskingRules, &c.Masking.Rules); err != nil {
		return err
	}
	return nil
}

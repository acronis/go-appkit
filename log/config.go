/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"fmt"

	"code.cloudfoundry.org/bytefmt"

	"git.acronis.com/abc/go-libs/v2/config"
)

const (
	cfgKeyLevel                        = "log.level"
	cfgKeyFormat                       = "log.format"
	cfgKeyOutput                       = "log.output"
	cfgKeyNoColor                      = "log.nocolor"
	cfgKeyFilePath                     = "log.file.path"
	cfgKeyFileRotationCompress         = "log.file.rotation.compress"
	cfgKeyFileRotationMaxSize          = "log.file.rotation.maxSize"
	cfgKeyFileRotationMaxBackups       = "log.file.rotation.maxBackups"
	cfgKeyFileRotationMaxAgeDays       = "log.file.rotation.maxAgeDays"
	cfgKeyFileRotationLocalTimeInNames = "log.file.rotation.localTimeInNames"
	cfgKeyAddCaller                    = "log.addCaller"
	cfgKeyErrorNoVerbose               = "log.error.noVerbose"
	cfgKeyErrorVerboseSuffix           = "log.error.verboseSuffix"
)

// Default and restriction values.
const (
	DefaultFileRotationMaxSizeBytes = 1024 * 1024 * 250
	MinFileRotationMaxSizeBytes     = 1024 * 1024

	DefaultFileRotationMaxBackups = 10
	MinFileRotationMaxBackups     = 1
)

// Config represents a set of configuration parameters for logging.
type Config struct {
	Level   Level
	Format  Format
	Output  Output
	NoColor bool
	File    FileOutputConfig

	// ErrorNoVerbose determines whether the verbose error message will be added to each logged error message.
	// If true, or if the verbose error message is equal to the plain error message (err.Error()),
	// no verbose error message will be added.
	// Otherwise, if the logged error implements the fmt.Formatter interface,
	// the verbose error message will be added as a separate field with the key "error" + ErrorVerboseSuffix.
	ErrorNoVerbose     bool
	ErrorVerboseSuffix string

	// AddCaller determines whether the caller (in package/file:line format) will be added to each logged message.
	//
	// Example of log with caller:
	// 	{"level":"info","time":"...","msg":"starting application HTTP server...","caller":"httpserver/http_server.go:98","address":":8888"}
	AddCaller bool

	keyPrefix string
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

// FileOutputConfig is a configuration for file log output.
type FileOutputConfig struct {
	Path     string
	Rotation FileRotationConfig
}

// FileRotationConfig is a configuration for file log rotation.
type FileRotationConfig struct {
	Compress         bool
	MaxSize          uint64
	MaxBackups       int
	MaxAgeDays       int
	LocalTimeInNames bool
}

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

// NewConfig creates a new instance of the Config.
func NewConfig() *Config {
	return NewConfigWithKeyPrefix("")
}

// NewConfigWithKeyPrefix creates a new instance of the Config.
// Allows to specify key prefix which will be used for parsing configuration parameters.
func NewConfigWithKeyPrefix(keyPrefix string) *Config {
	return &Config{keyPrefix: keyPrefix}
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
func (c *Config) KeyPrefix() string {
	return c.keyPrefix
}

// SetProviderDefaults sets default configuration values for logger in config.DataProvider.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyLevel, string(LevelInfo))
	dp.SetDefault(cfgKeyFormat, string(FormatJSON))
	dp.SetDefault(cfgKeyOutput, string(OutputStdout))
	dp.SetDefault(cfgKeyNoColor, false)
	dp.SetDefault(cfgKeyFileRotationCompress, false)
	dp.SetDefault(cfgKeyFileRotationMaxSize, bytefmt.ByteSize(DefaultFileRotationMaxSizeBytes))
	dp.SetDefault(cfgKeyFileRotationMaxBackups, DefaultFileRotationMaxBackups)
	dp.SetDefault(cfgKeyErrorNoVerbose, false)
	dp.SetDefault(cfgKeyErrorVerboseSuffix, "_verbose")
}

var (
	availableLevels  = []string{string(LevelError), string(LevelWarn), string(LevelInfo), string(LevelDebug)}
	availableFormats = []string{string(FormatJSON), string(FormatText)}
	availableOutputs = []string{string(OutputStdout), string(OutputStderr), string(OutputFile)}
)

// Set sets logger configuration values from config.DataProvider.
func (c *Config) Set(dp config.DataProvider) error {
	var err error

	var levelStr string
	if levelStr, err = dp.GetStringFromSet(cfgKeyLevel, availableLevels, true); err != nil {
		return err
	}
	c.Level = Level(levelStr)

	var formatStr string
	if formatStr, err = dp.GetStringFromSet(cfgKeyFormat, availableFormats, false); err != nil {
		return err
	}
	c.Format = Format(formatStr)

	var outputStr string
	if outputStr, err = dp.GetStringFromSet(cfgKeyOutput, availableOutputs, false); err != nil {
		return err
	}
	c.Output = Output(outputStr)

	if outputCfgErr := c.setFileOutputConfig(dp); outputCfgErr != nil {
		return outputCfgErr
	}

	if c.AddCaller, err = dp.GetBool(cfgKeyAddCaller); err != nil {
		return err
	}

	if c.NoColor, err = dp.GetBool(cfgKeyNoColor); err != nil {
		return err
	}

	if c.ErrorNoVerbose, err = dp.GetBool(cfgKeyErrorNoVerbose); err != nil {
		return err
	}
	if c.ErrorVerboseSuffix, err = dp.GetString(cfgKeyErrorVerboseSuffix); err != nil {
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

	if c.File.Rotation.MaxSize, err = dp.GetSizeInBytes(cfgKeyFileRotationMaxSize); err != nil {
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

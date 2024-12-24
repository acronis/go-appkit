/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"errors"
	"github.com/acronis/go-appkit/config"
	"time"
)

const (
	DefaultClientWaitTimeout = 10 * time.Second

	// configuration properties
	cfgKeyRetriesEnabled             = "retries.enabled"
	cfgKeyRetriesMax                 = "retries.maxAttempts"
	cfgKeyRateLimitsEnabled          = "rateLimits.enabled"
	cfgKeyRateLimitsLimit            = "rateLimits.limit"
	cfgKeyRateLimitsBurst            = "rateLimits.burst"
	cfgKeyRateLimitsWaitTimeout      = "rateLimits.waitTimeout"
	cfgKeyLoggerEnabled              = "logger.enabled"
	cfgKeyLoggerMode                 = "logger.mode"
	cfgKeyLoggerSlowRequestThreshold = "logger.slowRequestThreshold"
	cfgKeyMetricsEnabled             = "metrics.enabled"
	cfgKeyTimeout                    = "timeout"
)

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

// RateLimitConfig represents configuration options for HTTP client rate limits.
type RateLimitConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	Limit       int           `mapstructure:"limit"`
	Burst       int           `mapstructure:"burst"`
	WaitTimeout time.Duration `mapstructure:"waitTimeout"`
}

// Set is part of config interface implementation.
func (c *RateLimitConfig) Set(dp config.DataProvider) (err error) {
	enabled, err := dp.GetBool(cfgKeyRateLimitsEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	limit, err := dp.GetInt(cfgKeyRateLimitsLimit)
	if err != nil {
		return err
	}
	if limit <= 0 {
		return errors.New("client rate limit must be positive")
	}
	c.Limit = limit

	burst, err := dp.GetInt(cfgKeyRateLimitsBurst)
	if err != nil {
		return err
	}
	if burst < 0 {
		return errors.New("client burst must be positive")
	}
	c.Burst = burst

	waitTimeout, err := dp.GetDuration(cfgKeyRateLimitsWaitTimeout)
	if err != nil {
		return err
	}
	if waitTimeout < 0 {
		return errors.New("client wait timeout must be positive")
	}
	c.WaitTimeout = waitTimeout

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *RateLimitConfig) SetProviderDefaults(_ config.DataProvider) {}

// TransportOpts returns transport options.
func (c *RateLimitConfig) TransportOpts() RateLimitingRoundTripperOpts {
	return RateLimitingRoundTripperOpts{
		Burst:       c.Burst,
		WaitTimeout: c.WaitTimeout,
	}
}

// RetriesConfig represents configuration options for HTTP client retries policy.
type RetriesConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	MaxAttempts int  `mapstructure:"maxAttempts"`
}

// Set is part of config interface implementation.
func (c *RetriesConfig) Set(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyRetriesEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	maxAttempts, err := dp.GetInt(cfgKeyRetriesMax)
	if err != nil {
		return err
	}
	if maxAttempts < 0 {
		return errors.New("client max retry attempts must be positive")
	}
	c.MaxAttempts = maxAttempts

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *RetriesConfig) SetProviderDefaults(_ config.DataProvider) {}

// TransportOpts returns transport options.
func (c *RetriesConfig) TransportOpts() RetryableRoundTripperOpts {
	return RetryableRoundTripperOpts{MaxRetryAttempts: c.MaxAttempts}
}

// LoggerConfig represents configuration options for HTTP client logs.
type LoggerConfig struct {
	Enabled              bool          `mapstructure:"enabled"`
	SlowRequestThreshold time.Duration `mapstructure:"slowRequestThreshold"`
	Mode                 string        `mapstructure:"mode"`
}

// Set is part of config interface implementation.
func (c *LoggerConfig) Set(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyLoggerEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	slowRequestThreshold, err := dp.GetDuration(cfgKeyLoggerSlowRequestThreshold)
	if err != nil {
		return err
	}
	if slowRequestThreshold < 0 {
		return errors.New("client logger slow request threshold can not be negative")
	}
	c.SlowRequestThreshold = slowRequestThreshold

	mode, err := dp.GetString(cfgKeyLoggerMode)
	if err != nil {
		return err
	}
	if !LoggerMode(mode).IsValid() {
		return errors.New("client logger invalid mode, choose one of: [none, all, failed]")
	}
	c.Mode = mode

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *LoggerConfig) SetProviderDefaults(_ config.DataProvider) {}

// TransportOpts returns transport options.
func (c *LoggerConfig) TransportOpts() LoggingRoundTripperOpts {
	return LoggingRoundTripperOpts{
		Mode:                 c.Mode,
		SlowRequestThreshold: c.SlowRequestThreshold,
	}
}

// MetricsConfig represents configuration options for HTTP client logs.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// Set is part of config interface implementation.
func (c *MetricsConfig) Set(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyMetricsEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *MetricsConfig) SetProviderDefaults(_ config.DataProvider) {}

// Config represents options for HTTP client configuration.
type Config struct {
	Retries    RetriesConfig   `mapstructure:"retries"`
	RateLimits RateLimitConfig `mapstructure:"rateLimits"`
	Logger     LoggerConfig    `mapstructure:"logger"`
	Metrics    MetricsConfig   `mapstructure:"metrics"`
	Timeout    time.Duration   `mapstructure:"timeout"`

	keyPrefix string
}

// NewConfig creates a new instance of the Config.
func NewConfig() *Config {
	return NewConfigWithKeyPrefix("")
}

// NewConfigWithKeyPrefix creates a new instance of the Config.
// Allows specifying key prefix which will be used for parsing configuration parameters.
func NewConfigWithKeyPrefix(keyPrefix string) *Config {
	return &Config{keyPrefix: keyPrefix}
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
func (c *Config) KeyPrefix() string {
	return c.keyPrefix
}

// Set is part of config interface implementation.
func (c *Config) Set(dp config.DataProvider) error {
	timeout, err := dp.GetDuration(cfgKeyTimeout)
	if err != nil {
		return err
	}
	c.Timeout = timeout

	err = c.Retries.Set(config.NewKeyPrefixedDataProvider(dp, c.keyPrefix))
	if err != nil {
		return err
	}

	err = c.RateLimits.Set(config.NewKeyPrefixedDataProvider(dp, c.keyPrefix))
	if err != nil {
		return err
	}

	err = c.Logger.Set(config.NewKeyPrefixedDataProvider(dp, c.keyPrefix))
	if err != nil {
		return err
	}

	err = c.Metrics.Set(config.NewKeyPrefixedDataProvider(dp, c.keyPrefix))
	if err != nil {
		return err
	}

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *Config) SetProviderDefaults(_ config.DataProvider) {
	c.Timeout = DefaultClientWaitTimeout
}

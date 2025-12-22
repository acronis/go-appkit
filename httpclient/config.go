/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"errors"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/retry"
)

// RetryPolicy represents a retry policy strategy.
type RetryPolicy string

const (
	DefaultTimeout = time.Minute

	// RetryPolicyExponential is a policy for exponential retries.
	RetryPolicyExponential RetryPolicy = "exponential"

	// RetryPolicyConstant is a policy for constant retries.
	RetryPolicyConstant RetryPolicy = "constant"

	// configuration properties
	cfgKeyRetriesEnabled                          = "retries.enabled"
	cfgKeyRetriesMax                              = "retries.maxAttempts"
	cfgKeyRetriesPolicy                           = "retries.policy"
	cfgKeyRetriesPolicyExponentialInitialInterval = "retries.exponentialBackoff.initialInterval"
	cfgKeyRetriesPolicyExponentialMultiplier      = "retries.exponentialBackoff.multiplier"
	cfgKeyRetriesPolicyConstantInternal           = "retries.constantBackoff.interval"
	cfgKeyRateLimitsEnabled                       = "rateLimits.enabled"
	cfgKeyRateLimitsLimit                         = "rateLimits.limit"
	cfgKeyRateLimitsBurst                         = "rateLimits.burst"
	cfgKeyRateLimitsWaitTimeout                   = "rateLimits.waitTimeout"
	cfgKeyLogEnabled                              = "log.enabled"
	cfgKeyLogMode                                 = "log.mode"
	cfgKeyLogSlowRequestThreshold                 = "log.slowRequestThreshold"
	cfgKeyMetricsEnabled                          = "metrics.enabled"
	cfgKeyTimeout                                 = "timeout"
)

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

// Config represents options for HTTP client configuration.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	// Retries is a configuration for HTTP client retries policy.
	Retries RetriesConfig `mapstructure:"retries" yaml:"retries" json:"retries"`

	// RateLimits is a configuration for HTTP client rate limits.
	RateLimits RateLimitsConfig `mapstructure:"rateLimits" yaml:"rateLimits" json:"rateLimits"`

	// Log is a configuration for HTTP client logs.
	Log LogConfig `mapstructure:"log" yaml:"log" json:"log"`

	// Metrics is a configuration for HTTP client metrics.
	Metrics MetricsConfig `mapstructure:"metrics" yaml:"metrics" json:"metrics"`

	// Timeout is the maximum time to wait for a request to be made.
	Timeout config.TimeDuration `mapstructure:"timeout" yaml:"timeout" json:"timeout"`

	// keyPrefix is a prefix for configuration parameters.
	keyPrefix string
}

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
	var opts configOptions
	for _, opt := range options {
		opt(&opts)
	}
	return &Config{keyPrefix: opts.keyPrefix}
}

// NewConfigWithKeyPrefix creates a new instance of the Config with a key prefix.
// This prefix will be used by config.Loader.
// Deprecated: use NewConfig with WithKeyPrefix instead.
func NewConfigWithKeyPrefix(keyPrefix string) *Config {
	return &Config{keyPrefix: keyPrefix}
}

// NewDefaultConfig creates a new instance of the Config with default values.
func NewDefaultConfig(options ...ConfigOption) *Config {
	var opts configOptions
	for _, opt := range options {
		opt(&opts)
	}
	return &Config{
		keyPrefix: opts.keyPrefix,
		Timeout:   config.TimeDuration(DefaultTimeout),
	}
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
// Implements config.KeyPrefixProvider interface.
func (c *Config) KeyPrefix() string {
	return c.keyPrefix
}

// SetProviderDefaults is part of config interface implementation.
// Implements config.Config interface.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyTimeout, DefaultTimeout)
}

// RateLimitsConfig represents configuration options for HTTP client rate limits.
type RateLimitsConfig struct {
	// Enabled is a flag that enables rate limiting.
	Enabled bool `mapstructure:"enabled" yaml:"enabled" json:"enabled"`

	// Limit is the maximum number of requests that can be made.
	Limit int `mapstructure:"limit" yaml:"limit" json:"limit"`

	// Burst allow temporary spikes in request rate.
	Burst int `mapstructure:"burst" yaml:"burst" json:"burst"`

	// WaitTimeout is the maximum time to wait for a request to be made.
	WaitTimeout config.TimeDuration `mapstructure:"waitTimeout" yaml:"waitTimeout" json:"waitTimeout"`
}

// TransportOpts returns transport options.
func (c *RateLimitsConfig) TransportOpts() RateLimitingRoundTripperOpts {
	return RateLimitingRoundTripperOpts{
		Burst:       c.Burst,
		WaitTimeout: time.Duration(c.WaitTimeout),
	}
}

// ExponentialBackoffConfig represents configuration options for exponential backoff.
type ExponentialBackoffConfig struct {
	// InitialInterval is the initial interval for exponential backoff.
	InitialInterval config.TimeDuration `mapstructure:"initialInterval" yaml:"initialInterval" json:"initialInterval"`

	// Multiplier is the multiplier for exponential backoff.
	Multiplier float64 `mapstructure:"multiplier" yaml:"multiplier" json:"multiplier"`
}

// ConstantBackoffConfig represents configuration options for constant backoff.
type ConstantBackoffConfig struct {
	// Interval is the interval for constant backoff.
	Interval config.TimeDuration `mapstructure:"interval" yaml:"interval" json:"interval"`
}

// RetriesConfig represents configuration options for HTTP client retries policy.
type RetriesConfig struct {
	// Enabled is a flag that enables retries.
	Enabled bool `mapstructure:"enabled" yaml:"enabled" json:"enabled"`

	// MaxAttempts is the maximum number of attempts to retry the request.
	MaxAttempts int `mapstructure:"maxAttempts" yaml:"maxAttempts" json:"maxAttempts"`

	// Policy of a retry: [exponential, constant].
	Policy RetryPolicy `mapstructure:"policy" yaml:"policy" json:"policy"`

	// ExponentialBackoff is the configuration for exponential backoff.
	ExponentialBackoff ExponentialBackoffConfig `mapstructure:"exponentialBackoff" yaml:"exponentialBackoff" json:"exponentialBackoff"`

	// ConstantBackoff is the configuration for constant backoff.
	ConstantBackoff ConstantBackoffConfig `mapstructure:"constantBackoff" yaml:"constantBackoff" json:"constantBackoff"`
}

// GetPolicy returns a retry policy based on strategy or nil if none is provided.
func (c *RetriesConfig) GetPolicy() retry.Policy {
	switch c.Policy {
	case RetryPolicyExponential:
		return retry.PolicyFunc(func() backoff.BackOff {
			bf := backoff.NewExponentialBackOff()
			bf.InitialInterval = time.Duration(c.ExponentialBackoff.InitialInterval)
			bf.Multiplier = c.ExponentialBackoff.Multiplier
			bf.Reset()
			return bf
		})
	case RetryPolicyConstant:
		return retry.PolicyFunc(func() backoff.BackOff {
			bf := backoff.NewConstantBackOff(time.Duration(c.ConstantBackoff.Interval))
			bf.Reset()
			return bf
		})
	}

	return nil
}

// TransportOpts returns transport options.
func (c *RetriesConfig) TransportOpts() RetryableRoundTripperOpts {
	return RetryableRoundTripperOpts{MaxRetryAttempts: c.MaxAttempts}
}

// LogConfig represents configuration options for HTTP client logs.
type LogConfig struct {
	// Enabled is a flag that enables logging.
	Enabled bool `mapstructure:"enabled" yaml:"enabled" json:"enabled"`

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold config.TimeDuration `mapstructure:"slowRequestThreshold" yaml:"slowRequestThreshold" json:"slowRequestThreshold"`

	// Mode of logging: [all, failed]. 'all' by default.
	Mode LoggingMode `mapstructure:"mode" yaml:"mode" json:"mode"`
}

// TransportOpts returns transport options.
func (c *LogConfig) TransportOpts() LoggingRoundTripperOpts {
	return LoggingRoundTripperOpts{
		Mode:                 c.Mode,
		SlowRequestThreshold: time.Duration(c.SlowRequestThreshold),
	}
}

// MetricsConfig represents configuration options for HTTP client logs.
type MetricsConfig struct {
	// Enabled is a flag that enables metrics.
	Enabled bool `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
}

// Set sets http client configuration based on the passed config.DataProvider.
// Implements config.Config interface.
func (c *Config) Set(dp config.DataProvider) error {
	timeout, err := dp.GetDuration(cfgKeyTimeout)
	if err != nil {
		return err
	}
	c.Timeout = config.TimeDuration(timeout)

	if err := c.setRetries(dp); err != nil {
		return err
	}
	if err := c.setRateLimits(dp); err != nil {
		return err
	}
	if err := c.setLog(dp); err != nil {
		return err
	}
	if err := c.setMetrics(dp); err != nil {
		return err
	}
	return nil
}

func (c *Config) setRetries(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyRetriesEnabled)
	if err != nil {
		return err
	}
	c.Retries.Enabled = enabled

	if !c.Retries.Enabled {
		return nil
	}

	maxAttempts, err := dp.GetInt(cfgKeyRetriesMax)
	if err != nil {
		return err
	}
	if maxAttempts < 0 {
		return dp.WrapKeyErr(cfgKeyRetriesMax, errors.New("must be positive"))
	}
	c.Retries.MaxAttempts = maxAttempts

	return c.setRetriesPolicy(dp)
}

func (c *Config) setRetriesPolicy(dp config.DataProvider) error {
	policy, err := dp.GetString(cfgKeyRetriesPolicy)
	if err != nil {
		return err
	}
	c.Retries.Policy = RetryPolicy(policy)

	if c.Retries.Policy != "" && c.Retries.Policy != RetryPolicyExponential && c.Retries.Policy != RetryPolicyConstant {
		return dp.WrapKeyErr(cfgKeyRetriesPolicy, errors.New("must be one of: [exponential, constant]"))
	}

	if c.Retries.Policy == RetryPolicyExponential {
		var interval time.Duration
		interval, err = dp.GetDuration(cfgKeyRetriesPolicyExponentialInitialInterval)
		if err != nil {
			return err
		}
		if interval < 0 {
			return dp.WrapKeyErr(cfgKeyRetriesPolicyExponentialInitialInterval, errors.New("must be positive"))
		}

		var multiplier float64
		multiplier, err = dp.GetFloat64(cfgKeyRetriesPolicyExponentialMultiplier)
		if err != nil {
			return err
		}
		if multiplier <= 1 {
			return dp.WrapKeyErr(cfgKeyRetriesPolicyExponentialMultiplier, errors.New("must be greater than 1"))
		}

		c.Retries.ExponentialBackoff = ExponentialBackoffConfig{
			InitialInterval: config.TimeDuration(interval),
			Multiplier:      multiplier,
		}

		return nil
	}

	if c.Retries.Policy == RetryPolicyConstant {
		var interval time.Duration
		interval, err = dp.GetDuration(cfgKeyRetriesPolicyConstantInternal)
		if err != nil {
			return err
		}
		if interval < 0 {
			return dp.WrapKeyErr(cfgKeyRetriesPolicyConstantInternal, errors.New("must be positive"))
		}
		c.Retries.ConstantBackoff = ConstantBackoffConfig{Interval: config.TimeDuration(interval)}
	}

	return nil
}

func (c *Config) setRateLimits(dp config.DataProvider) (err error) {
	enabled, err := dp.GetBool(cfgKeyRateLimitsEnabled)
	if err != nil {
		return err
	}
	c.RateLimits.Enabled = enabled

	if !c.RateLimits.Enabled {
		return nil
	}

	limit, err := dp.GetInt(cfgKeyRateLimitsLimit)
	if err != nil {
		return err
	}
	if limit <= 0 {
		return dp.WrapKeyErr(cfgKeyRateLimitsLimit, errors.New("must be positive"))
	}
	c.RateLimits.Limit = limit

	burst, err := dp.GetInt(cfgKeyRateLimitsBurst)
	if err != nil {
		return err
	}
	if burst < 0 {
		return dp.WrapKeyErr(cfgKeyRateLimitsBurst, errors.New("must be positive"))
	}
	c.RateLimits.Burst = burst

	waitTimeout, err := dp.GetDuration(cfgKeyRateLimitsWaitTimeout)
	if err != nil {
		return err
	}
	if waitTimeout < 0 {
		return dp.WrapKeyErr(cfgKeyRateLimitsWaitTimeout, errors.New("must be positive"))
	}
	c.RateLimits.WaitTimeout = config.TimeDuration(waitTimeout)

	return nil
}

func (c *Config) setLog(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyLogEnabled)
	if err != nil {
		return err
	}
	c.Log.Enabled = enabled

	if !c.Log.Enabled {
		return nil
	}

	slowRequestThreshold, err := dp.GetDuration(cfgKeyLogSlowRequestThreshold)
	if err != nil {
		return err
	}
	if slowRequestThreshold < 0 {
		return dp.WrapKeyErr(cfgKeyLogSlowRequestThreshold, errors.New("can not be negative"))
	}
	c.Log.SlowRequestThreshold = config.TimeDuration(slowRequestThreshold)

	mode, err := dp.GetString(cfgKeyLogMode)
	if err != nil {
		return err
	}
	loggingMode := LoggingMode(mode)
	if !loggingMode.IsValid() {
		return dp.WrapKeyErr(cfgKeyLogMode, errors.New("choose one of: [all, failed]"))
	}
	c.Log.Mode = loggingMode

	return nil
}

func (c *Config) setMetrics(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyMetricsEnabled)
	if err != nil {
		return err
	}
	c.Metrics.Enabled = enabled

	return nil
}

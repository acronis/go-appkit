/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"errors"
	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/retry"
	"github.com/cenkalti/backoff/v4"
	"time"
)

const (
	// DefaultClientWaitTimeout is a default timeout for a client to wait for a request.
	DefaultClientWaitTimeout = 10 * time.Second

	// RetryPolicyExponential is a policy for exponential retries.
	RetryPolicyExponential = "exponential"

	// RetryPolicyConstant is a policy for constant retries.
	RetryPolicyConstant = "constant"

	// configuration properties
	cfgKeyRetriesEnabled                          = "retries.enabled"
	cfgKeyRetriesMax                              = "retries.maxAttempts"
	cfgKeyRetriesPolicyStrategy                   = "retries.policy.strategy"
	cfgKeyRetriesPolicyExponentialInitialInterval = "retries.policy.exponentialBackoffInitialInterval"
	cfgKeyRetriesPolicyExponentialMultiplier      = "retries.policy.exponentialBackoffMultiplier"
	cfgKeyRetriesPolicyConstantInternal           = "retries.policy.constantBackoffInterval"
	cfgKeyRateLimitsEnabled                       = "rateLimits.enabled"
	cfgKeyRateLimitsLimit                         = "rateLimits.limit"
	cfgKeyRateLimitsBurst                         = "rateLimits.burst"
	cfgKeyRateLimitsWaitTimeout                   = "rateLimits.waitTimeout"
	cfgKeyLoggerEnabled                           = "logger.enabled"
	cfgKeyLoggerMode                              = "logger.mode"
	cfgKeyLoggerSlowRequestThreshold              = "logger.slowRequestThreshold"
	cfgKeyMetricsEnabled                          = "metrics.enabled"
	cfgKeyTimeout                                 = "timeout"
)

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

// RateLimitConfig represents configuration options for HTTP client rate limits.
type RateLimitConfig struct {
	// Enabled is a flag that enables rate limiting.
	Enabled bool `mapstructure:"enabled"`

	// Limit is the maximum number of requests that can be made.
	Limit int `mapstructure:"limit"`

	// Burst allow temporary spikes in request rate.
	Burst int `mapstructure:"burst"`

	// WaitTimeout is the maximum time to wait for a request to be made.
	WaitTimeout time.Duration `mapstructure:"waitTimeout"`
}

// Set is part of config interface implementation.
func (c *RateLimitConfig) Set(dp config.DataProvider) (err error) {
	enabled, err := dp.GetBool(cfgKeyRateLimitsEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	if !c.Enabled {
		return nil
	}

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

// PolicyConfig represents configuration options for policy retry.
type PolicyConfig struct {
	// Strategy is a strategy for retry policy.
	Strategy string `mapstructure:"strategy"`

	// ExponentialBackoffInitialInterval is the initial interval for exponential backoff.
	ExponentialBackoffInitialInterval time.Duration `mapstructure:"exponentialBackoffInitialInterval"`

	// ExponentialBackoffMultiplier is the multiplier for exponential backoff.
	ExponentialBackoffMultiplier float64 `mapstructure:"exponentialBackoffMultiplier"`

	// ConstantBackoffInterval is the interval for constant backoff.
	ConstantBackoffInterval time.Duration `mapstructure:"constantBackoffInterval"`
}

// Set is part of config interface implementation.
func (c *PolicyConfig) Set(dp config.DataProvider) (err error) {
	strategy, err := dp.GetString(cfgKeyRetriesPolicyStrategy)
	if err != nil {
		return err
	}
	c.Strategy = strategy

	if c.Strategy != "" && c.Strategy != RetryPolicyExponential && c.Strategy != RetryPolicyConstant {
		return errors.New("client retry policy must be one of: [exponential, constant]")
	}

	if c.Strategy == RetryPolicyExponential {
		var interval time.Duration
		interval, err = dp.GetDuration(cfgKeyRetriesPolicyExponentialInitialInterval)
		if err != nil {
			return nil
		}
		if interval < 0 {
			return errors.New("client exponential backoff initial interval must be positive")
		}
		c.ExponentialBackoffInitialInterval = interval

		var multiplier float64
		multiplier, err = dp.GetFloat64(cfgKeyRetriesPolicyExponentialMultiplier)
		if err != nil {
			return err
		}
		if multiplier <= 1 {
			return errors.New("client exponential backoff multiplier must be greater than 1")
		}
		c.ExponentialBackoffMultiplier = multiplier

		return nil
	} else if c.Strategy == RetryPolicyConstant {
		var interval time.Duration
		interval, err = dp.GetDuration(cfgKeyRetriesPolicyConstantInternal)
		if err != nil {
			return err
		}
		if interval < 0 {
			return errors.New("client constant backoff interval must be positive")
		}
		c.ConstantBackoffInterval = interval
	}

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *PolicyConfig) SetProviderDefaults(_ config.DataProvider) {}

// RetriesConfig represents configuration options for HTTP client retries policy.
type RetriesConfig struct {
	// Enabled is a flag that enables retries.
	Enabled bool `mapstructure:"enabled"`

	// MaxAttempts is the maximum number of attempts to retry the request.
	MaxAttempts int `mapstructure:"maxAttempts"`

	// Policy of a retry: [exponential, constant]. default is exponential.
	Policy PolicyConfig `mapstructure:"policy"`
}

// GetPolicy returns a retry policy based on strategy or nil if none is provided.
func (c *RetriesConfig) GetPolicy() retry.Policy {
	if c.Policy.Strategy == RetryPolicyExponential {
		return retry.PolicyFunc(func() backoff.BackOff {
			bf := backoff.NewExponentialBackOff()
			bf.InitialInterval = c.Policy.ExponentialBackoffInitialInterval
			bf.Multiplier = c.Policy.ExponentialBackoffMultiplier
			bf.Reset()
			return bf
		})
	} else if c.Policy.Strategy == RetryPolicyConstant {
		return retry.PolicyFunc(func() backoff.BackOff {
			bf := backoff.NewConstantBackOff(c.Policy.ConstantBackoffInterval)
			bf.Reset()
			return bf
		})
	}

	return nil
}

// Set is part of config interface implementation.
func (c *RetriesConfig) Set(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyRetriesEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	if !c.Enabled {
		return nil
	}

	maxAttempts, err := dp.GetInt(cfgKeyRetriesMax)
	if err != nil {
		return err
	}
	if maxAttempts < 0 {
		return errors.New("client max retry attempts must be positive")
	}
	c.MaxAttempts = maxAttempts

	err = c.Policy.Set(config.NewKeyPrefixedDataProvider(dp, ""))
	if err != nil {
		return err
	}

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
	// Enabled is a flag that enables logging.
	Enabled bool `mapstructure:"enabled"`

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold time.Duration `mapstructure:"slowRequestThreshold"`

	// Mode of logging.
	Mode string `mapstructure:"mode"`
}

// Set is part of config interface implementation.
func (c *LoggerConfig) Set(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyLoggerEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	if !c.Enabled {
		return nil
	}

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
	// Enabled is a flag that enables metrics.
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
	// Retries is a configuration for HTTP client retries policy.
	Retries RetriesConfig `mapstructure:"retries"`

	// RateLimits is a configuration for HTTP client rate limits.
	RateLimits RateLimitConfig `mapstructure:"rateLimits"`

	// Logger is a configuration for HTTP client logs.
	Logger LoggerConfig `mapstructure:"logger"`

	// Metrics is a configuration for HTTP client metrics.
	Metrics MetricsConfig `mapstructure:"metrics"`

	// Timeout is the maximum time to wait for a request to be made.
	Timeout time.Duration `mapstructure:"timeout"`

	// keyPrefix is a prefix for configuration parameters.
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

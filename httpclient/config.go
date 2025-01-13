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
	// DefaultClientWaitTimeout is a default timeout for a client to wait for a request.
	DefaultClientWaitTimeout = 10 * time.Second

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

// RateLimitsConfig represents configuration options for HTTP client rate limits.
type RateLimitsConfig struct {
	// Enabled is a flag that enables rate limiting.
	Enabled bool

	// Limit is the maximum number of requests that can be made.
	Limit int

	// Burst allow temporary spikes in request rate.
	Burst int

	// WaitTimeout is the maximum time to wait for a request to be made.
	WaitTimeout time.Duration
}

// Set is part of config interface implementation.
func (c *RateLimitsConfig) Set(dp config.DataProvider) (err error) {
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
		return dp.WrapKeyErr(cfgKeyRateLimitsLimit, errors.New("must be positive"))
	}
	c.Limit = limit

	burst, err := dp.GetInt(cfgKeyRateLimitsBurst)
	if err != nil {
		return err
	}
	if burst < 0 {
		return dp.WrapKeyErr(cfgKeyRateLimitsBurst, errors.New("must be positive"))
	}
	c.Burst = burst

	waitTimeout, err := dp.GetDuration(cfgKeyRateLimitsWaitTimeout)
	if err != nil {
		return err
	}
	if waitTimeout < 0 {
		return dp.WrapKeyErr(cfgKeyRateLimitsWaitTimeout, errors.New("must be positive"))
	}
	c.WaitTimeout = waitTimeout

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *RateLimitsConfig) SetProviderDefaults(_ config.DataProvider) {}

// TransportOpts returns transport options.
func (c *RateLimitsConfig) TransportOpts() RateLimitingRoundTripperOpts {
	return RateLimitingRoundTripperOpts{
		Burst:       c.Burst,
		WaitTimeout: c.WaitTimeout,
	}
}

// ExponentialBackoffConfig represents configuration options for exponential backoff.
type ExponentialBackoffConfig struct {
	// InitialInterval is the initial interval for exponential backoff.
	InitialInterval time.Duration

	// Multiplier is the multiplier for exponential backoff.
	Multiplier float64
}

// ConstantBackoffConfig represents configuration options for constant backoff.
type ConstantBackoffConfig struct {
	// Interval is the interval for constant backoff.
	Interval time.Duration
}

// RetriesConfig represents configuration options for HTTP client retries policy.
type RetriesConfig struct {
	// Enabled is a flag that enables retries.
	Enabled bool

	// MaxAttempts is the maximum number of attempts to retry the request.
	MaxAttempts int

	// Policy of a retry: [exponential, constant].
	Policy RetryPolicy

	// ExponentialBackoff is the configuration for exponential backoff.
	ExponentialBackoff ExponentialBackoffConfig

	// ConstantBackoff is the configuration for constant backoff.
	ConstantBackoff ConstantBackoffConfig
}

// GetPolicy returns a retry policy based on strategy or nil if none is provided.
func (c *RetriesConfig) GetPolicy() retry.Policy {
	if c.Policy == RetryPolicyExponential {
		return retry.PolicyFunc(func() backoff.BackOff {
			bf := backoff.NewExponentialBackOff()
			bf.InitialInterval = c.ExponentialBackoff.InitialInterval
			bf.Multiplier = c.ExponentialBackoff.Multiplier
			bf.Reset()
			return bf
		})
	} else if c.Policy == RetryPolicyConstant {
		return retry.PolicyFunc(func() backoff.BackOff {
			bf := backoff.NewConstantBackOff(c.ConstantBackoff.Interval)
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
		return dp.WrapKeyErr(cfgKeyRetriesMax, errors.New("must be positive"))
	}
	c.MaxAttempts = maxAttempts

	return c.setPolicy(dp)
}

// SetProviderDefaults is part of config interface implementation.
func (c *RetriesConfig) SetProviderDefaults(_ config.DataProvider) {}

// TransportOpts returns transport options.
func (c *RetriesConfig) TransportOpts() RetryableRoundTripperOpts {
	return RetryableRoundTripperOpts{MaxRetryAttempts: c.MaxAttempts}
}

// setPolicy sets the policy based on the configuration.
func (c *RetriesConfig) setPolicy(dp config.DataProvider) error {
	policy, err := dp.GetString(cfgKeyRetriesPolicy)
	if err != nil {
		return err
	}
	c.Policy = RetryPolicy(policy)

	if c.Policy != "" && c.Policy != RetryPolicyExponential && c.Policy != RetryPolicyConstant {
		return dp.WrapKeyErr(cfgKeyRetriesPolicy, errors.New("must be one of: [exponential, constant]"))
	}

	if c.Policy == RetryPolicyExponential {
		var interval time.Duration
		interval, err = dp.GetDuration(cfgKeyRetriesPolicyExponentialInitialInterval)
		if err != nil {
			return nil
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

		c.ExponentialBackoff = ExponentialBackoffConfig{
			InitialInterval: interval,
			Multiplier:      multiplier,
		}

		return nil
	} else if c.Policy == RetryPolicyConstant {
		var interval time.Duration
		interval, err = dp.GetDuration(cfgKeyRetriesPolicyConstantInternal)
		if err != nil {
			return err
		}
		if interval < 0 {
			return dp.WrapKeyErr(cfgKeyRetriesPolicyConstantInternal, errors.New("must be positive"))
		}
		c.ConstantBackoff = ConstantBackoffConfig{Interval: interval}
	}

	return nil
}

// LogConfig represents configuration options for HTTP client logs.
type LogConfig struct {
	// Enabled is a flag that enables logging.
	Enabled bool

	// SlowRequestThreshold is a threshold for slow requests.
	SlowRequestThreshold time.Duration

	// Mode of logging: [all, failed]. 'all' by default.
	Mode LoggingMode
}

// Set is part of config interface implementation.
func (c *LogConfig) Set(dp config.DataProvider) error {
	enabled, err := dp.GetBool(cfgKeyLogEnabled)
	if err != nil {
		return err
	}
	c.Enabled = enabled

	if !c.Enabled {
		return nil
	}

	slowRequestThreshold, err := dp.GetDuration(cfgKeyLogSlowRequestThreshold)
	if err != nil {
		return err
	}
	if slowRequestThreshold < 0 {
		return dp.WrapKeyErr(cfgKeyLogSlowRequestThreshold, errors.New("can not be negative"))
	}
	c.SlowRequestThreshold = slowRequestThreshold

	mode, err := dp.GetString(cfgKeyLogMode)
	if err != nil {
		return err
	}
	loggingMode := LoggingMode(mode)
	if !loggingMode.IsValid() {
		return dp.WrapKeyErr(cfgKeyLogMode, errors.New("choose one of: [all, failed]"))
	}
	c.Mode = loggingMode

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *LogConfig) SetProviderDefaults(_ config.DataProvider) {}

// TransportOpts returns transport options.
func (c *LogConfig) TransportOpts() LoggingRoundTripperOpts {
	return LoggingRoundTripperOpts{
		Mode:                 c.Mode,
		SlowRequestThreshold: c.SlowRequestThreshold,
	}
}

// MetricsConfig represents configuration options for HTTP client logs.
type MetricsConfig struct {
	// Enabled is a flag that enables metrics.
	Enabled bool
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
	Retries RetriesConfig

	// RateLimits is a configuration for HTTP client rate limits.
	RateLimits RateLimitsConfig

	// Log is a configuration for HTTP client logs.
	Log LogConfig

	// Metrics is a configuration for HTTP client metrics.
	Metrics MetricsConfig

	// Timeout is the maximum time to wait for a request to be made.
	Timeout time.Duration

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

	err = c.Log.Set(config.NewKeyPrefixedDataProvider(dp, c.keyPrefix))
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

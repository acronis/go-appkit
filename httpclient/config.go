/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"github.com/acronis/go-appkit/config"
	"time"
)

const (
	// DefaultLimit is the default limit for rate limiting.
	DefaultLimit = 10

	// DefaultBurst is the default burst for rate limiting.
	DefaultBurst = 100

	// DefaultWaitTimeout is the default wait timeout for rate limiting.
	DefaultWaitTimeout = 10 * time.Second

	// configuration properties
	cfgKeyRetriesMax            = "retries.max_retry_attempts"
	cfgKeyRateLimitsLimit       = "rate_limits.limit"
	cfgKeyRateLimitsBurst       = "rate_limits.burst"
	cfgKeyRateLimitsWaitTimeout = "rate_limits.wait_timeout"
	cfgKeyTimeout               = "timeout"
)

// RateLimitConfig represents configuration options for HTTP client rate limits.
type RateLimitConfig struct {
	Limit       int           `mapstructure:"limit"`
	Burst       int           `mapstructure:"burst"`
	WaitTimeout time.Duration `mapstructure:"wait_timeout"`
}

// Set is part of config interface implementation.
func (c *RateLimitConfig) Set(dp config.DataProvider) (err error) {
	limit, err := dp.GetInt(cfgKeyRateLimitsLimit)
	if err != nil {
		return err
	}
	c.Limit = limit

	burst, err := dp.GetInt(cfgKeyRateLimitsBurst)
	if err != nil {
		return nil
	}
	c.Burst = burst

	waitTimeout, err := dp.GetDuration(cfgKeyRateLimitsWaitTimeout)
	if err != nil {
		return err
	}
	c.WaitTimeout = waitTimeout

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *RateLimitConfig) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyRateLimitsLimit, DefaultLimit)
	dp.SetDefault(cfgKeyRateLimitsBurst, DefaultBurst)
	dp.SetDefault(cfgKeyRateLimitsWaitTimeout, DefaultWaitTimeout)
}

// TransportOpts returns transport options.
func (c *RateLimitConfig) TransportOpts() RateLimitingRoundTripperOpts {
	return RateLimitingRoundTripperOpts{
		Burst:       c.Burst,
		WaitTimeout: c.WaitTimeout,
	}
}

// RetriesConfig represents configuration options for HTTP client retries policy.
type RetriesConfig struct {
	MaxRetryAttempts int `mapstructure:"max_retry_attempts"`
}

// Set is part of config interface implementation.
func (c *RetriesConfig) Set(dp config.DataProvider) error {
	maxRetryAttempts, err := dp.GetInt(cfgKeyRetriesMax)
	if err != nil {
		return err
	}
	c.MaxRetryAttempts = maxRetryAttempts

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *RetriesConfig) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyRetriesMax, DefaultMaxRetryAttempts)
}

// TransportOpts returns transport options.
func (c *RetriesConfig) TransportOpts() RetryableRoundTripperOpts {
	return RetryableRoundTripperOpts{MaxRetryAttempts: c.MaxRetryAttempts}
}

// Config represents options for HTTP client configuration.
type Config struct {
	Retries    *RetriesConfig   `mapstructure:"retries"`
	RateLimits *RateLimitConfig `mapstructure:"rate_limits"`
	Timeout    time.Duration    `mapstructure:"timeout"`
}

// Set is part of config interface implementation.
func (c *Config) Set(dp config.DataProvider) error {
	timeout, err := dp.GetDuration(cfgKeyTimeout)
	if err != nil {
		return err
	}
	c.Timeout = timeout

	err = c.Retries.Set(config.NewKeyPrefixedDataProvider(dp, ""))
	if err != nil {
		return err
	}

	err = c.RateLimits.Set(config.NewKeyPrefixedDataProvider(dp, ""))
	if err != nil {
		return err
	}

	return nil
}

// SetProviderDefaults is part of config interface implementation.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	if c.Retries == nil {
		c.Retries = &RetriesConfig{}
	}
	c.Retries.SetProviderDefaults(dp)

	if c.RateLimits == nil {
		c.RateLimits = &RateLimitConfig{}
	}
	c.RateLimits.SetProviderDefaults(dp)

	if c.Timeout == 0 {
		c.Timeout = DefaultWaitTimeout
	}
}

// NewHTTPClientConfig is unified configuration format for HTTP clients.
func NewHTTPClientConfig() *Config {
	return &Config{
		Retries: &RetriesConfig{
			MaxRetryAttempts: DefaultMaxRetryAttempts,
		},
		RateLimits: &RateLimitConfig{
			Limit:       DefaultLimit,
			Burst:       DefaultBurst,
			WaitTimeout: DefaultWaitTimeout,
		},
		Timeout: DefaultWaitTimeout,
	}
}

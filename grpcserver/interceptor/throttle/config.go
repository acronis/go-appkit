/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"fmt"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/internal/throttleconfig"
)

// Rate-limiting algorithms.
const (
	RateLimitAlgLeakyBucket   = throttleconfig.RateLimitAlgLeakyBucket
	RateLimitAlgSlidingWindow = throttleconfig.RateLimitAlgSlidingWindow
)

// ZoneKeyType is a type of keys zone.
type ZoneKeyType = throttleconfig.ZoneKeyType

// Zone key types.
const (
	ZoneKeyTypeNoKey      = throttleconfig.ZoneKeyTypeNoKey
	ZoneKeyTypeIdentity   = throttleconfig.ZoneKeyTypeIdentity
	ZoneKeyTypeHeader     = throttleconfig.ZoneKeyTypeHeader
	ZoneKeyTypeRemoteAddr = throttleconfig.ZoneKeyTypeRemoteAddr
)

// ZoneKeyConfig represents a configuration of zone's key.
type ZoneKeyConfig = throttleconfig.ZoneKeyConfig

// RuleRateLimit represents rule's rate limiting parameters.
type RuleRateLimit = throttleconfig.RuleRateLimit

// RuleInFlightLimit represents rule's in-flight limiting parameters.
type RuleInFlightLimit = throttleconfig.RuleInFlightLimit

// RateLimitRetryAfterValue represents structured retry-after value for rate limiting.
type RateLimitRetryAfterValue = throttleconfig.RateLimitRetryAfterValue

// RateLimitValue represents value for rate limiting.
type RateLimitValue = throttleconfig.RateLimitValue

// TagsList represents a list of tags.
type TagsList = throttleconfig.TagsList

// Config represents a configuration for throttling of gRPC requests on the server side.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	// RateLimitZones contains rate limiting zones.
	// Key is a zone's name, and value is a zone's configuration.
	RateLimitZones map[string]RateLimitZoneConfig `mapstructure:"rateLimitZones" yaml:"rateLimitZones" json:"rateLimitZones"`

	// InFlightLimitZones contains in-flight limiting zones.
	// Key is a zone's name, and value is a zone's configuration.
	InFlightLimitZones map[string]InFlightLimitZoneConfig `mapstructure:"inFlightLimitZones" yaml:"inFlightLimitZones" json:"inFlightLimitZones"` //nolint:lll

	// Rules contains list of so-called throttling rules.
	// Basically, throttling rule represents a gRPC service/method pattern,
	// and rate/in-flight limiting zones based on which all matched gRPC requests will be throttled.
	Rules []RuleConfig `mapstructure:"rules" yaml:"rules" json:"rules"`

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
	var opts configOptions
	for _, opt := range options {
		opt(&opts)
	}
	return &Config{keyPrefix: opts.keyPrefix}
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
// Implements config.KeyPrefixProvider interface.
func (c *Config) KeyPrefix() string {
	return c.keyPrefix
}

// SetProviderDefaults sets default configuration values for logger in config.DataProvider.
// Implements config.Config interface.
func (c *Config) SetProviderDefaults(_ config.DataProvider) {
}

// Set sets throttling configuration values from config.DataProvider.
// Implements config.Config interface.
func (c *Config) Set(dp config.DataProvider) error {
	if err := dp.Unmarshal(c, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.DecodeHook = MapstructureDecodeHook()
	}); err != nil {
		return err
	}
	return c.Validate()
}

// Validate validates configuration.
func (c *Config) Validate() error {
	for zoneName, zone := range c.RateLimitZones {
		if err := zone.Validate(); err != nil {
			return fmt.Errorf("validate rate limit zone %q: %w", zoneName, err)
		}
	}
	for zoneName, zone := range c.InFlightLimitZones {
		if err := zone.Validate(); err != nil {
			return fmt.Errorf("validate in-flight limit zone %q: %w", zoneName, err)
		}
	}
	for _, rule := range c.Rules {
		if err := rule.Validate(c.RateLimitZones, c.InFlightLimitZones); err != nil {
			return fmt.Errorf("validate rule %q: %w", rule.Name(), err)
		}
	}
	return nil
}

// ZoneConfig represents a basic zone configuration.
type ZoneConfig struct {
	Key          ZoneKeyConfig `mapstructure:"key" yaml:"key" json:"key"`
	MaxKeys      int           `mapstructure:"maxKeys" yaml:"maxKeys" json:"maxKeys"`
	DryRun       bool          `mapstructure:"dryRun" yaml:"dryRun" json:"dryRun"`
	IncludedKeys []string      `mapstructure:"includedKeys" yaml:"includedKeys" json:"includedKeys"`
	ExcludedKeys []string      `mapstructure:"excludedKeys" yaml:"excludedKeys" json:"excludedKeys"`
}

// Validate validates zone configuration.
func (c *ZoneConfig) Validate() error {
	if err := c.Key.Validate(); err != nil {
		return err
	}
	if c.MaxKeys < 0 {
		return fmt.Errorf("maximum keys should be >= 0, got %d", c.MaxKeys)
	}
	if len(c.IncludedKeys) != 0 && len(c.ExcludedKeys) != 0 {
		return fmt.Errorf("included and excluded lists cannot be specified at the same time")
	}
	return nil
}

// RateLimitZoneConfig represents zone configuration for rate limiting.
type RateLimitZoneConfig struct {
	ZoneConfig         `mapstructure:",squash" yaml:",inline"`
	Alg                string                   `mapstructure:"alg" yaml:"alg" json:"alg"`
	RateLimit          RateLimitValue           `mapstructure:"rateLimit" yaml:"rateLimit" json:"rateLimit"`
	BurstLimit         int                      `mapstructure:"burstLimit" yaml:"burstLimit" json:"burstLimit"`
	BacklogLimit       int                      `mapstructure:"backlogLimit" yaml:"backlogLimit" json:"backlogLimit"`
	BacklogTimeout     config.TimeDuration      `mapstructure:"backlogTimeout" yaml:"backlogTimeout" json:"backlogTimeout"`
	ResponseRetryAfter RateLimitRetryAfterValue `mapstructure:"responseRetryAfter" yaml:"responseRetryAfter" json:"responseRetryAfter"`
}

// Validate validates zone configuration for rate limiting.
func (c *RateLimitZoneConfig) Validate() error {
	if err := c.ZoneConfig.Validate(); err != nil {
		return err
	}
	if c.Alg != "" && c.Alg != RateLimitAlgLeakyBucket && c.Alg != RateLimitAlgSlidingWindow {
		return fmt.Errorf("unknown rate limit alg %q", c.Alg)
	}
	if c.RateLimit.Count < 1 {
		return fmt.Errorf("rate limit should be >= 1, got %d", c.RateLimit.Count)
	}
	if c.BurstLimit < 0 {
		return fmt.Errorf("burst limit should be >= 0, got %d", c.BurstLimit)
	}
	if c.BacklogLimit < 0 {
		return fmt.Errorf("backlog limit should be >= 0, got %d", c.BacklogLimit)
	}
	return nil
}

// InFlightLimitZoneConfig represents zone configuration for in-flight limiting.
type InFlightLimitZoneConfig struct {
	ZoneConfig         `mapstructure:",squash" yaml:",inline"`
	InFlightLimit      int                 `mapstructure:"inFlightLimit" yaml:"inFlightLimit" json:"inFlightLimit"`
	BacklogLimit       int                 `mapstructure:"backlogLimit" yaml:"backlogLimit" json:"backlogLimit"`
	BacklogTimeout     config.TimeDuration `mapstructure:"backlogTimeout" yaml:"backlogTimeout" json:"backlogTimeout"`
	ResponseRetryAfter config.TimeDuration `mapstructure:"responseRetryAfter" yaml:"responseRetryAfter" json:"responseRetryAfter"`
}

// Validate validates zone configuration for in-flight limiting.
func (c *InFlightLimitZoneConfig) Validate() error {
	if err := c.ZoneConfig.Validate(); err != nil {
		return err
	}
	if c.InFlightLimit < 1 {
		return fmt.Errorf("in-flight limit should be >= 1, got %d", c.InFlightLimit)
	}
	if c.BacklogLimit < 0 {
		return fmt.Errorf("backlog limit should be >= 0, got %d", c.BacklogLimit)
	}
	return nil
}

// RuleConfig represents configuration for throttling rule.
type RuleConfig struct {
	// Alias is an alternative name for the rule. It will be used as a label in metrics.
	Alias string `mapstructure:"alias" yaml:"alias" json:"alias"`

	// ServiceMethods contains a list of gRPC service methods for which the rule will be applied.
	// Patterns like "/package.Service/Method" or "/package.Service/*" are supported.
	ServiceMethods []string `mapstructure:"serviceMethods" yaml:"serviceMethods" json:"serviceMethods"`

	// ExcludedServiceMethods contains list of gRPC service methods to be excluded from throttling limitations.
	ExcludedServiceMethods []string `mapstructure:"excludedServiceMethods" yaml:"excludedServiceMethods" json:"excludedServiceMethods"`

	// Tags is useful when the different rules of the same config should be used by different interceptors.
	Tags TagsList `mapstructure:"tags" yaml:"tags" json:"tags"`

	// RateLimits contains a list of the rate limiting zones that are used in the rule.
	RateLimits []RuleRateLimit `mapstructure:"rateLimits" yaml:"rateLimits" json:"rateLimits"`

	// InFlightLimits contains a list of the in-flight limiting zones that are used in the rule.
	InFlightLimits []RuleInFlightLimit `mapstructure:"inFlightLimits" yaml:"inFlightLimits" json:"inFlightLimits"`
}

// Name returns throttling rule name.
func (c *RuleConfig) Name() string {
	if c.Alias != "" {
		return c.Alias
	}
	return strings.Join(c.ServiceMethods, "; ")
}

// Validate validates throttling rule configuration.
func (c *RuleConfig) Validate(
	rateLimitZones map[string]RateLimitZoneConfig, inFlightLimitZones map[string]InFlightLimitZoneConfig,
) error {
	for _, zone := range c.RateLimits {
		if _, ok := rateLimitZones[zone.Zone]; !ok {
			return fmt.Errorf("rate limit zone %q is undefined", zone.Zone)
		}
	}
	for _, zone := range c.InFlightLimits {
		if _, ok := inFlightLimitZones[zone.Zone]; !ok {
			return fmt.Errorf("in-flight limit zone %q is undefined", zone.Zone)
		}
	}

	if len(c.ServiceMethods) == 0 {
		return fmt.Errorf("serviceMethods is missing")
	}

	for _, method := range c.ServiceMethods {
		if err := validateServiceMethodPattern(method); err != nil {
			return fmt.Errorf("serviceMethod %q %w", method, err)
		}
	}

	for _, method := range c.ExcludedServiceMethods {
		if err := validateServiceMethodPattern(method); err != nil {
			return fmt.Errorf("excludedServiceMethod %q %w", method, err)
		}
	}

	return nil
}

// validateServiceMethodPattern validates that a gRPC service method pattern is valid.
func validateServiceMethodPattern(method string) error {
	wildcardCount := strings.Count(method, "*")
	if wildcardCount > 1 {
		return fmt.Errorf("contains multiple wildcards or non-trailing wildcard")
	}
	if wildcardCount == 1 && !strings.HasSuffix(method, "*") {
		return fmt.Errorf("contains multiple wildcards or non-trailing wildcard")
	}
	return nil
}

// MapstructureDecodeHook returns a DecodeHookFunc for mapstructure to handle custom types.
func MapstructureDecodeHook() mapstructure.DecodeHookFunc {
	return throttleconfig.MapstructureDecodeHook()
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/restapi"
)

// Rate-limiting algorithms.
const (
	RateLimitAlgLeakyBucket   = "leaky_bucket"
	RateLimitAlgSlidingWindow = "sliding_window"
)

// Config represents a configuration for throttling of HTTP requests on the server side.
type Config struct {
	// RateLimitZones contains rate limiting zones.
	// Key is a zone's name, and value is a zone's configuration.
	RateLimitZones map[string]RateLimitZoneConfig `mapstructure:"rateLimitZones" yaml:"rateLimitZones"`

	// InFlightLimitZones contains in-flight limiting zones.
	// Key is a zone's name, and value is a zone's configuration.
	InFlightLimitZones map[string]InFlightLimitZoneConfig `mapstructure:"inFlightLimitZones" yaml:"inFlightLimitZones"`

	// Rules contains list of so-called throttling rules.
	// Basically, throttling rule represents a route (or multiple routes),
	// and rate/in-flight limiting zones based on which all matched HTTP requests will be throttled.
	Rules []RuleConfig `mapstructure:"rules" yaml:"rules"`

	keyPrefix string
}

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

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

// SetProviderDefaults sets default configuration values for logger in config.DataProvider.
func (c *Config) SetProviderDefaults(_ config.DataProvider) {
}

// Set sets throttling configuration values from config.DataProvider.
func (c *Config) Set(dp config.DataProvider) error {
	if err := dp.Unmarshal(c, func(decoderConfig *mapstructure.DecoderConfig) {
		decoderConfig.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
			mapstructure.TextUnmarshallerHookFunc(),
			mapstructureTrimSpaceStringsHookFunc(),
		)
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
	Key                ZoneKeyConfig `mapstructure:"key" yaml:"key"`
	MaxKeys            int           `mapstructure:"maxKeys" yaml:"maxKeys"`
	ResponseStatusCode int           `mapstructure:"responseStatusCode" yaml:"responseStatusCode"`
	DryRun             bool          `mapstructure:"dryRun" yaml:"dryRun"`
	IncludedKeys       []string      `mapstructure:"includedKeys" yaml:"includedKeys"`
	ExcludedKeys       []string      `mapstructure:"excludedKeys" yaml:"excludedKeys"`
}

// Validate validates zone configuration.
func (c *ZoneConfig) Validate() error {
	if err := c.Key.Validate(); err != nil {
		return err
	}
	if c.ResponseStatusCode < 0 {
		return fmt.Errorf("response status code should be >= 0, got %d", c.ResponseStatusCode)
	}
	if c.MaxKeys < 0 {
		return fmt.Errorf("maximum keys should be >= 0, got %d", c.MaxKeys)
	}
	if len(c.IncludedKeys) != 0 && len(c.ExcludedKeys) != 0 {
		return fmt.Errorf("included and excluded lists cannot be specified at the same time")
	}
	return nil
}

func (c *ZoneConfig) getResponseStatusCode() int {
	if c.ResponseStatusCode != 0 {
		return c.ResponseStatusCode
	}
	if c.Key.Type == ZoneKeyTypeIdentity {
		return http.StatusTooManyRequests
	}
	return http.StatusServiceUnavailable
}

// RateLimitZoneConfig represents zone configuration for rate limiting.
type RateLimitZoneConfig struct {
	ZoneConfig         `mapstructure:",squash" yaml:",inline"`
	Alg                string                   `mapstructure:"alg" yaml:"alg"`
	RateLimit          RateLimitValue           `mapstructure:"rateLimit" yaml:"rateLimit"`
	BurstLimit         int                      `mapstructure:"burstLimit" yaml:"burstLimit"`
	BacklogLimit       int                      `mapstructure:"backlogLimit" yaml:"backlogLimit"`
	BacklogTimeout     time.Duration            `mapstructure:"backlogTimeout" yaml:"backlogTimeout"`
	ResponseRetryAfter RateLimitRetryAfterValue `mapstructure:"responseRetryAfter" yaml:"responseRetryAfter"`
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
	InFlightLimit      int           `mapstructure:"inFlightLimit" yaml:"inFlightLimit"`
	BacklogLimit       int           `mapstructure:"backlogLimit" yaml:"backlogLimit"`
	BacklogTimeout     time.Duration `mapstructure:"backlogTimeout" yaml:"backlogTimeout"`
	ResponseRetryAfter time.Duration `mapstructure:"responseRetryAfter" yaml:"responseRetryAfter"`
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

// ZoneKeyType is a type of keys zone.
type ZoneKeyType string

// Zone key types.
const (
	ZoneKeyTypeNoKey      ZoneKeyType = ""
	ZoneKeyTypeIdentity   ZoneKeyType = "identity"
	ZoneKeyTypeHTTPHeader ZoneKeyType = "header"
	ZoneKeyTypeRemoteAddr ZoneKeyType = "remote_addr"
)

// ZoneKeyConfig represents a configuration of zone's key.
type ZoneKeyConfig struct {
	// Type determines type of key that will be used for throttling.
	Type ZoneKeyType `mapstructure:"type" yaml:"type"`

	// HeaderName is a name of the HTTP request header which value will be used as a key.
	// Matters only when Type is a "header".
	HeaderName string `mapstructure:"headerName" yaml:"headerName"`

	// NoBypassEmpty specifies whether throttling will be used if the value obtained by the key is empty.
	NoBypassEmpty bool `mapstructure:"noBypassEmpty" yaml:"noBypassEmpty"`
}

// Validate validates keys zone configuration.
func (c *ZoneKeyConfig) Validate() error {
	switch c.Type {
	case ZoneKeyTypeNoKey, ZoneKeyTypeIdentity, ZoneKeyTypeRemoteAddr:
	case ZoneKeyTypeHTTPHeader:
		if c.HeaderName == "" {
			return fmt.Errorf("header name should be specified for %q key zone type", ZoneKeyTypeHTTPHeader)
		}
	default:
		return fmt.Errorf("unknown key zone type %q", c.Type)
	}
	return nil
}

// RuleConfig represents configuration for throttling rule.
type RuleConfig struct {
	// Alias is an alternative name for the rule. It will be used as a label in metrics.
	Alias string `mapstructure:"alias" yaml:"alias"`

	// Routes contains a list of routes (HTTP verb + URL path) for which the rule will be applied.
	Routes []restapi.RouteConfig `mapstructure:"routes" yaml:"routes"`

	// ExcludedRoutes contains list of routes (HTTP verb + URL path) to be excluded from throttling limitations.
	// The following service endpoints fit should typically be added to this list:
	// - healthcheck endpoint serving as readiness probe
	// - status endpoint serving as liveness probe
	ExcludedRoutes []restapi.RouteConfig `mapstructure:"excludedRoutes" yaml:"excludedRoutes"`

	// Tags is useful when the different rules of the same config should be used by different middlewares.
	// As example let's suppose we would like to have 2 different throttling rules:
	// 1) for absolutely all requests;
	// 2) for all identity-aware (authorized) requests.
	// In the code, we will have 2 middlewares that will be executed on the different steps of the HTTP request serving,
	// and each one should do only its own throttling.
	// We can achieve this using different tags for rules and passing needed tag in the MiddlewareOpts.
	Tags []string `mapstructure:"tags" yaml:"tags"`

	// RateLimits contains a list of the rate limiting zones that are used in the rule.
	RateLimits []RuleRateLimit `mapstructure:"rateLimits" yaml:"rateLimits"`

	// InFlightLimits contains a list of the in-flight limiting zones that are used in the rule.
	InFlightLimits []RuleInFlightLimit `mapstructure:"inFlightLimits" yaml:"inFlightLimits"`
}

// Name returns throttling rule name.
func (c *RuleConfig) Name() string {
	if c.Alias != "" {
		return c.Alias
	}
	parts := make([]string, 0, len(c.Routes))
	for _, r := range c.Routes {
		parts = append(parts, strings.TrimSpace(strings.Join(r.Methods, "|")+" "+r.Path.Raw))
	}
	return strings.Join(parts, "; ")
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

	if len(c.Routes) == 0 {
		return fmt.Errorf("routes is missing")
	}

	for i := range c.Routes {
		err := c.Routes[i].Validate()
		if err != nil {
			return fmt.Errorf("validate route #%d: %w", i+1, err)
		}
	}
	for i := range c.ExcludedRoutes {
		err := c.ExcludedRoutes[i].Validate()
		if err != nil {
			return fmt.Errorf("validate excluded route #%d: %w", i+1, err)
		}
	}

	return nil
}

// RuleRateLimit represents rule's rate limiting parameters.
type RuleRateLimit struct {
	Zone string `mapstructure:"zone" yaml:"zone"`
}

// RuleInFlightLimit represents rule's in-flight limiting parameters.
type RuleInFlightLimit struct {
	Zone string `mapstructure:"zone" yaml:"zone"`
}

// RateLimitRetryAfterValue represents structured retry-after value for rate limiting.
type RateLimitRetryAfterValue struct {
	IsAuto   bool
	Duration time.Duration
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (ra *RateLimitRetryAfterValue) UnmarshalText(text []byte) error {
	switch v := string(text); v {
	case "":
		*ra = RateLimitRetryAfterValue{Duration: 0}
	case "auto":
		*ra = RateLimitRetryAfterValue{IsAuto: true}
	default:
		dur, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		*ra = RateLimitRetryAfterValue{Duration: dur}
	}
	return nil
}

// RateLimitValue represents value for rate limiting.
type RateLimitValue struct {
	Count    int
	Duration time.Duration
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (rl *RateLimitValue) UnmarshalText(text []byte) error {
	rate := string(text)
	incorrectFormatErr := fmt.Errorf(
		"incorrect format for rate %q, should be N/(s|m|h), for example 10/s, 100/m, 1000/h", rate)
	parts := strings.SplitN(rate, "/", 2)
	if len(parts) != 2 {
		return incorrectFormatErr
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return incorrectFormatErr
	}
	var dur time.Duration
	switch strings.ToLower(parts[1]) {
	case "s":
		dur = time.Second
	case "m":
		dur = time.Minute
	case "h":
		dur = time.Hour
	default:
		return incorrectFormatErr
	}
	*rl = RateLimitValue{Count: count, Duration: dur}
	return nil
}

func mapstructureTrimSpaceStringsHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Kind,
		t reflect.Kind,
		data interface{}) (interface{}, error) {
		if f != reflect.Slice || t != reflect.Slice {
			return data, nil
		}
		switch dt := data.(type) {
		case []string:
			res := make([]string, 0, len(dt))
			for _, s := range dt {
				res = append(res, strings.TrimSpace(s))
			}
			return res, nil
		default:
			return data, nil
		}
	}
}

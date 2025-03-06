/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/restapi"
)

// Rate-limiting algorithms.
const (
	RateLimitAlgLeakyBucket   = "leaky_bucket"
	RateLimitAlgSlidingWindow = "sliding_window"
)

// Config represents a configuration for throttling of HTTP requests on the server side.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	// RateLimitZones contains rate limiting zones.
	// Key is a zone's name, and value is a zone's configuration.
	RateLimitZones map[string]RateLimitZoneConfig `mapstructure:"rateLimitZones" yaml:"rateLimitZones" json:"rateLimitZones"`

	// InFlightLimitZones contains in-flight limiting zones.
	// Key is a zone's name, and value is a zone's configuration.
	InFlightLimitZones map[string]InFlightLimitZoneConfig `mapstructure:"inFlightLimitZones" yaml:"inFlightLimitZones" json:"inFlightLimitZones"` // nolint: lll

	// Rules contains list of so-called throttling rules.
	// Basically, throttling rule represents a route (or multiple routes),
	// and rate/in-flight limiting zones based on which all matched HTTP requests will be throttled.
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

// NewConfigWithKeyPrefix creates a new instance of the Config with a key prefix.
// This prefix will be used by config.Loader.
// Deprecated: use NewConfig with WithKeyPrefix instead.
func NewConfigWithKeyPrefix(keyPrefix string) *Config {
	return &Config{keyPrefix: keyPrefix}
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
	Key                ZoneKeyConfig `mapstructure:"key" yaml:"key" json:"key"`
	MaxKeys            int           `mapstructure:"maxKeys" yaml:"maxKeys" json:"maxKeys"`
	ResponseStatusCode int           `mapstructure:"responseStatusCode" yaml:"responseStatusCode" json:"responseStatusCode"`
	DryRun             bool          `mapstructure:"dryRun" yaml:"dryRun" json:"dryRun"`
	IncludedKeys       []string      `mapstructure:"includedKeys" yaml:"includedKeys" json:"includedKeys"`
	ExcludedKeys       []string      `mapstructure:"excludedKeys" yaml:"excludedKeys" json:"excludedKeys"`
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
	Type ZoneKeyType `mapstructure:"type" yaml:"type" json:"type"`

	// HeaderName is a name of the HTTP request header which value will be used as a key.
	// Matters only when Type is a "header".
	HeaderName string `mapstructure:"headerName" yaml:"headerName" json:"headerName"`

	// NoBypassEmpty specifies whether throttling will be used if the value obtained by the key is empty.
	NoBypassEmpty bool `mapstructure:"noBypassEmpty" yaml:"noBypassEmpty" json:"noBypassEmpty"`
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
	Alias string `mapstructure:"alias" yaml:"alias" json:"alias"`

	// Routes contains a list of routes (HTTP verb + URL path) for which the rule will be applied.
	Routes []restapi.RouteConfig `mapstructure:"routes" yaml:"routes" json:"routes"`

	// ExcludedRoutes contains list of routes (HTTP verb + URL path) to be excluded from throttling limitations.
	// The following service endpoints fit should typically be added to this list:
	// - healthcheck endpoint serving as readiness probe
	// - status endpoint serving as liveness probe
	ExcludedRoutes []restapi.RouteConfig `mapstructure:"excludedRoutes" yaml:"excludedRoutes" json:"excludedRoutes"`

	// Tags is useful when the different rules of the same config should be used by different middlewares.
	// As example let's suppose we would like to have 2 different throttling rules:
	// 1) for absolutely all requests;
	// 2) for all identity-aware (authorized) requests.
	// In the code, we will have 2 middlewares that will be executed on the different steps of the HTTP request serving,
	// and each one should do only its own throttling.
	// We can achieve this using different tags for rules and passing needed tag in the MiddlewareOpts.
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

const rateLimitRetryAfterAuto = "auto"

// String returns a string representation of the retry-after value.
// Implements fmt.Stringer interface.
func (ra RateLimitRetryAfterValue) String() string {
	if ra.IsAuto {
		return rateLimitRetryAfterAuto
	}
	return ra.Duration.String()
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// Implements the encoding.TextUnmarshaler interface which is used by mapstructure.TextUnmarshallerHookFunc.
func (ra *RateLimitRetryAfterValue) UnmarshalText(text []byte) error {
	return ra.unmarshal(string(text))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// Implements the json.Unmarshaler interface.
func (ra *RateLimitRetryAfterValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	return ra.unmarshal(text)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
// Implements the yaml.Unmarshaler interface.
func (ra *RateLimitRetryAfterValue) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		return err
	}
	return ra.unmarshal(text)
}

func (ra *RateLimitRetryAfterValue) unmarshal(retryAfterVal string) error {
	switch v := retryAfterVal; v {
	case "":
		*ra = RateLimitRetryAfterValue{Duration: 0}
	case rateLimitRetryAfterAuto:
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

// MarshalText implements the encoding.TextMarshaler interface.
func (ra RateLimitRetryAfterValue) MarshalText() ([]byte, error) {
	return []byte(ra.String()), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (ra RateLimitRetryAfterValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(ra.String())
}

// MarshalYAML implements the yaml.Marshaler interface.
func (ra RateLimitRetryAfterValue) MarshalYAML() (interface{}, error) {
	return ra.String(), nil
}

// RateLimitValue represents value for rate limiting.
type RateLimitValue struct {
	Count    int
	Duration time.Duration
}

// String returns a string representation of the rate limit value.
// Implements fmt.Stringer interface.
func (rl RateLimitValue) String() string {
	if rl.Duration == 0 && rl.Count == 0 {
		return ""
	}
	var d string
	switch rl.Duration {
	case time.Second:
		d = "s"
	case time.Minute:
		d = "m"
	case time.Hour:
		d = "h"
	default:
		d = rl.Duration.String()
	}
	return fmt.Sprintf("%d/%s", rl.Count, d)
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
// Implements the encoding.TextUnmarshaler interface which is used by mapstructure.TextUnmarshallerHookFunc.
func (rl *RateLimitValue) UnmarshalText(text []byte) error {
	return rl.unmarshal(string(text))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
// Implements the json.Unmarshaler interface.
func (rl *RateLimitValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	return rl.unmarshal(text)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
// Implements the yaml.Unmarshaler interface.
func (rl *RateLimitValue) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		return err
	}
	return rl.unmarshal(text)
}

func (rl *RateLimitValue) unmarshal(rate string) error {
	if rate == "" {
		*rl = RateLimitValue{}
		return nil
	}
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

// MarshalText implements the encoding.TextMarshaler interface.
func (rl RateLimitValue) MarshalText() ([]byte, error) {
	return []byte(rl.String()), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (rl RateLimitValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(rl.String())
}

// MarshalYAML implements the yaml.Marshaler interface.
func (rl RateLimitValue) MarshalYAML() (interface{}, error) {
	return rl.String(), nil
}

// TagsList represents a list of tags.
type TagsList []string

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (tl *TagsList) UnmarshalText(text []byte) error {
	tl.unmarshal(string(text))
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (tl *TagsList) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		tl.unmarshal(s)
		return nil
	}
	var l []string
	if err := json.Unmarshal(data, &l); err == nil {
		*tl = l
		return nil
	}
	return fmt.Errorf("invalid methods list: %s", data)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (tl *TagsList) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		tl.unmarshal(s)
		return nil
	}
	var l []string
	if err := value.Decode(&l); err == nil {
		*tl = l
		return nil
	}
	return fmt.Errorf("invalid methods list: %v", value)
}

func (tl *TagsList) unmarshal(data string) {
	data = strings.TrimSpace(data)
	if data == "" {
		*tl = TagsList{}
		return
	}
	methods := strings.Split(data, ",")
	for _, m := range methods {
		*tl = append(*tl, strings.TrimSpace(m))
	}
}

func (tl TagsList) String() string {
	return strings.Join(tl, ",")
}

// MarshalText implements the encoding.TextMarshaler interface.
func (tl TagsList) MarshalText() ([]byte, error) {
	return []byte(tl.String()), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (tl TagsList) MarshalJSON() ([]byte, error) {
	return json.Marshal(tl.String())
}

// MarshalYAML implements the yaml.Marshaler interface.
func (tl TagsList) MarshalYAML() (interface{}, error) {
	return tl.String(), nil
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

// MapstructureDecodeHook returns a DecodeHookFunc for mapstructure to handle custom types.
func MapstructureDecodeHook() mapstructure.DecodeHookFunc {
	return mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.TextUnmarshallerHookFunc(),
		mapstructureTrimSpaceStringsHookFunc(),
	)
}

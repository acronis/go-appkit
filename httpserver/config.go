/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"fmt"
	"time"

	"github.com/acronis/go-appkit/config"
)

const cfgDefaultKeyPrefix = "server"

const (
	cfgKeyServerAddress                 = "address"
	cfgKeyServerUnixSocketPath          = "unixSocketPath"
	cfgKeyServerTLSCert                 = "tls.cert"
	cfgKeyServerTLSKey                  = "tls.key"
	cfgKeyServerTLSEnabled              = "tls.enabled"
	cfgKeyServerTimeoutsWrite           = "timeouts.write"
	cfgKeyServerTimeoutsRead            = "timeouts.read"
	cfgKeyServerTimeoutsReadHeader      = "timeouts.readHeader"
	cfgKeyServerTimeoutsIdle            = "timeouts.idle"
	cfgKeyServerTimeoutsShutdown        = "timeouts.shutdown"
	cfgKeyServerLimitsMaxRequests       = "limits.maxRequests"
	cfgKeyServerLimitsMaxBodySize       = "limits.maxBodySize"
	cfgKeyServerLogRequestStart         = "log.requestStart"
	cfgKeyServerLogRequestHeaders       = "log.requestHeaders"
	cfgKeyServerLogExcludedEndpoints    = "log.excludedEndpoints"
	cfgKeyServerLogSecretQueryParams    = "log.secretQueryParams" // nolint:gosec // false positive
	cfgKeyServerLogAddRequestInfo       = "log.addRequestInfo"
	cfgKeyServerLogSlowRequestThreshold = "log.slowRequestThreshold"
)

const (
	defaultServerAddress            = ":8080"
	defaultServerTimeoutsWrite      = time.Minute
	defaultServerTimeoutsRead       = time.Second * 15
	defaultServerTimeoutsReadHeader = time.Second * 10
	defaultServerTimeoutsIdle       = time.Minute
	defaultServerTimeoutsShutdown   = time.Second * 5
	defaultSlowRequestThreshold     = time.Second
	defaultServerLimitsMaxRequests  = 5000
)

// Config represents a set of configuration parameters for HTTPServer.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	Address        string         `mapstructure:"address" yaml:"address" json:"address"`
	UnixSocketPath string         `mapstructure:"unixSocketPath" yaml:"unixSocketPath" json:"unixSocketPath"`
	Timeouts       TimeoutsConfig `mapstructure:"timeouts" yaml:"timeouts" json:"timeouts"`
	Limits         LimitsConfig   `mapstructure:"limits" yaml:"limits" json:"limits"`
	Log            LogConfig      `mapstructure:"log" yaml:"log" json:"log"`
	TLS            TLSConfig      `mapstructure:"tls" yaml:"tls" json:"tls"`

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
	opts := configOptions{keyPrefix: cfgDefaultKeyPrefix} // cfgDefaultKeyPrefix is used here for backward compatibility
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
		Address:   defaultServerAddress,
		Timeouts: TimeoutsConfig{
			Write:      config.TimeDuration(defaultServerTimeoutsWrite),
			Read:       config.TimeDuration(defaultServerTimeoutsRead),
			ReadHeader: config.TimeDuration(defaultServerTimeoutsReadHeader),
			Idle:       config.TimeDuration(defaultServerTimeoutsIdle),
			Shutdown:   config.TimeDuration(defaultServerTimeoutsShutdown),
		},
		Limits: LimitsConfig{
			MaxRequests: defaultServerLimitsMaxRequests,
		},
		Log: LogConfig{
			SlowRequestThreshold: config.TimeDuration(defaultSlowRequestThreshold),
		},
	}
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
// Implements config.KeyPrefixProvider interface.
func (c *Config) KeyPrefix() string {
	if c.keyPrefix == "" {
		return cfgDefaultKeyPrefix
	}
	return c.keyPrefix
}

// SetProviderDefaults sets default configuration values for HTTPServer in config.DataProvider.
// Implements config.Config interface.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyServerAddress, defaultServerAddress)

	dp.SetDefault(cfgKeyServerTimeoutsWrite, defaultServerTimeoutsWrite)
	dp.SetDefault(cfgKeyServerTimeoutsRead, defaultServerTimeoutsRead)
	dp.SetDefault(cfgKeyServerTimeoutsReadHeader, defaultServerTimeoutsReadHeader)
	dp.SetDefault(cfgKeyServerTimeoutsIdle, defaultServerTimeoutsIdle)
	dp.SetDefault(cfgKeyServerTimeoutsShutdown, defaultServerTimeoutsShutdown)

	dp.SetDefault(cfgKeyServerLimitsMaxRequests, defaultServerLimitsMaxRequests)

	dp.SetDefault(cfgKeyServerLogRequestStart, false)
	dp.SetDefault(cfgKeyServerLogAddRequestInfo, false)
	dp.SetDefault(cfgKeyServerLogSlowRequestThreshold, defaultSlowRequestThreshold)
}

// TimeoutsConfig represents a set of configuration parameters for HTTPServer relating to timeouts.
type TimeoutsConfig struct {
	Write      config.TimeDuration `mapstructure:"write" yaml:"write" json:"write"`
	Read       config.TimeDuration `mapstructure:"read" yaml:"read" json:"read"`
	ReadHeader config.TimeDuration `mapstructure:"readHeader" yaml:"readHeader" json:"readHeader"`
	Idle       config.TimeDuration `mapstructure:"idle" yaml:"idle" json:"idle"`
	Shutdown   config.TimeDuration `mapstructure:"shutdown" yaml:"shutdown" json:"shutdown"`
}

// Set sets timeout server configuration values from config.DataProvider.
// Implements config.Config interface.
func (t *TimeoutsConfig) Set(dp config.DataProvider) error {
	var err error
	var dur time.Duration

	if dur, err = dp.GetDuration(cfgKeyServerTimeoutsWrite); err != nil {
		return err
	}
	t.Write = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerTimeoutsRead); err != nil {
		return err
	}
	t.Read = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerTimeoutsReadHeader); err != nil {
		return err
	}
	t.ReadHeader = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerTimeoutsIdle); err != nil {
		return err
	}
	t.Idle = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerTimeoutsShutdown); err != nil {
		return err
	}
	t.Shutdown = config.TimeDuration(dur)

	return nil
}

// LimitsConfig represents a set of configuration parameters for HTTPServer relating to limits.
type LimitsConfig struct {
	// MaxRequests is the maximum number of requests that can be processed concurrently.
	MaxRequests int `mapstructure:"maxRequests" yaml:"maxRequests" json:"maxRequests"`

	// MaxBodySizeBytes is the maximum size of the request body in bytes.
	MaxBodySizeBytes config.BytesCount `mapstructure:"maxBodySize" yaml:"maxBodySize" json:"maxBodySize"`
}

// Set sets limit server configuration values from config.DataProvider.
func (l *LimitsConfig) Set(dp config.DataProvider) error {
	var err error

	if l.MaxRequests, err = dp.GetInt(cfgKeyServerLimitsMaxRequests); err != nil {
		return err
	}
	if l.MaxRequests < 0 {
		return dp.WrapKeyErr(cfgKeyServerLimitsMaxRequests, fmt.Errorf("maxRequests must be positive"))
	}

	if l.MaxBodySizeBytes, err = dp.GetBytesCount(cfgKeyServerLimitsMaxBodySize); err != nil {
		return dp.WrapKeyErr(cfgKeyServerLimitsMaxBodySize, err)
	}

	return nil
}

// LogConfig represents a set of configuration parameters for HTTPServer relating to logging.
type LogConfig struct {
	RequestStart           bool                `mapstructure:"requestStart" yaml:"requestStart" json:"requestStart"`
	RequestHeaders         []string            `mapstructure:"requestHeaders" yaml:"requestHeaders" json:"requestHeaders"`
	ExcludedEndpoints      []string            `mapstructure:"excludedEndpoints" yaml:"excludedEndpoints" json:"excludedEndpoints"`
	SecretQueryParams      []string            `mapstructure:"secretQueryParams" yaml:"secretQueryParams"`
	AddRequestInfoToLogger bool                `mapstructure:"addRequestInfo" yaml:"addRequestInfo" json:"addRequestInfo"`
	SlowRequestThreshold   config.TimeDuration `mapstructure:"slowRequestThreshold" yaml:"slowRequestThreshold" json:"slowRequestThreshold"`
}

// Set sets log server configuration values from config.DataProvider.
func (l *LogConfig) Set(dp config.DataProvider) error {
	var err error

	if l.RequestStart, err = dp.GetBool(cfgKeyServerLogRequestStart); err != nil {
		return err
	}
	if l.RequestHeaders, err = dp.GetStringSlice(cfgKeyServerLogRequestHeaders); err != nil {
		return err
	}
	if l.ExcludedEndpoints, err = dp.GetStringSlice(cfgKeyServerLogExcludedEndpoints); err != nil {
		return err
	}
	if l.SecretQueryParams, err = dp.GetStringSlice(cfgKeyServerLogSecretQueryParams); err != nil {
		return err
	}
	if l.AddRequestInfoToLogger, err = dp.GetBool(cfgKeyServerLogAddRequestInfo); err != nil {
		return err
	}

	var dur time.Duration
	if dur, err = dp.GetDuration(cfgKeyServerLogSlowRequestThreshold); err != nil {
		return err
	}
	l.SlowRequestThreshold = config.TimeDuration(dur)

	return nil
}

// TLSConfig contains configuration parameters needed to initialize(or not) secure server
type TLSConfig struct {
	Enabled     bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Certificate string `mapstructure:"cert" yaml:"cert" json:"cert"`
	Key         string `mapstructure:"key" yaml:"key" json:"key"`
}

// Set sets security server configuration values from config.DataProvider.
func (s *TLSConfig) Set(dp config.DataProvider) error {
	var err error

	if s.Enabled, err = dp.GetBool(cfgKeyServerTLSEnabled); err != nil {
		return err
	}

	if s.Certificate, err = dp.GetString(cfgKeyServerTLSCert); err != nil {
		return err
	}

	if s.Key, err = dp.GetString(cfgKeyServerTLSKey); err != nil {
		return err
	}

	if s.Enabled && (s.Certificate == "" || s.Key == "") {
		return dp.WrapKeyErr(cfgKeyServerTLSKey, fmt.Errorf("both cert and key should be set"))
	}

	return nil
}

// Set sets HTTPServer configuration values from config.DataProvider.
func (c *Config) Set(dp config.DataProvider) error {
	var err error

	if c.Address, err = dp.GetString(cfgKeyServerAddress); err != nil {
		return err
	}
	if c.UnixSocketPath, err = dp.GetString(cfgKeyServerUnixSocketPath); err != nil {
		return err
	}
	if c.Address == "" && c.UnixSocketPath == "" {
		return dp.WrapKeyErr(cfgKeyServerAddress, fmt.Errorf("either address or unixSocketPath should be set"))
	}

	err = c.TLS.Set(dp)
	if err != nil {
		return err
	}

	err = c.Timeouts.Set(dp)
	if err != nil {
		return err
	}

	err = c.Limits.Set(dp)
	if err != nil {
		return err
	}

	err = c.Log.Set(dp)
	if err != nil {
		return err
	}

	return nil
}

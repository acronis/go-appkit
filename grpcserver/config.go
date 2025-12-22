/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package grpcserver

import (
	"fmt"
	"time"

	"github.com/acronis/go-appkit/config"
)

const cfgDefaultKeyPrefix = "grpcServer"

const (
	cfgKeyServerAddress               = "address"
	cfgKeyServerUnixSocketPath        = "unixSocketPath"
	cfgKeyServerTLSCert               = "tls.cert"
	cfgKeyServerTLSKey                = "tls.key"
	cfgKeyServerTLSEnabled            = "tls.enabled"
	cfgKeyServerShutdownTimeout       = "timeouts.shutdown"
	cfgKeyServerKeepaliveTime         = "keepalive.time"
	cfgKeyServerKeepaliveTimeout      = "keepalive.timeout"
	cfgKeyServerKeepaliveMinTime      = "keepalive.minTime"
	cfgKeyServerMaxConcurrentStreams  = "limits.maxConcurrentStreams"
	cfgKeyServerMaxRecvMessageSize    = "limits.maxRecvMessageSize"
	cfgKeyServerMaxSendMessageSize    = "limits.maxSendMessageSize"
	cfgKeyServerLogCallStart          = "log.callStart"
	cfgKeyServerLogExcludedMethods    = "log.excludedMethods"
	cfgKeyServerLogSlowCallThreshold  = "log.slowCallThreshold"
	cfgKeyServerLogTimeSlotsThreshold = "log.timeSlotsThreshold"
)

const (
	defaultServerAddress            = ":9090"
	defaultServerShutdownTimeout    = time.Second * 5
	defaultServerKeepaliveTime      = time.Minute * 2
	defaultServerKeepaliveTimeout   = time.Second * 20
	defaultServerMaxRecvMessageSize = 1024 * 1024 * 4 // 4MB
	defaultServerMaxSendMessageSize = 1024 * 1024 * 4 // 4MB
	defaultSlowCallThreshold        = time.Second
)

// Config represents a set of configuration parameters for gRPC Server.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	Address        string          `mapstructure:"address" yaml:"address" json:"address"`
	UnixSocketPath string          `mapstructure:"unixSocketPath" yaml:"unixSocketPath" json:"unixSocketPath"`
	Timeouts       TimeoutsConfig  `mapstructure:"timeouts" yaml:"timeouts" json:"timeouts"`
	Keepalive      KeepaliveConfig `mapstructure:"keepalive" yaml:"keepalive" json:"keepalive"`
	Limits         LimitsConfig    `mapstructure:"limits" yaml:"limits" json:"limits"`
	Log            LogConfig       `mapstructure:"log" yaml:"log" json:"log"`
	TLS            TLSConfig       `mapstructure:"tls" yaml:"tls" json:"tls"`

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
	opts := configOptions{keyPrefix: cfgDefaultKeyPrefix}
	for _, opt := range options {
		opt(&opts)
	}
	return &Config{keyPrefix: opts.keyPrefix}
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
			Shutdown: config.TimeDuration(defaultServerShutdownTimeout),
		},
		Keepalive: KeepaliveConfig{
			Time:    config.TimeDuration(defaultServerKeepaliveTime),
			Timeout: config.TimeDuration(defaultServerKeepaliveTimeout),
		},
		Limits: LimitsConfig{
			MaxRecvMessageSize: config.ByteSize(defaultServerMaxRecvMessageSize),
			MaxSendMessageSize: config.ByteSize(defaultServerMaxSendMessageSize),
		},
		Log: LogConfig{
			SlowCallThreshold: config.TimeDuration(defaultSlowCallThreshold),
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

// SetProviderDefaults sets default configuration values for gRPC Server in config.DataProvider.
// Implements config.Config interface.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyServerAddress, defaultServerAddress)
	dp.SetDefault(cfgKeyServerShutdownTimeout, defaultServerShutdownTimeout)
	dp.SetDefault(cfgKeyServerKeepaliveTime, defaultServerKeepaliveTime)
	dp.SetDefault(cfgKeyServerKeepaliveTimeout, defaultServerKeepaliveTimeout)
	dp.SetDefault(cfgKeyServerMaxRecvMessageSize, defaultServerMaxRecvMessageSize)
	dp.SetDefault(cfgKeyServerMaxSendMessageSize, defaultServerMaxSendMessageSize)
	dp.SetDefault(cfgKeyServerLogCallStart, false)
	dp.SetDefault(cfgKeyServerLogSlowCallThreshold, defaultSlowCallThreshold)
}

// TimeoutsConfig represents a set of configuration parameters for gRPC Server relating to timeouts.
type TimeoutsConfig struct {
	Shutdown config.TimeDuration `mapstructure:"shutdown" yaml:"shutdown" json:"shutdown"`
}

// Set sets timeout server configuration values from config.DataProvider.
// Implements config.Config interface.
func (t *TimeoutsConfig) Set(dp config.DataProvider) error {
	var err error
	var dur time.Duration

	if dur, err = dp.GetDuration(cfgKeyServerShutdownTimeout); err != nil {
		return err
	}
	t.Shutdown = config.TimeDuration(dur)

	return nil
}

// KeepaliveConfig represents a set of configuration parameters for gRPC Server relating to keepalive.
type KeepaliveConfig struct {
	Time    config.TimeDuration `mapstructure:"time" yaml:"time" json:"time"`
	Timeout config.TimeDuration `mapstructure:"timeout" yaml:"timeout" json:"timeout"`
	MinTime config.TimeDuration `mapstructure:"minTime" yaml:"minTime" json:"minTime"`
}

// Set sets keepalive server configuration values from config.DataProvider.
// Implements config.Config interface.
func (k *KeepaliveConfig) Set(dp config.DataProvider) error {
	var err error
	var dur time.Duration

	if dur, err = dp.GetDuration(cfgKeyServerKeepaliveTime); err != nil {
		return err
	}
	k.Time = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerKeepaliveTimeout); err != nil {
		return err
	}
	k.Timeout = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerKeepaliveMinTime); err != nil {
		return err
	}
	k.MinTime = config.TimeDuration(dur)

	return nil
}

// LimitsConfig represents a set of configuration parameters for gRPC Server relating to limits.
type LimitsConfig struct {
	// MaxConcurrentStreams is the maximum number of concurrent streams per connection.
	MaxConcurrentStreams uint32 `mapstructure:"maxConcurrentStreams" yaml:"maxConcurrentStreams" json:"maxConcurrentStreams"`

	// MaxRecvMessageSize is the maximum size of a received message in bytes.
	MaxRecvMessageSize config.ByteSize `mapstructure:"maxRecvMessageSize" yaml:"maxRecvMessageSize" json:"maxRecvMessageSize"`

	// MaxSendMessageSize is the maximum size of a sent message in bytes.
	MaxSendMessageSize config.ByteSize `mapstructure:"maxSendMessageSize" yaml:"maxSendMessageSize" json:"maxSendMessageSize"`
}

// Set sets limit server configuration values from config.DataProvider.
func (l *LimitsConfig) Set(dp config.DataProvider) error {
	var err error

	var maxConcurrentStreams int
	if maxConcurrentStreams, err = dp.GetInt(cfgKeyServerMaxConcurrentStreams); err != nil {
		// MaxConcurrentStreams is optional, so we only return error if it's not a missing key
		if dp.IsSet(cfgKeyServerMaxConcurrentStreams) {
			return err
		}
		maxConcurrentStreams = 0 // default value
	}
	if maxConcurrentStreams < 0 {
		return dp.WrapKeyErr(cfgKeyServerMaxConcurrentStreams, fmt.Errorf("cannot be negative"))
	}
	l.MaxConcurrentStreams = uint32(maxConcurrentStreams) //nolint:gosec // validated non-negative above

	if l.MaxRecvMessageSize, err = dp.GetSizeInBytes(cfgKeyServerMaxRecvMessageSize); err != nil {
		return dp.WrapKeyErr(cfgKeyServerMaxRecvMessageSize, err)
	}

	if l.MaxSendMessageSize, err = dp.GetSizeInBytes(cfgKeyServerMaxSendMessageSize); err != nil {
		return dp.WrapKeyErr(cfgKeyServerMaxSendMessageSize, err)
	}

	return nil
}

// LogConfig represents a set of configuration parameters for gRPC Server relating to logging.
type LogConfig struct {
	CallStart          bool                `mapstructure:"callStart" yaml:"callStart" json:"callStart"`
	ExcludedMethods    []string            `mapstructure:"excludedMethods" yaml:"excludedMethods" json:"excludedMethods"`
	SlowCallThreshold  config.TimeDuration `mapstructure:"slowCallThreshold" yaml:"slowCallThreshold" json:"slowCallThreshold"`
	TimeSlotsThreshold config.TimeDuration `mapstructure:"timeSlotsThreshold" yaml:"timeSlotsThreshold" json:"timeSlotsThreshold"`
}

// Set sets log server configuration values from config.DataProvider.
func (l *LogConfig) Set(dp config.DataProvider) error {
	var err error

	if l.CallStart, err = dp.GetBool(cfgKeyServerLogCallStart); err != nil {
		return err
	}
	if l.ExcludedMethods, err = dp.GetStringSlice(cfgKeyServerLogExcludedMethods); err != nil {
		return err
	}

	var dur time.Duration
	if dur, err = dp.GetDuration(cfgKeyServerLogSlowCallThreshold); err != nil {
		return err
	}
	l.SlowCallThreshold = config.TimeDuration(dur)

	if dur, err = dp.GetDuration(cfgKeyServerLogTimeSlotsThreshold); err != nil {
		return err
	}
	l.TimeSlotsThreshold = config.TimeDuration(dur)

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

	return nil
}

// Set sets gRPC Server configuration values from config.DataProvider.
func (c *Config) Set(dp config.DataProvider) error {
	var err error

	if c.Address, err = dp.GetString(cfgKeyServerAddress); err != nil {
		return err
	}
	if c.UnixSocketPath, err = dp.GetString(cfgKeyServerUnixSocketPath); err != nil {
		return err
	}

	err = c.TLS.Set(dp)
	if err != nil {
		return err
	}

	err = c.Timeouts.Set(dp)
	if err != nil {
		return err
	}

	err = c.Keepalive.Set(dp)
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

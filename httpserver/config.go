/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"time"

	"github.com/acronis/go-appkit/config"
)

const (
	cfgKeyServerAddress                 = "server.address"
	cfgKeyServerUnixSocketPath          = "server.unixSocketPath"
	cfgKeyServerTLSCert                 = "server.tls.cert"
	cfgKeyServerTLSKey                  = "server.tls.key"
	cfgKeyServerTLSEnabled              = "server.tls.enabled"
	cfgKeyServerTimeoutsWrite           = "server.timeouts.write"
	cfgKeyServerTimeoutsRead            = "server.timeouts.read"
	cfgKeyServerTimeoutsReadHeader      = "server.timeouts.readHeader"
	cfgKeyServerTimeoutsIdle            = "server.timeouts.idle"
	cfgKeyServerTimeoutsShutdown        = "server.timeouts.shutdown"
	cfgKeyServerLimitsMaxRequests       = "server.limits.maxRequests"
	cfgKeyServerLimitsMaxBodySize       = "server.limits.maxBodySize"
	cfgKeyServerLogRequestStart         = "server.log.requestStart"
	cfgKeyServerLogRequestHeaders       = "server.log.requestHeaders"
	cfgKeyServerLogExcludedEndpoints    = "server.log.excludedEndpoints"
	cfgKeyServerLogSecretQueryParams    = "server.log.secretQueryParams" // nolint:gosec // false positive
	cfgKeyServerLogAddRequestInfo       = "server.log.addRequestInfo"
	cfgKeyServerLogSlowRequestThreshold = "server.log.slowRequestThreshold"
)

const (
	defaultServerAddress            = ":8080"
	defaultServerTimeoutsWrite      = "1m"
	defaultServerTimeoutsRead       = "15s"
	defaultServerTimeoutsReadHeader = "10s"
	defaultServerTimeoutsIdle       = "1m"
	defaultServerTimeoutsShutdown   = "5s"
	defaultServerLimitsMaxRequests  = 5000
	defaultSlowRequestThreshold     = "1s"
)

// TimeoutsConfig represents a set of configuration parameters for HTTPServer relating to timeouts.
type TimeoutsConfig struct {
	Write      time.Duration
	Read       time.Duration
	ReadHeader time.Duration
	Idle       time.Duration
	Shutdown   time.Duration
}

// Set sets timeout server configuration values from config.DataProvider.
func (t *TimeoutsConfig) Set(dp config.DataProvider) error {
	var err error

	if t.Write, err = dp.GetDuration(cfgKeyServerTimeoutsWrite); err != nil {
		return err
	}
	if t.Read, err = dp.GetDuration(cfgKeyServerTimeoutsRead); err != nil {
		return err
	}
	if t.ReadHeader, err = dp.GetDuration(cfgKeyServerTimeoutsReadHeader); err != nil {
		return err
	}
	if t.Idle, err = dp.GetDuration(cfgKeyServerTimeoutsIdle); err != nil {
		return err
	}
	if t.Shutdown, err = dp.GetDuration(cfgKeyServerTimeoutsShutdown); err != nil {
		return err
	}

	return nil
}

// LimitsConfig represents a set of configuration parameters for HTTPServer relating to limits.
type LimitsConfig struct {
	MaxRequests      int
	MaxBodySizeBytes uint64
}

// Set sets limit server configuration values from config.DataProvider.
func (l *LimitsConfig) Set(dp config.DataProvider) error {
	var err error

	if l.MaxRequests, err = dp.GetInt(cfgKeyServerLimitsMaxRequests); err != nil {
		return err
	}
	if l.MaxBodySizeBytes, err = dp.GetSizeInBytes(cfgKeyServerLimitsMaxBodySize); err != nil {
		return err
	}

	return nil
}

// LogConfig represents a set of configuration parameters for HTTPServer relating to logging.
type LogConfig struct {
	RequestStart           bool
	RequestHeaders         []string
	ExcludedEndpoints      []string
	SecretQueryParams      []string
	AddRequestInfoToLogger bool
	SlowRequestThreshold   time.Duration
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
	if l.SlowRequestThreshold, err = dp.GetDuration(cfgKeyServerLogSlowRequestThreshold); err != nil {
		return err
	}

	return nil
}

// TLSConfig contains configuration parameters needed to initialize(or not) secure server
type TLSConfig struct {
	Enabled     bool
	Certificate string
	Key         string
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

// Config represents a set of configuration parameters for HTTPServer.
type Config struct {
	Address        string
	UnixSocketPath string
	Timeouts       TimeoutsConfig
	Limits         LimitsConfig
	Log            LogConfig
	TLS            TLSConfig

	keyPrefix string
}

var _ config.Config = (*Config)(nil)
var _ config.KeyPrefixProvider = (*Config)(nil)

// NewConfig creates a new instance of the Config.
func NewConfig() *Config {
	return NewConfigWithKeyPrefix("")
}

// NewConfigWithKeyPrefix creates a new instance of the Config.
// Allows to specify key prefix which will be used for parsing configuration parameters.
func NewConfigWithKeyPrefix(keyPrefix string) *Config {
	return &Config{keyPrefix: keyPrefix}
}

// KeyPrefix returns a key prefix with which all configuration parameters should be presented.
func (c *Config) KeyPrefix() string {
	return c.keyPrefix
}

// SetProviderDefaults sets default configuration values for HTTPServer in config.DataProvider.
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

// Set sets HTTPServer configuration values from config.DataProvider.
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

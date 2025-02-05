/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package profserver

import (
	"fmt"

	"github.com/acronis/go-appkit/config"
)

const cfgDefaultKeyPrefix = "profServer"

const (
	cfgKeyProfServerEnabled = "enabled"
	cfgKeyProfServerAddress = "address"
)

const defaultServerAddress = "127.0.0.1:8081"

// Config represents a set of configuration parameters for profiling server.
// Configuration can be loaded in different formats (YAML, JSON) using config.Loader, viper,
// or with json.Unmarshal/yaml.Unmarshal functions directly.
type Config struct {
	Enabled bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`
	Address string `mapstructure:"address" yaml:"address" json:"address"`

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
		Enabled:   true,
		Address:   defaultServerAddress,
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

// SetProviderDefaults sets default configuration values for profiling server in config.DataProvider.
// Implements config.Config interface.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyProfServerEnabled, true)
	dp.SetDefault(cfgKeyProfServerAddress, defaultServerAddress)
}

// Set sets profiling server configuration values from config.DataProvider.
// Implements config.Config interface.
func (c *Config) Set(dp config.DataProvider) error {
	var err error
	if c.Enabled, err = dp.GetBool(cfgKeyProfServerEnabled); err != nil {
		return err
	}
	if c.Address, err = dp.GetString(cfgKeyProfServerAddress); err != nil {
		return err
	}
	if c.Enabled && c.Address == "" {
		return dp.WrapKeyErr(cfgKeyProfServerAddress, fmt.Errorf("cannot be empty"))
	}
	return nil
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package profserver

import "github.com/acronis/go-appkit/config"

const (
	cfgKeyProfServerEnabled = "profserver.enabled"
	cfgKeyProfServerAddress = "profserver.address"
)

// Config represents a set of configuration parameters for profiling server.
type Config struct {
	Enabled bool
	Address string

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

// SetProviderDefaults sets default configuration values for profiling server in config.DataProvider.
func (c *Config) SetProviderDefaults(dp config.DataProvider) {
	dp.SetDefault(cfgKeyProfServerEnabled, true)
	dp.SetDefault(cfgKeyProfServerAddress, ":8081")
}

// Set sets profiling server configuration values from config.DataProvider.
func (c *Config) Set(dp config.DataProvider) error {
	var err error
	if c.Enabled, err = dp.GetBool(cfgKeyProfServerEnabled); err != nil {
		return err
	}
	if c.Address, err = dp.GetString(cfgKeyProfServerAddress); err != nil {
		return err
	}
	return nil
}

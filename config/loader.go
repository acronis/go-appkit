/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"io"
)

// Loader loads configuration values from data provider (with initializing default values before)
// and sets them in configuration objects.
type Loader struct {
	DataProvider DataProvider
}

// NewDefaultLoader creates a new configurations loader with an ability to read values from the environment variables.
func NewDefaultLoader(envVarsPrefix string) *Loader {
	va := NewViperAdapter()
	va.UseEnvVars(envVarsPrefix)
	return NewLoader(va)
}

// NewLoader creates a new configurations' loader.
func NewLoader(dp DataProvider) *Loader {
	return &Loader{dp}
}

// LoadFromFile loads configuration values from file and sets them in configuration objects.
func (l *Loader) LoadFromFile(path string, dataType DataType, cfg Config, cfgs ...Config) error {
	if err := l.DataProvider.SetFromFile(path, dataType); err != nil {
		return err
	}
	return l.load(append([]Config{cfg}, cfgs...))
}

// LoadFromReader loads configuration values from reader and sets them in configuration objects.
func (l *Loader) LoadFromReader(reader io.Reader, dataType DataType, cfg Config, cfgs ...Config) error {
	if err := l.DataProvider.SetFromReader(reader, dataType); err != nil {
		return err
	}
	return l.load(append([]Config{cfg}, cfgs...))
}

func (l *Loader) load(cfgs []Config) error {
	dpForCfg := func(cfg Config) DataProvider {
		if kpHolder, ok := cfg.(KeyPrefixProvider); ok && kpHolder.KeyPrefix() != "" {
			return NewKeyPrefixedDataProvider(l.DataProvider, kpHolder.KeyPrefix())
		}
		return l.DataProvider
	}
	for _, cfg := range cfgs {
		cfg.SetProviderDefaults(dpForCfg(cfg))
	}
	for _, cfg := range cfgs {
		if err := cfg.Set(dpForCfg(cfg)); err != nil {
			return err
		}
	}
	return nil
}

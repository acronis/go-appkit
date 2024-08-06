/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

type testAppConfig struct {
	Server struct {
		Address string
	}
}

func (c *testAppConfig) SetProviderDefaults(dp DataProvider) {
	dp.SetDefault("server.addr", ":80")
}

func (c *testAppConfig) Set(dp DataProvider) error {
	var err error
	c.Server.Address, err = dp.GetString("server.addr")
	return err
}

type testPersonConfig struct {
	Name string
}

func (c *testPersonConfig) KeyPrefix() string {
	return "person"
}

func (c *testPersonConfig) SetProviderDefaults(_ DataProvider) {}

func (c *testPersonConfig) Set(dp DataProvider) error {
	var err error
	c.Name, err = dp.GetString("name")
	return err
}

func TestLoader_LoadFromReader(t *testing.T) {
	cfgLoader := NewLoader(NewViperAdapter())

	t.Run("load config, use defaults", func(t *testing.T) {
		appCfg := &testAppConfig{}
		err := cfgLoader.LoadFromReader(bytes.NewBufferString(`{}`), DataTypeJSON, appCfg)
		require.NoError(t, err)
		require.Equal(t, ":80", appCfg.Server.Address)
	})

	t.Run("load config", func(t *testing.T) {
		appCfg := &testAppConfig{}
		err := cfgLoader.LoadFromReader(bytes.NewBufferString(`{"server":{"addr":":777"}}`), DataTypeJSON, appCfg)
		require.NoError(t, err)
		require.Equal(t, ":777", appCfg.Server.Address)
	})

	t.Run("load config, use key prefix", func(t *testing.T) {
		personCfg := &testPersonConfig{}
		err := cfgLoader.LoadFromReader(bytes.NewBufferString(testPersonConfigJSON), DataTypeJSON, personCfg)
		require.NoError(t, err)
		require.Equal(t, "Steve", personCfg.Name)
	})
}

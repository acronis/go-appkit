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

const testPersonConfigYAML = `
person:
  name: Steve
  age: 30
  preferences:
    language: go
    sport: football
`

const testPersonConfigJSON = `{"person": {"name":"Steve","age":30,"preferences":{"language": "go", "sport":"football"}}}`

type testInternalConfig struct {
	FieldStr string
	FieldInt int

	keyPrefix string
}

func (c *testInternalConfig) KeyPrefix() string {
	return c.keyPrefix
}

func (c *testInternalConfig) SetProviderDefaults(dp DataProvider) {
	p := ""
	if c.keyPrefix != "" {
		p = c.keyPrefix + "_"
	}
	dp.SetDefault("str", p+"default")
	dp.SetDefault("int", 146)
}

func (c *testInternalConfig) Set(dp DataProvider) (err error) {
	if c.FieldStr, err = dp.GetString("str"); err != nil {
		return err
	}
	if c.FieldInt, err = dp.GetInt("int"); err != nil {
		return err
	}
	return nil
}

type testConfig struct {
	InternalCfg1    *testInternalConfig
	InternalCfg2    *testInternalConfig
	InternalCfg3    *testInternalConfig
	NilInternalCfg4 *testInternalConfig
	NilCfg          Config
	FieldBool       bool
}

func (c *testConfig) SetProviderDefaults(dp DataProvider) {
	CallSetProviderDefaultsForFields(c, dp)
}

func (c *testConfig) Set(dp DataProvider) (err error) {
	if err = CallSetForFields(c, dp); err != nil {
		return
	}
	if c.FieldBool, err = dp.GetBool("bool"); err != nil {
		return
	}
	return nil
}

const testConfigYAML = `
bool: true
str: "some string"
int: 42
config2:
  str: "yet another string"
  int: 73
`

func TestCallHelpers(t *testing.T) {
	cfg := &testConfig{
		InternalCfg1: &testInternalConfig{},
		InternalCfg2: &testInternalConfig{keyPrefix: "config2"},
		InternalCfg3: &testInternalConfig{keyPrefix: "config3"},
	}
	l := NewDefaultLoader("")
	err := l.LoadFromReader(bytes.NewReader([]byte(testConfigYAML)), DataTypeYAML, cfg)
	require.NoError(t, err)
	require.Nil(t, cfg.NilInternalCfg4)
	require.Nil(t, cfg.NilCfg)
	require.Equal(t, true, cfg.FieldBool)
	require.Equal(t, "some string", cfg.InternalCfg1.FieldStr)
	require.Equal(t, 42, cfg.InternalCfg1.FieldInt)
	require.Equal(t, "yet another string", cfg.InternalCfg2.FieldStr)
	require.Equal(t, 73, cfg.InternalCfg2.FieldInt)
	require.Equal(t, "config3_default", cfg.InternalCfg3.FieldStr)
	require.Equal(t, 146, cfg.InternalCfg3.FieldInt)
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
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

type nestedConfigurationRoot struct {
	SubConfig1 *nestedConfigurationLevel1
	SubConfig2 *nestedConfigurationLevel1
	SubConfig3 *nestedConfigurationLevel1

	keyPrefix string
}

func newNestedConfiguration() *nestedConfigurationRoot {
	return &nestedConfigurationRoot{
		SubConfig1: newConfigurationNode("subConfig1"),
		SubConfig2: newConfigurationNode("subConfig2"),
		SubConfig3: newConfigurationNode("subConfig3"),
		keyPrefix:  "",
	}
}

func (c *nestedConfigurationRoot) KeyPrefix() string {
	return c.keyPrefix
}

func (c *nestedConfigurationRoot) SetProviderDefaults(dp DataProvider) {
	CallSetProviderDefaultsForFields(c, dp)
}

func (c *nestedConfigurationRoot) Set(dp DataProvider) error {
	return CallSetForFields(c, dp)
}

type nestedConfigurationLevel1 struct {
	SomeField   int
	LeafConfig1 *leafConfiguration
	LeafConfig2 *leafConfiguration
	LeafConfig3 *leafConfiguration

	keyPrefix string
}

func newConfigurationNode(prefix string) *nestedConfigurationLevel1 {
	return &nestedConfigurationLevel1{
		LeafConfig1: newLeafConfig("leafConfig1"),
		LeafConfig2: newLeafConfig("leafConfig2"),
		LeafConfig3: newLeafConfig("leafConfig3"),
		keyPrefix:   prefix,
	}
}

func (c *nestedConfigurationLevel1) KeyPrefix() string {
	return c.keyPrefix
}

func (c *nestedConfigurationLevel1) SetProviderDefaults(dp DataProvider) {
	dp.SetDefault("someField", 3)
	CallSetProviderDefaultsForFields(c, dp)
}

func (c *nestedConfigurationLevel1) Set(dp DataProvider) error {
	var err error
	if c.SomeField, err = dp.GetInt("someField"); err != nil {
		return err
	}

	return CallSetForFields(c, dp)
}

type leafConfiguration struct {
	Field1 int
	Field2 string

	keyPrefix string
}

func newLeafConfig(prefix string) *leafConfiguration {
	return &leafConfiguration{
		keyPrefix: prefix,
	}
}

func (c *leafConfiguration) KeyPrefix() string {
	return c.keyPrefix
}

func (c *leafConfiguration) SetProviderDefaults(dp DataProvider) {
	dp.SetDefault("field1", 10)
	dp.SetDefault("field2", "default")
}

func (c *leafConfiguration) Set(dp DataProvider) error {
	var err error

	if c.Field1, err = dp.GetInt("field1"); err != nil {
		return err
	}

	if c.Field2, err = dp.GetString("field2"); err != nil {
		return err
	}

	return err
}

func TestConfigurationsCanBeNested(t *testing.T) {
	nestedConfigYAML := `
subConfig1:
  leafConfig1:
    field1: 42
    field2: "hello"
subConfig2:
  leafConfig2:
    field1: 17
    field2: "world"
subConfig3:
  someField: 30
  leafConfig1:
    field1: 42
    field2: "hello"
  leafConfig2:
    field1: 17
    field2: "world"
`

	cfg := newNestedConfiguration()
	err := NewDefaultLoader("").LoadFromReader(bytes.NewReader([]byte(nestedConfigYAML)), DataTypeYAML, cfg)
	require.NoError(t, err)
	assert.Equal(t, 42, cfg.SubConfig1.LeafConfig1.Field1)
	assert.Equal(t, "hello", cfg.SubConfig1.LeafConfig1.Field2)
	assert.Equal(t, 10, cfg.SubConfig1.LeafConfig2.Field1)
	assert.Equal(t, "default", cfg.SubConfig1.LeafConfig2.Field2)
	assert.Equal(t, 3, cfg.SubConfig1.SomeField)

	assert.Equal(t, 10, cfg.SubConfig2.LeafConfig1.Field1)
	assert.Equal(t, "default", cfg.SubConfig2.LeafConfig1.Field2)
	assert.Equal(t, 17, cfg.SubConfig2.LeafConfig2.Field1)
	assert.Equal(t, "world", cfg.SubConfig2.LeafConfig2.Field2)
	assert.Equal(t, 3, cfg.SubConfig1.SomeField)

	assert.Equal(t, 42, cfg.SubConfig3.LeafConfig1.Field1)
	assert.Equal(t, "hello", cfg.SubConfig3.LeafConfig1.Field2)
	assert.Equal(t, 17, cfg.SubConfig3.LeafConfig2.Field1)
	assert.Equal(t, "world", cfg.SubConfig3.LeafConfig2.Field2)
	assert.Equal(t, 30, cfg.SubConfig3.SomeField)
}

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

const testPrefixedPersonConfigYAML = `
myPrefix:
  person:
    name: Steve
    age: 30
    preferences:
      language: go
      sport: football
`

func TestKeyPrefixedDataProvider_GetString(t *testing.T) {
	var dp DataProvider = NewKeyPrefixedDataProvider(NewViperAdapter(), "myPrefix")
	err := dp.SetFromReader(bytes.NewBufferString(testPrefixedPersonConfigYAML), DataTypeYAML)
	require.NoError(t, err)

	name, err := dp.GetString("person.name")
	require.NoError(t, err)
	require.Equal(t, "Steve", name)

	age, err := dp.GetInt("person.age")
	require.NoError(t, err)
	require.Equal(t, 30, age)

	prefSport, err := dp.GetString("person.preferences.sport")
	require.NoError(t, err)
	require.Equal(t, "football", prefSport)

	lang, err := dp.GetString("person.preferences.language")
	require.NoError(t, err)
	require.Equal(t, "go", lang)
}

func TestKeyPrefixedDataProvider_Unmarshal(t *testing.T) {
	type cfg struct {
		Person struct {
			Name        string `mapstructure:"name"`
			Age         int    `mapstructure:"age"`
			Preferences struct {
				Language string `mapstructure:"language"`
				Sport    string `mapstructure:"sport"`
			} `mapstructure:"preferences"`
		} `mapstructure:"person"`
	}

	var dp DataProvider = NewKeyPrefixedDataProvider(NewViperAdapter(), "myPrefix")
	err := dp.SetFromReader(bytes.NewBufferString(testPrefixedPersonConfigYAML), DataTypeYAML)
	require.NoError(t, err)

	c := cfg{}
	err = dp.Unmarshal(&c)
	require.NoError(t, err)

	require.Equal(t, "Steve", c.Person.Name)
	require.Equal(t, 30, c.Person.Age)
	require.Equal(t, "go", c.Person.Preferences.Language)
	require.Equal(t, "football", c.Person.Preferences.Sport)
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestViperAdapter_SetFromReader(t *testing.T) {
	t.Run("yaml", func(t *testing.T) {
		va := NewViperAdapter()
		err := va.SetFromReader(bytes.NewBufferString(testPersonConfigYAML), DataTypeYAML)
		require.NoError(t, err)

		name, err := va.GetString("person.name")
		require.NoError(t, err)
		require.Equal(t, "Steve", name)

		prefSport, err := va.GetString("person.preferences.sport")
		require.NoError(t, err)
		require.Equal(t, "football", prefSport)
	})

	t.Run("json", func(t *testing.T) {
		va := NewViperAdapter()
		err := va.SetFromReader(bytes.NewBufferString(testPersonConfigJSON), DataTypeJSON)
		require.NoError(t, err)

		name, err := va.GetString("person.name")
		require.NoError(t, err)
		require.Equal(t, "Steve", name)

		prefSport, err := va.GetString("person.preferences.sport")
		require.NoError(t, err)
		require.Equal(t, "football", prefSport)
	})
}

func TestViperAdapter_DumpToFile(t *testing.T) {
	tests := []struct {
		DataType   DataType
		ConfigText string
	}{
		{DataType: DataTypeJSON, ConfigText: testPersonConfigJSON},
		{DataType: DataTypeYAML, ConfigText: testPersonConfigYAML},
	}

	for i := range tests {
		test := tests[i]
		t.Run(string(test.DataType), func(t *testing.T) {
			va1 := NewViperAdapter()
			err := va1.SetFromReader(bytes.NewBufferString(test.ConfigText), test.DataType)
			require.NoError(t, err)

			fname := path.Join(os.TempDir(), fmt.Sprintf("config.%s", test.DataType))

			err = va1.SaveToFile(fname, test.DataType)
			require.NoError(t, err)

			va2 := NewViperAdapter()
			err = va2.SetFromFile(fname, test.DataType)
			require.NoError(t, err)

			name, err := va2.GetString("person.name")
			require.NoError(t, err)
			require.Equal(t, "Steve", name)

			prefSport, err := va2.GetString("person.preferences.sport")
			require.NoError(t, err)
			require.Equal(t, "football", prefSport)
		})
	}
}

func TestViperAdapter_UseEnvVars(t *testing.T) {
	require.NoError(t, os.Setenv("TEST_PERSON_NAME", "Bob"))
	require.NoError(t, os.Setenv("TEST_PERSON_PREFERENCES_SPORT", "hokey"))

	va := NewViperAdapter()
	va.UseEnvVars("test")

	err := va.SetFromReader(bytes.NewBufferString(testPersonConfigYAML), DataTypeYAML)
	require.NoError(t, err)

	name, err := va.GetString("person.name")
	require.NoError(t, err)
	require.Equal(t, "Bob", name)

	prefSport, err := va.GetString("person.preferences.sport")
	require.NoError(t, err)
	require.Equal(t, "hokey", prefSport)
}

func TestViperAdapter_GetFloat(t *testing.T) {
	viperAdapter := NewViperAdapter()
	const key = "key"

	tests := []struct {
		configVal   interface{}
		wantError   bool
		wantFloat32 float32
		wantFloat64 float64
	}{
		{"foobar", true, 0, 0},
		{[]int{1, 2}, true, 0, 0},
		{1, false, 1, 1},
		{1.1, false, 1.1, 1.1},
	}
	for _, tt := range tests {
		viperAdapter.Set(key, tt.configVal)

		gotFloat32, err := viperAdapter.GetFloat32(key)
		if tt.wantError {
			require.Error(t, err, "%v is invalid float32, error should be", tt.configVal)
		} else {
			require.NoError(t, err, "%v is valid float32, error should not be", tt.configVal)
		}
		require.Equal(t, tt.wantFloat32, gotFloat32)

		gotFloat64, err := viperAdapter.GetFloat64(key)
		if tt.wantError {
			require.Error(t, err, "%v is invalid float64, error should be", tt.configVal)
		} else {
			require.NoError(t, err, "%v is valid float64, error should not be", tt.configVal)
		}
		require.Equal(t, tt.wantFloat64, gotFloat64)
	}
}

func TestViperAdapter_GetStringFromSet(t *testing.T) {
	viperAdapter := NewViperAdapter()
	const key = "stringfromset.key"
	set := []string{"one", "two", "three"}

	t.Run("attempt to get invalid string", func(t *testing.T) {
		invalidVals := []interface{}{true, []string{"foo", "bar"}}
		for _, invVal := range invalidVals {
			viperAdapter.Set(key, invVal)
			_, err := viperAdapter.GetStringFromSet(key, set, false)
			require.Error(t, err, "%v is invalid string, error should be", invVal)
		}
	})

	t.Run("attempt to get string not from set", func(t *testing.T) {
		var err error

		viperAdapter.Set(key, "four")
		_, err = viperAdapter.GetStringFromSet(key, set, false)
		require.Error(t, err)

		viperAdapter.Set(key, "ONE")
		_, err = viperAdapter.GetStringFromSet(key, set, false)
		require.Error(t, err)
	})

	t.Run("get string from set", func(t *testing.T) {
		var err error
		var got string

		viperAdapter.Set(key, "one")
		got, err = viperAdapter.GetStringFromSet(key, set, false)
		require.NoError(t, err)
		require.Equal(t, "one", got)

		viperAdapter.Set(key, "ONE")
		got, err = viperAdapter.GetStringFromSet(key, set, true)
		require.NoError(t, err)
		require.Equal(t, "ONE", got)
	})
}

func TestViperAdapter_GetSizeInBytes(t *testing.T) {
	viperAdapter := NewViperAdapter()
	const key = "sizeinbytes.key"

	t.Run("attempt to get invalid size in bytes", func(t *testing.T) {
		invalidVals := []interface{}{10, true, "not bytes", []string{"foo", "bar"}, "1s", "1h"}
		for _, invVal := range invalidVals {
			viperAdapter.Set(key, invVal)
			_, err := viperAdapter.GetSizeInBytes(key)
			require.Error(t, err, "%v is invalid size in bytes, error should be", invVal)
		}
	})

	t.Run("get size in bytes", func(t *testing.T) {
		testData := map[string]uint64{
			"1K":  1024,
			"2M":  1024 * 1024 * 2,
			"3G":  1024 * 1024 * 1024 * 3,
			"4Gi": 1024 * 1024 * 1024 * 4, // k8s format.
		}
		for val, want := range testData {
			viperAdapter.Set(key, val)
			got, err := viperAdapter.GetSizeInBytes(key)
			require.NoError(t, err, "there is no error should be")
			require.Equal(t, want, got)
		}
	})
}

func TestViperAdapter_GetDuration(t *testing.T) {
	viperAdapter := NewViperAdapter()
	const key = "duration.key"

	t.Run("attempt to get invalid durations", func(t *testing.T) {
		invalidVals := []interface{}{"", "not duration", "s", "10foo", true, []int{1, 2}}
		for _, invVal := range invalidVals {
			viperAdapter.Set(key, invVal)
			_, err := viperAdapter.GetDuration(key)
			require.Error(t, err, "%v is invalid duration, error should be", invVal)
		}
	})

	t.Run("get durations", func(t *testing.T) {
		testData := map[string]time.Duration{
			"10s":    time.Second * 10,
			"7m":     time.Minute * 7,
			"1h2m3s": time.Hour*1 + time.Minute*2 + time.Second*3,
		}
		for val, want := range testData {
			viperAdapter.Set(key, val)
			got, err := viperAdapter.GetDuration(key)
			require.NoError(t, err, "there is no error should be")
			require.Equal(t, want, got)
		}
	})
}
func TestViperAdapter_GetIntSlice(t *testing.T) {
	viperAdapter := NewViperAdapter()
	const key = "slice.key"

	t.Run("attempt to get invalid slice of integers", func(t *testing.T) {
		invalidVals := []interface{}{"string", 10, true, []string{"foo", "bar"}}
		for _, invVal := range invalidVals {
			viperAdapter.Set(key, invVal)
			_, err := viperAdapter.GetIntSlice(key)
			require.Error(t, err, "%v is invalid slice of integers, error should be", invVal)
		}
	})

	t.Run("get slice of integers", func(t *testing.T) {
		ints := []int{11, 22}
		viperAdapter.Set(key, ints)
		got, err := viperAdapter.GetIntSlice(key)
		require.NoError(t, err, "there is no error should be")
		require.ElementsMatch(t, ints, got)
	})
}

func TestViperAdapter_GetStringSlice(t *testing.T) {
	const key = "slice.key"
	strs := []string{"foo", "bar"}
	viperAdapter := NewViperAdapter()
	viperAdapter.Set(key, strs)
	got, err := viperAdapter.GetStringSlice(key)
	require.NoError(t, err, "there is no error should be")
	require.ElementsMatch(t, strs, got)
}

const (
	cfgKeyDumpPersonName                = "person.name"
	cfgKeyDumpPersonAge                 = "person.age"
	cfgKeyDumpPersonPreferencesLanguage = "person.preferences.language"
	cfgKeyDumpPersonPreferencesSport    = "person.preferences.sport"
)

type configForDumpTest struct {
	Person struct {
		Name        string
		Age         int
		Preferences struct {
			Language string
			Sport    string
		}
	}
}

func (c *configForDumpTest) UpdateProviderValues(dp DataProvider) {
	dp.Set(cfgKeyDumpPersonName, c.Person.Name)
	dp.Set(cfgKeyDumpPersonAge, c.Person.Age)
	dp.Set(cfgKeyDumpPersonPreferencesLanguage, c.Person.Preferences.Language)
	dp.Set(cfgKeyDumpPersonPreferencesSport, c.Person.Preferences.Sport)
}

func (c *configForDumpTest) SetProviderDefaults(dp DataProvider) {}

func (c *configForDumpTest) Set(dp DataProvider) error {
	var err error
	if c.Person.Name, err = dp.GetString(cfgKeyDumpPersonName); err != nil {
		return err
	}
	if c.Person.Age, err = dp.GetInt(cfgKeyDumpPersonAge); err != nil {
		return err
	}
	if c.Person.Preferences.Sport, err = dp.GetString(cfgKeyDumpPersonPreferencesSport); err != nil {
		return err
	}
	if c.Person.Preferences.Language, err = dp.GetString(cfgKeyDumpPersonPreferencesLanguage); err != nil {
		return err
	}
	return nil
}

func TestUpdateAndDumpDataProviderToFile(t *testing.T) {
	tests := []struct {
		DataType   DataType
		ConfigText string
	}{
		{DataType: DataTypeJSON, ConfigText: testPersonConfigJSON},
		{DataType: DataTypeYAML, ConfigText: testPersonConfigYAML},
	}

	for i := range tests {
		test := tests[i]
		t.Run(string(test.DataType), func(t *testing.T) {
			cfgInitial := configForDumpTest{}
			initialLoader := NewLoader(NewViperAdapter())
			err := initialLoader.LoadFromReader(bytes.NewBufferString(test.ConfigText), test.DataType, &cfgInitial)
			require.NoError(t, err)

			cfgChanged := cfgInitial
			cfgChanged.Person.Name = "Loki"
			cfgChanged.Person.Age = 40
			cfgChanged.Person.Preferences.Language = "python"
			cfgChanged.Person.Preferences.Sport = "hockey"
			dataProvider := initialLoader.DataProvider
			UpdateDataProvider(dataProvider, &cfgChanged)

			fname := path.Join(os.TempDir(), fmt.Sprintf("config.%s", test.DataType))
			err = dataProvider.SaveToFile(fname, test.DataType)
			require.NoError(t, err)

			cfgFromDump := configForDumpTest{}
			dumpLoader := NewLoader(NewViperAdapter())

			err = dumpLoader.LoadFromFile(fname, test.DataType, &cfgFromDump)
			require.NoError(t, err)
			require.Equal(t, cfgChanged, cfgFromDump)
			require.Equal(t, "Loki", cfgFromDump.Person.Name)
		})
	}
}

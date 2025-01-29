/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"fmt"
	"io"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

// ViperAdapter is DataProvider implementation that uses viper library under the hood.
type ViperAdapter struct {
	viper *viper.Viper
}

var _ DataProvider = (*ViperAdapter)(nil)

// NewViperAdapter creates a new ViperAdapter.
func NewViperAdapter() *ViperAdapter {
	return &ViperAdapter{viper.New()}
}

// UseEnvVars enables the ability to use environment variables for configuration parameters.
// Prefix defines what environment variables will be looked.
// E.g., if your prefix is "spf", the env registry will look for env
// variables that start with "SPF_".
func (va *ViperAdapter) UseEnvVars(prefix string) {
	va.viper.AutomaticEnv()
	va.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	va.viper.SetEnvPrefix(prefix)
}

// Set sets the value for the key in the override register.
func (va *ViperAdapter) Set(key string, value interface{}) {
	va.viper.Set(key, value)
}

// SetDefault sets the default value for this key.
// Default only used when no value is provided by the user via config or ENV.
func (va *ViperAdapter) SetDefault(key string, value interface{}) {
	va.viper.SetDefault(key, value)
}

// IsSet checks to see if the key has been set in any of the data locations.
// IsSet is case-insensitive for a key.
func (va *ViperAdapter) IsSet(key string) bool {
	return va.viper.IsSet(key)
}

// Get retrieves any value given the key to use.
func (va *ViperAdapter) Get(key string) interface{} {
	return va.viper.Get(key)
}

// SetFromFile specifies that discovering and loading configuration data will be performed from file.
func (va *ViperAdapter) SetFromFile(path string, dataType DataType) error {
	va.viper.SetConfigType(string(dataType))
	va.viper.SetConfigFile(path)
	return va.viper.ReadInConfig()
}

// SetFromReader specifies that discovering and loading configuration data will be performed from reader.
func (va *ViperAdapter) SetFromReader(reader io.Reader, dataType DataType) error {
	va.viper.SetConfigType(string(dataType))
	return va.viper.ReadConfig(reader)
}

// GetInt tries to retrieve the value associated with the key as an integer.
func (va *ViperAdapter) GetInt(key string) (res int, err error) {
	res, err = cast.ToIntE(va.Get(key))
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetIntSlice tries to retrieve the value associated with the key as a slice of integers.
func (va *ViperAdapter) GetIntSlice(key string) (res []int, err error) {
	val := va.Get(key)
	if val == nil {
		return
	}
	res, err = cast.ToIntSliceE(val)
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetFloat32 tries to retrieve the value associated with the key as an float32.
func (va *ViperAdapter) GetFloat32(key string) (res float32, err error) {
	res, err = cast.ToFloat32E(va.Get(key))
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetFloat64 tries to retrieve the value associated with the key as an float64.
func (va *ViperAdapter) GetFloat64(key string) (res float64, err error) {
	res, err = cast.ToFloat64E(va.Get(key))
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetString tries to retrieve the value associated with the key as a string.
func (va *ViperAdapter) GetString(key string) (res string, err error) {
	res, err = cast.ToStringE(va.Get(key))
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetBool tries to retrieve the value associated with the key as a bool.
func (va *ViperAdapter) GetBool(key string) (res bool, err error) {
	res, err = cast.ToBoolE(va.Get(key))
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetStringSlice tries to retrieve the value associated with the key as an slice of strings.
func (va *ViperAdapter) GetStringSlice(key string) (res []string, err error) {
	val := va.Get(key)
	if val == nil {
		return
	}
	res, err = cast.ToStringSliceE(val)
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetSizeInBytes tries to retrieve the value associated with the key as a size in bytes.
func (va *ViperAdapter) GetSizeInBytes(key string) (uint64, error) {
	sizeStr, err := va.GetString(key)
	if err != nil {
		return 0, WrapKeyErrIfNeeded(key, err)
	}
	if sizeStr == "" {
		return 0, nil
	}
	// Handle k8s power-of-two values.
	for _, k8sByteSuffix := range [...]string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei"} {
		if strings.HasSuffix(sizeStr, k8sByteSuffix) {
			sizeStr = sizeStr[:len(sizeStr)-1]
			break
		}
	}
	res, err := bytefmt.ToBytes(sizeStr)
	if err != nil {
		return 0, WrapKeyErrIfNeeded(key, err)
	}
	return res, nil
}

// GetStringFromSet tries to retrieve the value associated with the key as a string from the specified set.
func (va *ViperAdapter) GetStringFromSet(key string, set []string, ignoreCase bool) (string, error) {
	str, err := va.GetString(key)
	if err != nil {
		return "", WrapKeyErrIfNeeded(key, err)
	}
	for _, s := range set {
		if (ignoreCase && strings.EqualFold(str, s)) || str == s {
			return str, nil
		}
	}
	return "", WrapKeyErrIfNeeded(key, fmt.Errorf("unknown value %q, should be one of %v", str, set))
}

// GetDuration tries to retrieve the value associated with the key as a duration.
func (va *ViperAdapter) GetDuration(key string) (res time.Duration, err error) {
	val := va.Get(key)
	if val == nil {
		return
	}
	res, err = cast.ToDurationE(va.Get(key))
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetStringMapString tries to retrieve the value associated with the key as an map where key and value are strings.
func (va *ViperAdapter) GetStringMapString(key string) (res map[string]string, err error) {
	val := va.Get(key)
	if val == nil {
		res = make(map[string]string)
		return
	}
	res, err = cast.ToStringMapStringE(val)
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// GetBytesCount tries to retrieve the value associated with the key as a size in bytes.
func (va *ViperAdapter) GetBytesCount(key string) (BytesCount, error) {
	val := va.Get(key)
	if val == nil {
		return 0, nil
	}
	switch v := val.(type) {
	case string:
		num, err := bytefmt.ToBytes(v)
		if err != nil {
			return 0, fmt.Errorf("invalid bytes format: %s", v)
		}
		return BytesCount(num), nil

	case int, int8, int16, int32, int64: // Handle all signed integers
		num := cast.ToInt64(val)
		if num < 0 {
			return 0, fmt.Errorf("negative value is not allowed: %d", num)
		}
		return BytesCount(num), nil

	case uint, uint8, uint16, uint32, uint64: // Handle all unsigned integers
		return BytesCount(cast.ToUint64(val)), nil

	case float32, float64: // Handle floating-point values (converting them to uint64)
		return BytesCount(uint64(cast.ToFloat64(val))), nil

	case BytesCount:
		return v, nil

	default:
		return 0, fmt.Errorf("unsupported type for BytesCount: %T", val)
	}
}

// Unmarshal unmarshals the config into a Struct.
func (va *ViperAdapter) Unmarshal(rawVal interface{}, opts ...DecoderConfigOption) (err error) {
	options := make([]viper.DecoderConfigOption, len(opts))
	for i, opt := range opts {
		options[i] = viper.DecoderConfigOption(opt)
	}
	err = va.viper.Unmarshal(rawVal, options...)
	return
}

// UnmarshalKey takes a single key and unmarshals it into a Struct.
func (va *ViperAdapter) UnmarshalKey(key string, rawVal interface{}, opts ...DecoderConfigOption) (err error) {
	options := make([]viper.DecoderConfigOption, len(opts))
	for i, opt := range opts {
		options[i] = viper.DecoderConfigOption(opt)
	}
	err = va.viper.UnmarshalKey(key, rawVal, options...)
	err = WrapKeyErrIfNeeded(key, err)
	return
}

// WrapKeyErr wraps error adding information about a key where this error occurs.
func (va *ViperAdapter) WrapKeyErr(key string, err error) error {
	return WrapKeyErr(key, err)
}

// SaveToFile writes config into file according data type.
func (va *ViperAdapter) SaveToFile(path string, dataType DataType) error {
	va.viper.SetConfigType(string(dataType))
	return va.viper.WriteConfigAs(path)
}

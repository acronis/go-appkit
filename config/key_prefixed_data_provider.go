/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"io"
	"strings"
	"time"
)

// KeyPrefixedDataProvider is a DataProvider implementation that uses a specified key prefix for parsing configuration parameters.
type KeyPrefixedDataProvider struct {
	delegate  DataProvider
	keyPrefix string
}

var _ DataProvider = (*KeyPrefixedDataProvider)(nil)

// NewKeyPrefixedDataProvider creates a new KeyPrefixedDataProvider.
func NewKeyPrefixedDataProvider(delegate DataProvider, keyPrefix string) *KeyPrefixedDataProvider {
	return &KeyPrefixedDataProvider{delegate: delegate, keyPrefix: keyPrefix}
}

func (kp *KeyPrefixedDataProvider) makeKey(key string) string {
	return strings.Trim(kp.keyPrefix+"."+key, ".")
}

// UseEnvVars enables the ability to use environment variables for configuration parameters.
// Prefix defines what environment variables will be looked.
// E.g., if your prefix is "spf", the env registry will look for env
// variables that start with "SPF_".
func (kp *KeyPrefixedDataProvider) UseEnvVars(prefix string) {
	kp.delegate.UseEnvVars(prefix)
}

// Set sets the value for the key in the override register.
func (kp *KeyPrefixedDataProvider) Set(key string, value interface{}) {
	kp.delegate.Set(kp.makeKey(key), value)
}

// SetDefault sets the default value for this key.
// Default only used when no value is provided by the user via config or ENV.
func (kp *KeyPrefixedDataProvider) SetDefault(key string, value interface{}) {
	kp.delegate.SetDefault(kp.makeKey(key), value)
}

// IsSet checks to see if the key has been set in any of the data locations.
// IsSet is case-insensitive for a key.
func (kp *KeyPrefixedDataProvider) IsSet(key string) bool {
	return kp.delegate.IsSet(kp.makeKey(key))
}

// Get retrieves any value given the key to use.
func (kp *KeyPrefixedDataProvider) Get(key string) interface{} {
	return kp.delegate.Get(kp.makeKey(key))
}

// SetFromFile specifies that discovering and loading configuration data will be performed from file.
func (kp *KeyPrefixedDataProvider) SetFromFile(path string, dataType DataType) error {
	return kp.delegate.SetFromFile(path, dataType)
}

// SetFromReader specifies that discovering and loading configuration data will be performed from reader.
func (kp *KeyPrefixedDataProvider) SetFromReader(reader io.Reader, dataType DataType) error {
	return kp.delegate.SetFromReader(reader, dataType)
}

// GetInt tries to retrieve the value associated with the key as an integer.
func (kp *KeyPrefixedDataProvider) GetInt(key string) (res int, err error) {
	return kp.delegate.GetInt(kp.makeKey(key))
}

// GetIntSlice tries to retrieve the value associated with the key as a slice of integers.
func (kp *KeyPrefixedDataProvider) GetIntSlice(key string) (res []int, err error) {
	return kp.delegate.GetIntSlice(kp.makeKey(key))
}

// GetFloat32 tries to retrieve the value associated with the key as an float32.
func (kp *KeyPrefixedDataProvider) GetFloat32(key string) (res float32, err error) {
	return kp.delegate.GetFloat32(kp.makeKey(key))
}

// GetFloat64 tries to retrieve the value associated with the key as an float64.
func (kp *KeyPrefixedDataProvider) GetFloat64(key string) (res float64, err error) {
	return kp.delegate.GetFloat64(kp.makeKey(key))
}

// GetString tries to retrieve the value associated with the key as a string.
func (kp *KeyPrefixedDataProvider) GetString(key string) (res string, err error) {
	return kp.delegate.GetString(kp.makeKey(key))
}

// GetBool tries to retrieve the value associated with the key as a bool.
func (kp *KeyPrefixedDataProvider) GetBool(key string) (res bool, err error) {
	return kp.delegate.GetBool(kp.makeKey(key))
}

// GetStringSlice tries to retrieve the value associated with the key as an slice of strings.
func (kp *KeyPrefixedDataProvider) GetStringSlice(key string) (res []string, err error) {
	return kp.delegate.GetStringSlice(kp.makeKey(key))
}

// GetSizeInBytes tries to retrieve the value associated with the key as a size in bytes.
func (kp *KeyPrefixedDataProvider) GetSizeInBytes(key string) (uint64, error) {
	return kp.delegate.GetSizeInBytes(kp.makeKey(key))
}

// GetStringFromSet tries to retrieve the value associated with the key as a string from the specified set.
func (kp *KeyPrefixedDataProvider) GetStringFromSet(key string, set []string, ignoreCase bool) (string, error) {
	return kp.delegate.GetStringFromSet(kp.makeKey(key), set, ignoreCase)
}

// GetDuration tries to retrieve the value associated with the key as a duration.
func (kp *KeyPrefixedDataProvider) GetDuration(key string) (res time.Duration, err error) {
	return kp.delegate.GetDuration(kp.makeKey(key))
}

// GetStringMapString tries to retrieve the value associated with the key as an map where key and value are strings.
func (kp *KeyPrefixedDataProvider) GetStringMapString(key string) (res map[string]string, err error) {
	return kp.delegate.GetStringMapString(kp.makeKey(key))
}

// GetBytesCount tries to retrieve the value associated with the key as a size in bytes.
func (kp *KeyPrefixedDataProvider) GetBytesCount(key string) (BytesCount, error) {
	return kp.delegate.GetBytesCount(kp.makeKey(key))
}

// Unmarshal unmarshals the config into a Struct.
func (kp *KeyPrefixedDataProvider) Unmarshal(rawVal interface{}, opts ...DecoderConfigOption) (err error) {
	return kp.delegate.UnmarshalKey(kp.makeKey(""), rawVal, opts...)
}

// UnmarshalKey takes a single key and unmarshals it into a Struct.
func (kp *KeyPrefixedDataProvider) UnmarshalKey(key string, rawVal interface{}, opts ...DecoderConfigOption) (err error) {
	return kp.delegate.UnmarshalKey(kp.makeKey(key), rawVal, opts...)
}

// WrapKeyErr wraps error adding information about a key where this error occurs.
func (kp *KeyPrefixedDataProvider) WrapKeyErr(key string, err error) error {
	return WrapKeyErr(kp.makeKey(key), err)
}

// SaveToFile writes config into file according data type.
func (kp *KeyPrefixedDataProvider) SaveToFile(path string, dataType DataType) error {
	return kp.delegate.SaveToFile(path, dataType)
}

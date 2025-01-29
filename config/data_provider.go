/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"fmt"
	"io"
	"time"

	"github.com/mitchellh/mapstructure"
)

// DataType is a type of data format in which configuration may be described.
type DataType string

// Supported data formats.
const (
	DataTypeYAML DataType = "yaml"
	DataTypeJSON DataType = "json"
)

// DataProvider is an interface for providing configuration data
// from different sources (files, reader, environment variables).
type DataProvider interface {
	UseEnvVars(prefix string)

	Set(key string, value interface{})
	SetDefault(key string, value interface{})

	SetFromFile(path string, dataType DataType) error
	SetFromReader(reader io.Reader, dataType DataType) error

	SaveToFile(path string, dataType DataType) error

	IsSet(key string) bool

	Get(key string) interface{}
	GetBool(key string) (bool, error)
	GetInt(key string) (int, error)
	GetIntSlice(key string) ([]int, error)
	GetFloat32(key string) (res float32, err error)
	GetFloat64(key string) (res float64, err error)
	GetString(key string) (string, error)
	GetStringFromSet(key string, set []string, ignoreCase bool) (string, error)
	GetStringSlice(key string) ([]string, error)
	GetDuration(key string) (time.Duration, error)
	GetSizeInBytes(key string) (uint64, error)
	GetStringMapString(key string) (map[string]string, error)
	GetBytesCount(key string) (BytesCount, error)

	Unmarshal(rawVal interface{}, opts ...DecoderConfigOption) error
	UnmarshalKey(key string, rawVal interface{}, opts ...DecoderConfigOption) error

	WrapKeyErr(key string, err error) error
}

// A DecoderConfigOption can be passed to UnmarshalKey to configure
// mapstructure.DecoderConfig options
type DecoderConfigOption func(*mapstructure.DecoderConfig)

// WrapKeyErrIfNeeded wraps error adding information about a key where this error occurs.
// If error is nil, it does nothing.
func WrapKeyErrIfNeeded(key string, err error) error {
	if err == nil {
		return nil
	}
	return WrapKeyErr(key, err)
}

// WrapKeyErr wraps error adding information about a key where this error occurs.
func WrapKeyErr(key string, err error) error {
	return fmt.Errorf("%s: %w", key, err)
}

// DataProviderUpdater objects can update data providers using their internal values.
type DataProviderUpdater interface {
	UpdateProviderValues(dp DataProvider)
}

// UpdateDataProvider changes data provider values from config structures.
func UpdateDataProvider(dp DataProvider, obj DataProviderUpdater, objs ...DataProviderUpdater) {
	obj.UpdateProviderValues(dp)
	for _, o := range objs {
		o.UpdateProviderValues(dp)
	}
}

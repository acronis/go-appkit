/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"code.cloudfoundry.org/bytefmt"
	"gopkg.in/yaml.v3"
)

// BytesCount represents a number of bytes that can be parsed from JSON and YAML.
type BytesCount uint64

// UnmarshalJSON allows decoding from both integers and human-readable strings.
// Implements json.Unmarshaler interface.
func (b *BytesCount) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if num, err := strconv.ParseUint(s, 10, 64); err == nil {
		*b = BytesCount(num)
		return nil
	}

	if num, err := bytefmt.ToBytes(s); err == nil {
		*b = BytesCount(num)
		return nil
	}

	return fmt.Errorf("invalid bytes format: %s", s)
}

// UnmarshalYAML allows decoding from YAML.
// Implements yaml.Unmarshaler interface.
func (b *BytesCount) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err == nil {
		if num, err := bytefmt.ToBytes(raw); err == nil {
			*b = BytesCount(num)
			return nil
		}
	}

	var num uint64
	if err := value.Decode(&num); err == nil {
		*b = BytesCount(num)
		return nil
	}

	return fmt.Errorf("invalid bytes format: %v", value)
}

// UnmarshalText allows decoding from text.
// Implements encoding.TextUnmarshaler interface, which is used by mapstructure.TextUnmarshallerHookFunc.
func (b *BytesCount) UnmarshalText(text []byte) error {
	return b.UnmarshalJSON(text)
}

// String returns the human-readable string representation.
// Implements fmt.Stringer interface.
func (b BytesCount) String() string {
	return bytefmt.ByteSize(uint64(b))
}

// MarshalJSON encodes as a human-readable string in JSON.
// Implements json.Marshaler interface.
func (b BytesCount) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// MarshalYAML encodes as a human-readable string in YAML.
// Implements yaml.Marshaler interface.
func (b BytesCount) MarshalYAML() (interface{}, error) {
	return b.String(), nil
}

// MarshalText encodes as a human-readable string in text.
// Implements encoding.TextMarshaler interface.
func (b *BytesCount) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

// TimeDuration represents a time duration that can be parsed from JSON and YAML.
type TimeDuration time.Duration

// UnmarshalJSON allows decoding from both integers (milliseconds) and human-readable strings.
// Implements json.Unmarshaler interface.
func (d *TimeDuration) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if num, err := strconv.ParseInt(s, 10, 64); err == nil {
		if num < 0 {
			return fmt.Errorf("negative value is not allowed: %d", num)
		}
		*d = TimeDuration(time.Duration(num) * time.Millisecond)
		return nil
	}

	if dur, err := time.ParseDuration(s); err == nil {
		*d = TimeDuration(dur)
		return nil
	}

	return fmt.Errorf("invalid duration format: %s", s)
}

// UnmarshalYAML allows decoding from YAML.
// Implements yaml.Unmarshaler interface.
func (d *TimeDuration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err == nil {
		var dur time.Duration
		if dur, err = time.ParseDuration(raw); err == nil {
			*d = TimeDuration(dur)
			return nil
		}
	}

	var num int64
	if err := value.Decode(&num); err == nil {
		if num < 0 {
			return fmt.Errorf("negative value is not allowed: %d", num)
		}
		*d = TimeDuration(time.Duration(num) * time.Millisecond)
		return nil
	}

	return fmt.Errorf("invalid duration format: %v", value)
}

// UnmarshalText allows decoding from text.
// Implements encoding.TextUnmarshaler interface, which is used by mapstructure.TextUnmarshallerHookFunc.
func (d *TimeDuration) UnmarshalText(text []byte) error {
	return d.UnmarshalJSON(text)
}

// String returns the human-readable string representation.
// Implements fmt.Stringer interface.
func (d TimeDuration) String() string {
	return time.Duration(d).String()
}

// MarshalJSON encodes as a human-readable string in JSON.
// Implements json.Marshaler interface.
func (d TimeDuration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// MarshalYAML encodes as a human-readable string in YAML.
// Implements yaml.Marshaler interface.
func (d TimeDuration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// MarshalText encodes as a human-readable string in text.
// Implements encoding.TextMarshaler interface.
func (d TimeDuration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

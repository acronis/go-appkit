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

// ByteSize represents a size in bytes that can be parsed from JSON and YAML.
// This type is intended to be used in configuration structures
// and allows parsing both integers and human-readable strings (e.g. "42GB").
type ByteSize uint64

// UnmarshalJSON allows decoding from both integers and human-readable strings.
// Implements json.Unmarshaler interface.
func (b *ByteSize) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if num, err := strconv.ParseInt(s, 10, 64); err == nil {
		if num < 0 {
			return fmt.Errorf("negative value is not allowed: %d", num)
		}
		*b = ByteSize(num)
		return nil
	}
	bs, err := parseByteSizeFromString(s)
	if err != nil {
		return err
	}
	*b = bs
	return nil
}

// UnmarshalYAML allows decoding from YAML.
// Implements yaml.Unmarshaler interface.
func (b *ByteSize) UnmarshalYAML(value *yaml.Node) error {
	var num uint64
	if err := value.Decode(&num); err == nil {
		*b = ByteSize(num)
		return nil
	}
	var s string
	if err := value.Decode(&s); err == nil {
		bs, parseErr := parseByteSizeFromString(s)
		if parseErr != nil {
			return parseErr
		}
		*b = bs
		return nil
	}
	return fmt.Errorf("invalid byte size format: %v", value)
}

// UnmarshalText allows decoding from text.
// Implements encoding.TextUnmarshaler interface, which is used by mapstructure.TextUnmarshallerHookFunc.
func (b *ByteSize) UnmarshalText(text []byte) error {
	return b.UnmarshalJSON(text)
}

// String returns the human-readable string representation.
// Implements fmt.Stringer interface.
func (b ByteSize) String() string {
	return bytefmt.ByteSize(uint64(b))
}

// MarshalJSON encodes as a human-readable string in JSON.
// Implements json.Marshaler interface.
func (b ByteSize) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// MarshalYAML encodes as a human-readable string in YAML.
// Implements yaml.Marshaler interface.
func (b ByteSize) MarshalYAML() (interface{}, error) {
	return b.String(), nil
}

// MarshalText encodes as a human-readable string in text.
// Implements encoding.TextMarshaler interface.
func (b *ByteSize) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

func parseByteSizeFromString(s string) (ByteSize, error) {
	v := strings.TrimSpace(s)

	// Handle k8s power-of-two values.
	for _, k8sByteSuffix := range [...]string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei"} {
		if strings.HasSuffix(v, k8sByteSuffix) {
			v = v[:len(v)-1]
			break
		}
	}

	num, err := bytefmt.ToBytes(v)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size format (%s): %w", s, err)
	}
	return ByteSize(num), nil
}

// TimeDuration represents a time duration that can be parsed from JSON and YAML.
// This type is intended to be used in configuration structures
// and allows parsing both integers (nanoseconds) and human-readable strings (e.g. "1h30m").
type TimeDuration time.Duration

// UnmarshalJSON allows decoding from JSON and supports both integers (nanoseconds) and human-readable strings.
// Implements json.Unmarshaler interface.
func (d *TimeDuration) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if num, err := strconv.ParseInt(s, 10, 64); err == nil {
		if num < 0 {
			return fmt.Errorf("negative value is not allowed: %d", num)
		}
		*d = TimeDuration(num)
		return nil
	}

	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid time duration format (%s): %w", s, err)
	}
	*d = TimeDuration(dur)
	return nil
}

// UnmarshalYAML allows decoding from YAML and supports both integers (nanoseconds) and human-readable strings.
// Implements yaml.Unmarshaler interface.
func (d *TimeDuration) UnmarshalYAML(value *yaml.Node) error {
	var num int64
	if err := value.Decode(&num); err == nil {
		if num < 0 {
			return fmt.Errorf("negative value is not allowed: %d", num)
		}
		*d = TimeDuration(num)
		return nil
	}
	var raw string
	if err := value.Decode(&raw); err == nil {
		dur, parseErr := time.ParseDuration(raw)
		if parseErr != nil {
			return fmt.Errorf("invalid time duration format (%s): %w", raw, parseErr)
		}
		*d = TimeDuration(dur)
		return nil
	}
	return fmt.Errorf("invalid time duration format: %v", value)
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

/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttleconfig

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

// Rate-limiting algorithms.
const (
	RateLimitAlgLeakyBucket   = "leaky_bucket"
	RateLimitAlgSlidingWindow = "sliding_window"
)

// ZoneKeyType is a type of keys zone.
type ZoneKeyType string

// Zone key types.
const (
	ZoneKeyTypeNoKey      ZoneKeyType = ""
	ZoneKeyTypeIdentity   ZoneKeyType = "identity"
	ZoneKeyTypeHeader     ZoneKeyType = "header"
	ZoneKeyTypeRemoteAddr ZoneKeyType = "remote_addr"
)

// ZoneKeyConfig represents a configuration of zone's key.
type ZoneKeyConfig struct {
	// Type determines type of key that will be used for throttling.
	Type ZoneKeyType `mapstructure:"type" yaml:"type" json:"type"`

	// HeaderName is a name of the request header which value will be used as a key.
	// Matters only when Type is a "header".
	HeaderName string `mapstructure:"headerName" yaml:"headerName" json:"headerName"`

	// NoBypassEmpty specifies whether throttling will be used if the value obtained by the key is empty.
	NoBypassEmpty bool `mapstructure:"noBypassEmpty" yaml:"noBypassEmpty" json:"noBypassEmpty"`
}

// Validate validates zone key configuration.
func (c *ZoneKeyConfig) Validate() error {
	switch c.Type {
	case ZoneKeyTypeNoKey, ZoneKeyTypeIdentity, ZoneKeyTypeRemoteAddr:
	case ZoneKeyTypeHeader:
		if c.HeaderName == "" {
			return fmt.Errorf("header name should be specified for %q key zone type", ZoneKeyTypeHeader)
		}
	default:
		return fmt.Errorf("unknown key zone type %q", c.Type)
	}
	return nil
}

// RuleRateLimit represents rule's rate limiting parameters.
type RuleRateLimit struct {
	Zone string   `mapstructure:"zone" yaml:"zone" json:"zone"`
	Tags TagsList `mapstructure:"tags" yaml:"tags" json:"tags"`
}

// RuleInFlightLimit represents rule's in-flight limiting parameters.
type RuleInFlightLimit struct {
	Zone string   `mapstructure:"zone" yaml:"zone" json:"zone"`
	Tags TagsList `mapstructure:"tags" yaml:"tags" json:"tags"`
}

// RateLimitRetryAfterValue represents structured retry-after value for rate limiting.
type RateLimitRetryAfterValue struct {
	IsAuto   bool
	Duration time.Duration
}

const rateLimitRetryAfterAuto = "auto"

// String returns a string representation of the retry-after value.
// Implements fmt.Stringer interface.
func (ra RateLimitRetryAfterValue) String() string {
	if ra.IsAuto {
		return rateLimitRetryAfterAuto
	}
	return ra.Duration.String()
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (ra *RateLimitRetryAfterValue) UnmarshalText(text []byte) error {
	return ra.unmarshal(string(text))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (ra *RateLimitRetryAfterValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	return ra.unmarshal(text)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (ra *RateLimitRetryAfterValue) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		return err
	}
	return ra.unmarshal(text)
}

func (ra *RateLimitRetryAfterValue) unmarshal(retryAfterVal string) error {
	switch v := retryAfterVal; v {
	case "":
		*ra = RateLimitRetryAfterValue{Duration: 0}
	case rateLimitRetryAfterAuto:
		*ra = RateLimitRetryAfterValue{IsAuto: true}
	default:
		dur, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		*ra = RateLimitRetryAfterValue{Duration: dur}
	}
	return nil
}

// MarshalText implements the encoding.TextMarshaler interface.
func (ra RateLimitRetryAfterValue) MarshalText() ([]byte, error) {
	return []byte(ra.String()), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (ra RateLimitRetryAfterValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(ra.String())
}

// MarshalYAML implements the yaml.Marshaler interface.
func (ra RateLimitRetryAfterValue) MarshalYAML() (interface{}, error) {
	return ra.String(), nil
}

// RateLimitValue represents value for rate limiting.
type RateLimitValue struct {
	Count    int
	Duration time.Duration
}

// String returns a string representation of the rate limit value.
// Implements fmt.Stringer interface.
func (rl RateLimitValue) String() string {
	if rl.Duration == 0 && rl.Count == 0 {
		return ""
	}
	var d string
	switch rl.Duration {
	case time.Second:
		d = "s"
	case time.Minute:
		d = "m"
	case time.Hour:
		d = "h"
	default:
		d = rl.Duration.String()
	}
	return fmt.Sprintf("%d/%s", rl.Count, d)
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (rl *RateLimitValue) UnmarshalText(text []byte) error {
	return rl.unmarshal(string(text))
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (rl *RateLimitValue) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	return rl.unmarshal(text)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (rl *RateLimitValue) UnmarshalYAML(value *yaml.Node) error {
	var text string
	if err := value.Decode(&text); err != nil {
		return err
	}
	return rl.unmarshal(text)
}

func (rl *RateLimitValue) unmarshal(rate string) error {
	if rate == "" {
		*rl = RateLimitValue{}
		return nil
	}
	incorrectFormatErr := fmt.Errorf(
		"incorrect format for rate %q, should be N/(s|m|h), for example 10/s, 100/m, 1000/h", rate)
	parts := strings.SplitN(rate, "/", 2)
	if len(parts) != 2 {
		return incorrectFormatErr
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return incorrectFormatErr
	}
	var dur time.Duration
	switch strings.ToLower(parts[1]) {
	case "s":
		dur = time.Second
	case "m":
		dur = time.Minute
	case "h":
		dur = time.Hour
	default:
		return incorrectFormatErr
	}
	*rl = RateLimitValue{Count: count, Duration: dur}
	return nil
}

// MarshalText implements the encoding.TextMarshaler interface.
func (rl RateLimitValue) MarshalText() ([]byte, error) {
	return []byte(rl.String()), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (rl RateLimitValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(rl.String())
}

// MarshalYAML implements the yaml.Marshaler interface.
func (rl RateLimitValue) MarshalYAML() (interface{}, error) {
	return rl.String(), nil
}

// TagsList represents a list of tags.
type TagsList []string

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (tl *TagsList) UnmarshalText(text []byte) error {
	tl.unmarshal(string(text))
	return nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (tl *TagsList) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		tl.unmarshal(s)
		return nil
	}
	var l []string
	if err := json.Unmarshal(data, &l); err == nil {
		*tl = l
		return nil
	}
	return fmt.Errorf("invalid methods list: %s", data)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (tl *TagsList) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		tl.unmarshal(s)
		return nil
	}
	var l []string
	if err := value.Decode(&l); err == nil {
		*tl = l
		return nil
	}
	return fmt.Errorf("invalid methods list: %v", value)
}

func (tl *TagsList) unmarshal(data string) {
	data = strings.TrimSpace(data)
	if data == "" {
		*tl = TagsList{}
		return
	}
	methods := strings.Split(data, ",")
	for _, m := range methods {
		*tl = append(*tl, strings.TrimSpace(m))
	}
}

func (tl TagsList) String() string {
	return strings.Join(tl, ",")
}

// MarshalText implements the encoding.TextMarshaler interface.
func (tl TagsList) MarshalText() ([]byte, error) {
	return []byte(tl.String()), nil
}

// MarshalJSON implements the json.Marshaler interface.
func (tl TagsList) MarshalJSON() ([]byte, error) {
	return json.Marshal(tl.String())
}

// MarshalYAML implements the yaml.Marshaler interface.
func (tl TagsList) MarshalYAML() (interface{}, error) {
	return tl.String(), nil
}

func mapstructureTrimSpaceStringsHookFunc() mapstructure.DecodeHookFunc {
	return func(
		f reflect.Kind,
		t reflect.Kind,
		data interface{}) (interface{}, error) {
		if f != reflect.Slice || t != reflect.Slice {
			return data, nil
		}
		switch dt := data.(type) {
		case []string:
			res := make([]string, 0, len(dt))
			for _, s := range dt {
				res = append(res, strings.TrimSpace(s))
			}
			return res, nil
		default:
			return data, nil
		}
	}
}

// MapstructureDecodeHook returns a DecodeHookFunc for mapstructure to handle custom types.
func MapstructureDecodeHook() mapstructure.DecodeHookFunc {
	return mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.TextUnmarshallerHookFunc(),
		mapstructureTrimSpaceStringsHookFunc(),
	)
}

// CheckStringSlicesIntersect checks if two string slices have any common elements.
func CheckStringSlicesIntersect(slice1, slice2 []string) bool {
	for i := range slice1 {
		for j := range slice2 {
			if slice1[i] == slice2[j] {
				return true
			}
		}
	}
	return false
}

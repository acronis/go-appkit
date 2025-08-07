package throttleconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestZoneKeyValidation(t *testing.T) {
	t.Run("valid configurations", func(t *testing.T) {
		configs := []ZoneKeyConfig{
			{Type: ZoneKeyTypeNoKey},
			{Type: ZoneKeyTypeIdentity},
			{Type: ZoneKeyTypeRemoteAddr},
			{Type: ZoneKeyTypeHeader, HeaderName: "x-tenant-id"},
		}

		for _, cfg := range configs {
			require.NoError(t, cfg.Validate())
		}
	})

	t.Run("invalid configurations", func(t *testing.T) {
		// Header type without header name
		cfg := ZoneKeyConfig{Type: ZoneKeyTypeHeader}
		err := cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "header name should be specified")

		// Unknown type
		cfg = ZoneKeyConfig{Type: "unknown"}
		err = cfg.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "unknown key zone type")
	})
}

func TestTagsList_UnmarshalText_MarshalText(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected TagsList
		marshalExpected   string
	}{
		{input: "tag1,tag2", unmarshalExpected: TagsList{"tag1", "tag2"}, marshalExpected: "tag1,tag2"},
		{input: "tag3, tag4", unmarshalExpected: TagsList{"tag3", "tag4"}, marshalExpected: "tag3,tag4"},
		{input: "", unmarshalExpected: TagsList{}, marshalExpected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var tl TagsList

			err := tl.UnmarshalText([]byte(tt.input))
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, tl)

			b, err := tl.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestTagsList_UnmarshalJSON_MarshalJSON(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected TagsList
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: `"tag1, tag2"`, unmarshalExpected: TagsList{"tag1", "tag2"}, marshalExpected: `"tag1,tag2"`},
		{input: `["tag3", "tag4"]`, unmarshalExpected: TagsList{"tag3", "tag4"}, marshalExpected: `"tag3,tag4"`},
		{input: `""`, unmarshalExpected: TagsList{}, marshalExpected: `""`},
		{input: `123`, unmarshalExpected: nil, unmarshalErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var tl TagsList

			err := tl.UnmarshalJSON([]byte(tt.input))
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, tl)

			b, err := tl.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestTagsList_UnmarshalYAML_MarshalYAML(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected TagsList
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: `"tag1, tag2"`, unmarshalExpected: TagsList{"tag1", "tag2"}, marshalExpected: "tag1,tag2\n"},
		{input: `["tag3", "tag4"]`, unmarshalExpected: TagsList{"tag3", "tag4"}, marshalExpected: "tag3,tag4\n"},
		{input: `""`, unmarshalExpected: TagsList{}, marshalExpected: "\"\"\n"},
		{input: `[123`, unmarshalExpected: TagsList{}, unmarshalErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var tl TagsList

			err := yaml.Unmarshal([]byte(tt.input), &tl)
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, tl)

			b, err := yaml.Marshal(tl)
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestRateLimitValue_UnmarshalText_MarshalText(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected RateLimitValue
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: "10/s", unmarshalExpected: RateLimitValue{Count: 10, Duration: time.Second}, marshalExpected: "10/s"},
		{input: "100/m", unmarshalExpected: RateLimitValue{Count: 100, Duration: time.Minute}, marshalExpected: "100/m"},
		{input: "1/h", unmarshalExpected: RateLimitValue{Count: 1, Duration: time.Hour}, marshalExpected: "1/h"},
		{input: "", unmarshalExpected: RateLimitValue{}, marshalExpected: ""},
		{input: `123`, unmarshalExpected: RateLimitValue{}, unmarshalErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var rl RateLimitValue

			err := rl.UnmarshalText([]byte(tt.input))
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, rl)

			b, err := rl.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestRateLimitValue_UnmarshalJSON_MarshalJSON(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected RateLimitValue
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: `"10/s"`, unmarshalExpected: RateLimitValue{Count: 10, Duration: time.Second}, marshalExpected: `"10/s"`},
		{input: `"100/m"`, unmarshalExpected: RateLimitValue{Count: 100, Duration: time.Minute}, marshalExpected: `"100/m"`},
		{input: `"1/h"`, unmarshalExpected: RateLimitValue{Count: 1, Duration: time.Hour}, marshalExpected: `"1/h"`},
		{input: `""`, unmarshalExpected: RateLimitValue{}, marshalExpected: `""`},
		{input: `123`, unmarshalExpected: RateLimitValue{}, unmarshalErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var rl RateLimitValue

			err := rl.UnmarshalJSON([]byte(tt.input))
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, rl)

			b, err := rl.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestRateLimitValue_UnmarshalYAML_MarshalYAML(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected RateLimitValue
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: `10/s`, unmarshalExpected: RateLimitValue{Count: 10, Duration: time.Second}, marshalExpected: "10/s\n"},
		{input: `100/m`, unmarshalExpected: RateLimitValue{Count: 100, Duration: time.Minute}, marshalExpected: "100/m\n"},
		{input: `1/h`, unmarshalExpected: RateLimitValue{Count: 1, Duration: time.Hour}, marshalExpected: "1/h\n"},
		{input: "", unmarshalExpected: RateLimitValue{}, marshalExpected: "\"\"\n"},
		{input: `[123`, unmarshalExpected: RateLimitValue{}, unmarshalErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var rl RateLimitValue

			err := yaml.Unmarshal([]byte(tt.input), &rl)
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, rl)

			b, err := yaml.Marshal(rl)
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestRateLimitRetryAfterValue_UnmarshalText_MarshalText(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected RateLimitRetryAfterValue
		marshalExpected   string
	}{
		{input: "15s", unmarshalExpected: RateLimitRetryAfterValue{Duration: 15 * time.Second}, marshalExpected: "15s"},
		{input: "auto", unmarshalExpected: RateLimitRetryAfterValue{IsAuto: true}, marshalExpected: "auto"},
		{input: "", unmarshalExpected: RateLimitRetryAfterValue{}, marshalExpected: "0s"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var ra RateLimitRetryAfterValue

			err := ra.UnmarshalText([]byte(tt.input))
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, ra)

			b, err := ra.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestRateLimitRetryAfterValue_UnmarshalJSON_MarshalJSON(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected RateLimitRetryAfterValue
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: `"15s"`, unmarshalExpected: RateLimitRetryAfterValue{Duration: 15 * time.Second}, marshalExpected: `"15s"`},
		{input: `"auto"`, unmarshalExpected: RateLimitRetryAfterValue{IsAuto: true}, marshalExpected: `"auto"`},
		{input: `""`, unmarshalExpected: RateLimitRetryAfterValue{}, marshalExpected: `"0s"`},
		{input: `123`, unmarshalExpected: RateLimitRetryAfterValue{}, unmarshalErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var ra RateLimitRetryAfterValue

			err := ra.UnmarshalJSON([]byte(tt.input))
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, ra)

			b, err := ra.MarshalJSON()
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

func TestRateLimitRetryAfterValue_UnmarshalYAML_MarshalYAML(t *testing.T) {
	tests := []struct {
		input             string
		unmarshalExpected RateLimitRetryAfterValue
		unmarshalErr      bool
		marshalExpected   string
	}{
		{input: `"15s"`, unmarshalExpected: RateLimitRetryAfterValue{Duration: 15 * time.Second}, marshalExpected: "15s\n"},
		{input: `"auto"`, unmarshalExpected: RateLimitRetryAfterValue{IsAuto: true}, marshalExpected: "auto\n"},
		{input: `""`, unmarshalExpected: RateLimitRetryAfterValue{}, marshalExpected: "0s\n"},
		{input: `[123`, unmarshalExpected: RateLimitRetryAfterValue{}, unmarshalErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var ra RateLimitRetryAfterValue

			err := yaml.Unmarshal([]byte(tt.input), &ra)
			if tt.unmarshalErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.unmarshalExpected, ra)

			b, err := yaml.Marshal(ra)
			require.NoError(t, err)
			require.Equal(t, tt.marshalExpected, string(b))
		})
	}
}

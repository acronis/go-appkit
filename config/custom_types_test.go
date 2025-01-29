/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBytesCount_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    BytesCount
		wantErr bool
	}{
		{"Valid Integer", `1024`, BytesCount(1024), false},
		{"Valid Human-Readable", `"10MB"`, BytesCount(10 * 1024 * 1024), false},
		{"Invalid Format", `"invalid"`, 0, true},
		{"Negative Value", `"-1024"`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b BytesCount
			err := json.Unmarshal([]byte(tt.input), &b)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, b)
			}
		})
	}
}

func TestBytesCount_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    BytesCount
		wantErr bool
	}{
		{"Valid Integer", "size: 2048", BytesCount(2048), false},
		{"Valid Human-Readable", "size: 20MB", BytesCount(20 * 1024 * 1024), false},
		{"Invalid Format", "size: invalid", 0, true},
		{"Negative Value", "size: -1024", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg struct{ Size BytesCount }
			err := yaml.Unmarshal([]byte(tt.input), &cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, cfg.Size)
			}
		})
	}
}

func TestBytesCount_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    BytesCount
		wantErr bool
	}{
		{"Valid Integer", "4096", BytesCount(4096), false},
		{"Valid Human-Readable", "20MB", BytesCount(20 * 1024 * 1024), false},
		{"Invalid Format", "invalid", 0, true},
		{"Negative Value", "-1024", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b BytesCount
			err := b.UnmarshalText([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, b)
			}
		})
	}
}

func TestBytesCount_String(t *testing.T) {
	tests := []struct {
		name  string
		input BytesCount
		want  string
	}{
		{"Bytes", BytesCount(512), "512B"},
		{"Kilobytes", BytesCount(1024), "1K"},
		{"Megabytes", BytesCount(2 * 1024 * 1024), "2M"},
		{"Gigabytes", BytesCount(3 * 1024 * 1024 * 1024), "3G"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.input.String())
		})
	}
}

func TestBytesCount_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input BytesCount
		want  string
	}{
		{"Bytes", BytesCount(256), `"256B"`},
		{"Kilobytes", BytesCount(1024), `"1K"`},
		{"Megabytes", BytesCount(5 * 1024 * 1024), `"5M"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestBytesCount_MarshalYAML(t *testing.T) {
	tests := []struct {
		name  string
		input BytesCount
		want  string
	}{
		{"Bytes", BytesCount(128), "128B\n"},
		{"Kilobytes", BytesCount(1024), "1K\n"},
		{"Megabytes", BytesCount(7 * 1024 * 1024), "7M\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestBytesCount_MarshalText(t *testing.T) {
	tests := []struct {
		name  string
		input BytesCount
		want  string
	}{
		{"Bytes", BytesCount(256), "256B"},
		{"Kilobytes", BytesCount(1024), "1K"},
		{"Megabytes", BytesCount(5 * 1024 * 1024), "5M"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.input.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestTimeDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    TimeDuration
		wantErr bool
	}{
		{"Valid Integer", `1000`, TimeDuration(time.Second), false},
		{"Valid Human-Readable", `"1s"`, TimeDuration(time.Second), false},
		{"Invalid Format", `"invalid"`, 0, true},
		{"Negative Value", `"-1000"`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d TimeDuration
			err := json.Unmarshal([]byte(tt.input), &d)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, d)
			}
		})
	}
}

func TestTimeDuration_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    TimeDuration
		wantErr bool
	}{
		{"Valid Integer", "duration: 2000", TimeDuration(2 * time.Second), false},
		{"Valid Human-Readable", "duration: 2s", TimeDuration(2 * time.Second), false},
		{"Invalid Format", "duration: invalid", 0, true},
		{"Negative Value", "duration: -2000", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg struct{ Duration TimeDuration }
			err := yaml.Unmarshal([]byte(tt.input), &cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, cfg.Duration)
			}
		})
	}
}

func TestTimeDuration_String(t *testing.T) {
	tests := []struct {
		name  string
		input TimeDuration
		want  string
	}{
		{"Milliseconds", TimeDuration(500 * time.Millisecond), "500ms"},
		{"Seconds", TimeDuration(time.Second), "1s"},
		{"Minutes", TimeDuration(time.Minute), "1m0s"},
		{"Hours", TimeDuration(time.Hour), "1h0m0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.input.String())
		})
	}
}

func TestTimeDuration_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input TimeDuration
		want  string
	}{
		{"Milliseconds", TimeDuration(250 * time.Millisecond), `"250ms"`},
		{"Seconds", TimeDuration(time.Second), `"1s"`},
		{"Minutes", TimeDuration(3 * time.Minute), `"3m0s"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestTimeDuration_MarshalYAML(t *testing.T) {
	tests := []struct {
		name  string
		input TimeDuration
		want  string
	}{
		{"Milliseconds", TimeDuration(150 * time.Millisecond), "150ms\n"},
		{"Seconds", TimeDuration(time.Second), "1s\n"},
		{"Minutes", TimeDuration(5 * time.Minute), "5m0s\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestTimeDuration_MarshalText(t *testing.T) {
	tests := []struct {
		name  string
		input TimeDuration
		want  string
	}{
		{"Milliseconds", TimeDuration(150 * time.Millisecond), "150ms"},
		{"Seconds", TimeDuration(time.Second), "1s"},
		{"Minutes", TimeDuration(5 * time.Minute), "5m0s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.input.MarshalText()
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

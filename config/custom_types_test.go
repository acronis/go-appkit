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

func TestByteSize_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ByteSize
		wantErr bool
	}{
		{"Valid Integer", `1024`, ByteSize(1024), false},
		{"Valid Human-Readable, MB", `"10MB"`, ByteSize(10 * 1024 * 1024), false},
		{"Valid Human-Readable, MiB", `"10MiB"`, ByteSize(10 * 1024 * 1024), false},
		{"Valid Human-Readable, k8s format, Mi", `"10Mi"`, ByteSize(10 * 1024 * 1024), false},
		{"Invalid Format", `"invalid"`, 0, true},
		{"Negative Value", `"-1024"`, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b ByteSize
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

func TestByteSize_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ByteSize
		wantErr bool
	}{
		{"Valid Integer", "size: 2048", ByteSize(2048), false},
		{"Valid Human-Readable, MB", "size: 20MB", ByteSize(20 * 1024 * 1024), false},
		{"Valid Human-Readable, MiB", "size: 20MiB", ByteSize(20 * 1024 * 1024), false},
		{"Valid Human-Readable, k8s format, Mi", "size: 20Mi", ByteSize(20 * 1024 * 1024), false},
		{"Invalid Format", "size: invalid", 0, true},
		{"Negative Value", "size: -1024", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg struct{ Size ByteSize }
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

func TestByteSize_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ByteSize
		wantErr bool
	}{
		{"Valid Integer", "4096", ByteSize(4096), false},
		{"Valid Human-Readable, MB", `"10MB"`, ByteSize(10 * 1024 * 1024), false},
		{"Valid Human-Readable, MiB", `"10MiB"`, ByteSize(10 * 1024 * 1024), false},
		{"Valid Human-Readable, k8s format, Mi", `"10Mi"`, ByteSize(10 * 1024 * 1024), false},
		{"Invalid Format", "invalid", 0, true},
		{"Negative Value", "-1024", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b ByteSize
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

func TestByteSize_String(t *testing.T) {
	tests := []struct {
		name  string
		input ByteSize
		want  string
	}{
		{"Bytes", ByteSize(512), "512B"},
		{"Kilobytes", ByteSize(1024), "1K"},
		{"Megabytes", ByteSize(2 * 1024 * 1024), "2M"},
		{"Gigabytes", ByteSize(3 * 1024 * 1024 * 1024), "3G"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.input.String())
		})
	}
}

func TestByteSize_MarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		input ByteSize
		want  string
	}{
		{"Bytes", ByteSize(256), `"256B"`},
		{"Kilobytes", ByteSize(1024), `"1K"`},
		{"Megabytes", ByteSize(5 * 1024 * 1024), `"5M"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestByteSize_MarshalYAML(t *testing.T) {
	tests := []struct {
		name  string
		input ByteSize
		want  string
	}{
		{"Bytes", ByteSize(128), "128B\n"},
		{"Kilobytes", ByteSize(1024), "1K\n"},
		{"Megabytes", ByteSize(7 * 1024 * 1024), "7M\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
		})
	}
}

func TestByteSize_MarshalText(t *testing.T) {
	tests := []struct {
		name  string
		input ByteSize
		want  string
	}{
		{"Bytes", ByteSize(256), "256B"},
		{"Kilobytes", ByteSize(1024), "1K"},
		{"Megabytes", ByteSize(5 * 1024 * 1024), "5M"},
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
		{"Valid Integer", `1000000000`, TimeDuration(time.Second), false},
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
		{"Valid Integer", "duration: 2000000000", TimeDuration(2 * time.Second), false},
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

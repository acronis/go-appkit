/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"testing"

	"github.com/ssgreg/logf"
	"github.com/stretchr/testify/require"
)

func TestLoggerToStd(t *testing.T) {
	oldStdOut := os.Stdout
	oldStdErr := os.Stderr
	defer func() {
		os.Stdout = oldStdOut
		os.Stderr = oldStdErr
	}()

	tests := []struct {
		Output Output
		Level  Level
		Msg    string
		Error  error
	}{
		{
			Output: OutputStdout,
			Level:  LevelInfo,
			Msg:    "test",
		},
		{
			Output: OutputStdout,
			Level:  LevelWarn,
			Msg:    "Hello, world!",
		},
		{
			Output: OutputStdout,
			Level:  LevelError,
			Msg:    "Hello, world!",
			Error:  errors.New("some error"),
		},
		{
			Output: OutputStderr,
			Level:  LevelInfo,
			Msg:    "Hello, world!",
		},
	}

	for i := range tests {
		test := tests[i]

		r, w, _ := os.Pipe()

		if test.Output == OutputStderr {
			os.Stderr = w
		} else {
			os.Stdout = w
		}

		go func() {
			logger, closer := NewLogger(&Config{Output: test.Output, NoColor: true, Format: FormatJSON, Level: LevelInfo, ErrorVerboseSuffix: "err"})
			switch test.Level {
			case LevelInfo:
				logger.Info(test.Msg)
			case LevelWarn:
				logger.Warn(test.Msg)
			case LevelError:
				logger.Error(test.Msg, logf.Error(test.Error))
			}
			closer()
			_ = w.Close()
		}()

		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		require.NoError(t, err, "io.Copy")

		var j map[string]interface{}
		require.NoError(t, json.Unmarshal(buf.Bytes(), &j))

		require.Equal(t, string(test.Level), j["level"])
		require.Equal(t, test.Msg, j["msg"])
		if test.Error != nil {
			require.Equal(t, test.Error.Error(), j["error"])
		}
		require.Equal(t, os.Getpid(), int(j["pid"].(float64)))
	}
}

func TestTextFormat(t *testing.T) {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		_ = w.Close()
		os.Stderr = old // restoring the real stdout
	}()

	go func() {
		logger, closer := NewLogger(&Config{Output: OutputStderr, NoColor: true, Format: FormatText, Level: LevelInfo, ErrorVerboseSuffix: "err"})
		logger.AtLevel(LevelError, func(logFunc LogFunc) {
			logFunc("test", logf.Error(errors.New("some error")))
		})
		closer()
		_ = w.Close()
	}()

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err, "io.Copy")

	require.Contains(t, buf.String(), `|ERRO|`)
	require.Contains(t, buf.String(), ` test `)
	require.Contains(t, buf.String(), `error="some error"`)
	require.Contains(t, buf.String(), fmt.Sprintf(`pid=%d`, os.Getpid()))
}

func TestLoggerWithMasking(t *testing.T) {
	logger, closer := NewLogger(&Config{
		Masking: MaskingConfig{
			Enabled: true, UseDefaultRules: true, Rules: []MaskingRuleConfig{
				{
					Field:   "api_key",
					Formats: []FieldMaskFormat{FieldMaskFormatHTTPHeader, FieldMaskFormatJSON, FieldMaskFormatURLEncoded},
					Masks:   []MaskConfig{{RegExp: "<api_key>.+?</api_key>", Mask: "<api_key>***</api_key>"}},
				},
			},
		},
	})
	defer closer()

	mLogger, ok := logger.(MaskingLogger)
	require.True(t, ok)

	require.IsType(t, &LogfAdapter{}, mLogger.log)

	masker, ok := mLogger.masker.(*Masker)
	require.True(t, ok)

	expectedMasks := []FieldMasker{
		{
			Field: "api_key",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`<api_key>.+?</api_key>`),
					Mask:   "<api_key>***</api_key>",
				},
				{
					RegExp: regexp.MustCompile(`(?i)api_key: .+?\r\n`),
					Mask:   "api_key: ***\r\n",
				},
				{
					RegExp: regexp.MustCompile(`(?i)"api_key"\s*:\s*".*?[^\\]"`),
					Mask:   `"api_key": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)api_key\s*=\s*[^&\s]+`),
					Mask:   "api_key=***",
				},
			},
		},
		{
			Field: "authorization",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)Authorization: .+?\r\n`),
					Mask:   "Authorization: ***\r\n",
				},
			},
		},
		{
			Field: "password",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)"password"\s*:\s*".*?[^\\]"`),
					Mask:   `"password": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)password\s*=\s*[^&\s]+`),
					Mask:   "password=***",
				},
			},
		},
		{
			Field: "client_secret",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)"client_secret"\s*:\s*".*?[^\\]"`),
					Mask:   `"client_secret": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)client_secret\s*=\s*[^&\s]+`),
					Mask:   "client_secret=***",
				},
			},
		},
		{
			Field: "access_token",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)"access_token"\s*:\s*".*?[^\\]"`),
					Mask:   `"access_token": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)access_token\s*=\s*[^&\s]+`),
					Mask:   "access_token=***",
				},
			},
		},
		{
			Field: "refresh_token",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)"refresh_token"\s*:\s*".*?[^\\]"`),
					Mask:   `"refresh_token": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)refresh_token\s*=\s*[^&\s]+`),
					Mask:   "refresh_token=***",
				},
			},
		},
		{
			Field: "id_token",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)"id_token"\s*:\s*".*?[^\\]"`),
					Mask:   `"id_token": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)id_token\s*=\s*[^&\s]+`),
					Mask:   "id_token=***",
				},
			},
		},
		{
			Field: "assertion",
			Masks: []Mask{
				{
					RegExp: regexp.MustCompile(`(?i)"assertion"\s*:\s*".*?[^\\]"`),
					Mask:   `"assertion": "***"`,
				},
				{
					RegExp: regexp.MustCompile(`(?i)assertion\s*=\s*[^&\s]+`),
					Mask:   "assertion=***",
				},
			},
		},
	}
	require.Equal(t, expectedMasks, masker.fieldMasks)
}

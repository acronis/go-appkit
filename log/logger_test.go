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

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package logtest

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/ssgreg/logf"

	"github.com/acronis/go-appkit/log"
)

type entryWriter struct {
	sync.Mutex
	encoder logf.Encoder
	output  io.Writer
}

//nolint:gocritic
func (ew *entryWriter) WriteEntry(e logf.Entry) {
	ew.Lock()
	defer ew.Unlock()

	var buf logf.Buffer
	err := ew.encoder.Encode(&buf, e)
	if err != nil {
		_, _ = fmt.Fprint(ew.output, err)
		return
	}
	_, _ = fmt.Fprint(ew.output, string(buf.Data))
}

// NewLogger returns a new simple preconfigured logger (output: stderr, format: json, level: debug).
// It may be used in tests and should never be used in production due to slow performance.
func NewLogger() log.FieldLogger {
	defaultOpts := LoggerOpts{
		Output: os.Stderr,
	}

	return NewLoggerWithOpts(defaultOpts)
}

// LoggerOpts allows to set custom options for test logger such as messages output target.
type LoggerOpts struct {
	Output io.Writer
}

// NewLoggerWithOpts returns logger instance configured according to options provided.
// If opts.Output value is nil it is set to os.Stderr.
func NewLoggerWithOpts(opts LoggerOpts) log.FieldLogger {
	jsonEncoder := logf.NewJSONEncoder(logf.JSONEncoderConfig{
		EncodeTime:   logf.RFC3339NanoTimeEncoder,
		FieldKeyTime: "time",
	})

	output := opts.Output
	if output == nil {
		output = os.Stderr
	}

	ew := &entryWriter{
		encoder: jsonEncoder,
		output:  output,
	}
	logger := logf.NewLogger(logf.LevelDebug, ew)
	return &log.LogfAdapter{Logger: logger}
}

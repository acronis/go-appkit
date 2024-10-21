/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ssgreg/logf"
	"github.com/ssgreg/logftext"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Field hold data of a specific field.
type Field = logf.Field

// CloseFunc allows to close channel writer.
type CloseFunc logf.ChannelWriterCloseFunc

// LogFunc allows logging a message with a bound level.
// nolint: revive
type LogFunc = logf.LogFunc

// Error returns a new Field with the given error. Key is 'error'.
var Error = logf.Error

// NamedError returns a new Field with the given key and error.
var NamedError = logf.NamedError

// String returns a new Field with the given key and string.
var String = logf.String

// Strings returns a new Field with the given key and slice of strings.
var Strings = logf.Strings

// Bytes returns a new Field with the given key and slice of bytes.
var Bytes = logf.Bytes

// Int returns a new Field with the given key and int.
var Int = logf.Int

// Int8 returns a new Field with the given key and int8.
var Int8 = logf.Int8

// Int16 returns a new Field with the given key and int16.
var Int16 = logf.Int16

// Int32 returns a new Field with the given key and int32.
var Int32 = logf.Int32

// Int64 returns a new Field with the given key and int64.
var Int64 = logf.Int64

// Uint8 returns a new Field with the given key and uint8.
var Uint8 = logf.Uint8

// Uint16 returns a new Field with the given key and uint16.
var Uint16 = logf.Uint16

// Uint32 returns a new Field with the given key and uint32.
var Uint32 = logf.Uint32

// Uint64 returns a new Field with the given key and uint64.
var Uint64 = logf.Uint64

// Float32 returns a new Field with the given key and float32.
var Float32 = logf.Float32

// Float64 returns a new Field with the given key and float64.
var Float64 = logf.Float64

// Duration returns a new Field with the given key and time.Duration.
var Duration = logf.Duration

// Bool returns a new Field with the given key and bool.
var Bool = logf.Bool

// Time returns a new Field with the given key and time.Time.
var Time = logf.Time

// Any returns a new Filed with the given key and value of any type. Is tries
// to choose the best way to represent key-value pair as a Field.
var Any = logf.Any

// DurationIn returns a new Field with the "duration" as key and received duration in unit as value (int64).
func DurationIn(val, unit time.Duration) Field {
	return Int64("duration", val.Nanoseconds()/unit.Nanoseconds())
}

// FieldLogger is an interface for loggers which writes logs in structured format.
type FieldLogger interface {
	With(...Field) FieldLogger

	Debug(string, ...Field)
	Info(string, ...Field)
	Warn(string, ...Field)
	Error(string, ...Field)

	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})

	AtLevel(Level, func(LogFunc))
	WithLevel(level Level) FieldLogger
}

// LogfAdapter adapts logf.Logger to FieldLogger interface.
type LogfAdapter struct {
	Logger *logf.Logger
}

// NewDisabledLogger returns a new logger that logs nothing.
func NewDisabledLogger() FieldLogger {
	return &LogfAdapter{logf.NewDisabledLogger()}
}

// NewLogger returns a new logger.
func NewLogger(cfg *Config) (FieldLogger, CloseFunc) {
	appender := makeLogfAppender(cfg)
	channel, closeFunc := logf.NewChannelWriter(logf.ChannelWriterConfig{
		Appender:          appender,
		EnableSyncOnError: true,
	})
	logfLogger := logf.NewLogger(convertLevelToLogfLevel(cfg.Level), channel)
	logfLogger = logfLogger.With(logf.Int("pid", os.Getpid()))
	if cfg.AddCaller {
		// show caller, but skip one last stackframe
		// to receive log line not in this file
		logfLogger = logfLogger.WithCaller().WithCallerSkip(1)
	}
	var logger FieldLogger = &LogfAdapter{logfLogger}

	if cfg.Masking.Enabled {
		rules := cfg.Masking.Rules
		if cfg.Masking.UseDefaultRules {
			rules = append(rules, DefaultMasks...)
		}
		logger = NewMaskingLogger(logger, NewMasker(rules))
	}
	return logger, CloseFunc(closeFunc)
}

// With returns a new logger with the given additional fields.
func (l *LogfAdapter) With(fs ...Field) FieldLogger {
	return &LogfAdapter{l.Logger.With(fs...)}
}

// Debug logs message at "debug" level.
func (l *LogfAdapter) Debug(s string, fields ...Field) {
	l.Logger.Debug(s, fields...)
}

// Info logs message at "info" level.
func (l *LogfAdapter) Info(s string, fields ...Field) {
	l.Logger.Info(s, fields...)
}

// Warn logs message at "warn" level.
func (l *LogfAdapter) Warn(s string, fields ...Field) {
	l.Logger.Warn(s, fields...)
}

// Error logs message at "error" level.
func (l *LogfAdapter) Error(s string, fields ...Field) {
	l.Logger.Error(s, fields...)
}

// Debugf logs a formatted message at "debug" level.
func (l *LogfAdapter) Debugf(format string, args ...interface{}) {
	l.logStringAtLevel(LevelDebug, format, args...)
}

// Infof logs a formatted message at "info" level.
func (l *LogfAdapter) Infof(format string, args ...interface{}) {
	l.logStringAtLevel(LevelInfo, format, args...)
}

// Warnf logs a formatted message at "warn" level.
func (l *LogfAdapter) Warnf(format string, args ...interface{}) {
	l.logStringAtLevel(LevelWarn, format, args...)
}

// Errorf logs a formatted message at "error" level.
func (l *LogfAdapter) Errorf(format string, args ...interface{}) {
	l.logStringAtLevel(LevelError, format, args...)
}

// logAtLevel logs a formatted message at given level.
func (l *LogfAdapter) logStringAtLevel(level Level, format string, args ...interface{}) {
	l.AtLevel(level, func(writer LogFunc) {
		writer(fmt.Sprintf(format, args...))
	})
}

// AtLevel calls the given fn if logging a message at the specified level
// is enabled, passing a LogFunc with the bound level.
func (l *LogfAdapter) AtLevel(level Level, fn func(logFunc LogFunc)) {
	l.Logger.AtLevel(convertLevelToLogfLevel(level), fn)
}

// WithLevel returns a new logger with additional level check.
// All log messages below ("debug" is a minimal level, "error" - maximal)
// the given AND previously set level will be ignored (i.e. it makes sense to only increase level).
func (l *LogfAdapter) WithLevel(level Level) FieldLogger {
	return &LogfAdapter{Logger: l.Logger.WithLevel(convertLevelToLogfLevel(level))}
}

func convertLevelToLogfLevel(value Level) logf.Level {
	switch value {
	case LevelError:
		return logf.LevelError
	case LevelWarn:
		return logf.LevelWarn
	case LevelInfo:
		return logf.LevelInfo
	case LevelDebug:
		return logf.LevelDebug
	}
	return logf.LevelInfo
}

func makeLogfAppender(cfg *Config) logf.Appender {
	switch cfg.Output {
	case OutputFile:
		writer := &lumberjack.Logger{
			Filename:   resolvePlaceholders(cfg.File.Path),
			MaxSize:    int(cfg.File.Rotation.MaxSize / 1024 / 1024),
			MaxBackups: cfg.File.Rotation.MaxBackups,
			MaxAge:     cfg.File.Rotation.MaxAgeDays,
			Compress:   cfg.File.Rotation.Compress,
			LocalTime:  cfg.File.Rotation.LocalTimeInNames,
		}
		return makeLogfAppenderWithWriter(cfg, writer)
	case OutputStderr:
		return makeLogfAppenderWithWriter(cfg, os.Stderr)
	}
	return makeLogfAppenderWithWriter(cfg, os.Stdout)
}

func makeLogfAppenderWithWriter(cfg *Config, w io.Writer) logf.Appender {
	timeEncoder := logf.RFC3339NanoTimeEncoder

	var errorEncoder logf.ErrorEncoder
	if cfg.ErrorVerboseSuffix != "" || cfg.ErrorNoVerbose {
		errorEncoder = logf.NewErrorEncoder(logf.ErrorEncoderConfig{
			NoVerboseField:     cfg.ErrorNoVerbose,
			VerboseFieldSuffix: cfg.ErrorVerboseSuffix,
		})
	}

	if cfg.Format == FormatText {
		noColor := cfg.NoColor
		return logftext.NewAppender(w, logftext.EncoderConfig{
			NoColor:     &noColor,
			EncodeTime:  timeEncoder,
			EncodeError: errorEncoder,
		})
	}

	return logf.NewWriteAppender(w, logf.NewJSONEncoder(logf.JSONEncoderConfig{
		EncodeTime:   timeEncoder,
		EncodeError:  errorEncoder,
		FieldKeyTime: "time",
	}))
}

func resolvePlaceholders(filePath string) string {
	values := map[string]string{
		"starttime": time.Now().Format("200601021504"),
		"pid":       strconv.Itoa(os.Getpid()),
	}
	res := filePath
	for placeholder, value := range values {
		res = strings.ReplaceAll(res, "{{"+placeholder+"}}", value)
	}
	return res
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import "fmt"

// PrefixedLogger represents a logger that prefixes all logging messages with a specific text.
type PrefixedLogger struct {
	delegate FieldLogger
	prefix   string
}

// NewPrefixedLogger returns a new PrefixedLogger instance.
func NewPrefixedLogger(delegate FieldLogger, prefix string) FieldLogger {
	return &PrefixedLogger{delegate, prefix}
}

// With returns a new logger with the given additional fields.
func (l *PrefixedLogger) With(fs ...Field) FieldLogger {
	return &PrefixedLogger{l.delegate.With(fs...), l.prefix}
}

// Debug logs a formatted message at "debug" level.
func (l *PrefixedLogger) Debug(text string, fs ...Field) {
	l.delegate.Debug(l.prefix+text, fs...)
}

// Info logs a formatted message at "info" level.
func (l *PrefixedLogger) Info(text string, fs ...Field) {
	l.delegate.Info(l.prefix+text, fs...)
}

// Warn logs a formatted message at "warn" level.
func (l *PrefixedLogger) Warn(text string, fs ...Field) {
	l.delegate.Warn(l.prefix+text, fs...)
}

// Error logs a formatted message at "error" level.
func (l *PrefixedLogger) Error(text string, fs ...Field) {
	l.delegate.Error(l.prefix+text, fs...)
}

// Debugf logs a formatted message at "debug" level.
func (l *PrefixedLogger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Infof logs a formatted message at "info" level.
func (l *PrefixedLogger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a formatted message at "warn" level.
func (l *PrefixedLogger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted message at "error" level.
func (l *PrefixedLogger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// AtLevel calls the given fn if logging a message at the specified level
// is enabled, passing a LogFunc with the bound level.
func (l *PrefixedLogger) AtLevel(level Level, fn func(logFunc LogFunc)) {
	l.delegate.AtLevel(level, func(logFunc LogFunc) {
		fn(func(msg string, fs ...Field) {
			logFunc(l.prefix+msg, fs...)
		})
	})
}

// WithLevel returns a new logger with additional level check.
// All log messages below ("debug" is a minimal level, "error" - maximal)
// the given AND previously set level will be ignored (i.e. it makes sense to only increase level).
func (l *PrefixedLogger) WithLevel(level Level) FieldLogger {
	return &PrefixedLogger{l.delegate.WithLevel(level), l.prefix}
}

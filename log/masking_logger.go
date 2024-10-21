package log

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"unsafe"

	"github.com/ssgreg/logf"
)

// MaskingLogger is a logger that masks secrets in log fields.
// Use it to make sure secrets are not leaked in logs:
// - If you dump HTTP requests and responses in debug mode.
// - If a secret is passed via URL (like &api_key=<secret>), network connectivity error will leak it.
type MaskingLogger struct {
	log    FieldLogger
	masker StringMasker
}

type StringMasker interface {
	Mask(s string) string
}

func NewMaskingLogger(l FieldLogger, r StringMasker) FieldLogger {
	return MaskingLogger{l, r}
}

// With returns a new logger with the given additional fields.
func (l MaskingLogger) With(fs ...Field) FieldLogger {
	return MaskingLogger{l.log.With(l.maskFields(fs)...), l.masker}
}

// Debug logs a formatted Message at "debug" level.
func (l MaskingLogger) Debug(text string, fs ...Field) {
	l.log.Debug(l.masker.Mask(text), l.maskFields(fs)...)
}

// Info logs a formatted Message at "info" level.
func (l MaskingLogger) Info(text string, fs ...Field) {
	l.log.Info(l.masker.Mask(text), l.maskFields(fs)...)
}

// Warn logs a formatted Message at "warn" level.
func (l MaskingLogger) Warn(text string, fs ...Field) {
	l.log.Warn(l.masker.Mask(text), l.maskFields(fs)...)
}

// Error logs a formatted Message at "error" level.
func (l MaskingLogger) Error(text string, fs ...Field) {
	l.log.Error(l.masker.Mask(text), l.maskFields(fs)...)
}

// Debugf logs a formatted Message at "debug" level.
func (l MaskingLogger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Infof logs a formatted Message at "info" level.
func (l MaskingLogger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warnf logs a formatted Message at "warn" level.
func (l MaskingLogger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted Message at "error" level.
func (l MaskingLogger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// AtLevel calls the given fn if logging a Message at the specified level
// is enabled, passing a LogFunc with the bound level.
func (l MaskingLogger) AtLevel(level Level, fn func(logFunc LogFunc)) {
	l.log.AtLevel(level, func(logFunc LogFunc) {
		fn(func(msg string, fs ...Field) {
			logFunc(l.masker.Mask(msg), l.maskFields(fs)...)
		})
	})
}

// WithLevel returns a new logger with additional level check.
// All log messages below ("debug" is a minimal level, "error" - maximal)
// the given AND previously set level will be ignored (i.e. it makes sense to only increase level).
func (l MaskingLogger) WithLevel(level Level) FieldLogger {
	return MaskingLogger{l.log.WithLevel(level), l.masker}
}

var stringSliceType = reflect.TypeOf([]string{})

// maskFields masks secrets in log fields
// nolint: funlen,gocyclo
func (l MaskingLogger) maskFields(fields []Field) []Field {
	var changedFields []*Field
	for i, field := range fields {
		field := field // Important when working with unsafe.Pointer
		switch field.Type {
		case logf.FieldTypeBytesToString:
			s := *(*string)(unsafe.Pointer(&field.Bytes)) // nolint: gosec
			masked := l.masker.Mask(s)
			if masked != s {
				if changedFields == nil {
					changedFields = make([]*Field, len(fields))
				}
				newField := String(field.Key, masked)
				changedFields[i] = &newField
			}
		case logf.FieldTypeError:
			if field.Any != nil {
				err := field.Any.(error)
				s := err.Error()
				masked := l.masker.Mask(s)
				if masked != s {
					if changedFields == nil {
						changedFields = make([]*Field, len(fields))
					}
					newField := NamedError(field.Key, newMaskedError(err, l.masker, masked))
					changedFields[i] = &newField
				}
			}
		case logf.FieldTypeBytes, logf.FieldTypeRawBytes:
			if field.Bytes != nil {
				masked := l.masker.Mask(string(field.Bytes))
				if masked != string(field.Bytes) {
					if changedFields == nil {
						changedFields = make([]*Field, len(fields))
					}
					newField := logf.ConstBytes(field.Key, []byte(masked))
					changedFields[i] = &newField
				}
			}
		case logf.FieldTypeArray:
			if field.Any != nil {
				value := reflect.ValueOf(field.Any)
				if value.CanConvert(stringSliceType) {
					ss := value.Convert(stringSliceType).Interface().([]string)
					var changed bool
					masked := make([]string, len(ss))
					for i, s := range ss {
						masked[i] = l.masker.Mask(s)
						if masked[i] != s {
							changed = true
						}
					}
					if changed {
						if changedFields == nil {
							changedFields = make([]*Field, len(fields))
						}
						newField := Strings(field.Key, masked)
						changedFields[i] = &newField
					}
				}
			}
		case logf.FieldTypeAny:
			// NOTE: Not masked
		}
	}

	if changedFields == nil {
		return fields
	}

	newFields := make([]Field, len(fields))
	copy(newFields, fields)
	for i, field := range changedFields {
		if field != nil {
			newFields[i] = *field
		}
	}

	return newFields
}

func newMaskedError(err error, r StringMasker, masked string) error {
	switch err.(type) {
	case fmt.Formatter:
		return maskedError{
			s:        masked,
			verboseS: r.Mask(fmt.Sprintf("%+v", err)),
		}
	default:
		return errors.New(masked)
	}
}

// maskedError is needed to support logf "error_verbose" field.
type maskedError struct {
	s        string
	verboseS string
}

func (e maskedError) Error() string {
	return e.s
}

func (e maskedError) Format(f fmt.State, verb rune) {
	_, _ = io.WriteString(f, e.verboseS)
}

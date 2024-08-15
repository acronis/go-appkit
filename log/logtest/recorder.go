/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package logtest

import (
	"sync"
	"time"

	"github.com/ssgreg/logf"

	"github.com/acronis/go-appkit/log"
)

// RecordedEntry represents recorded entry which was logged.
type RecordedEntry struct {
	LoggerName string
	Fields     []log.Field
	Level      log.Level
	Time       time.Time
	Text       string
}

// FindField tries to find field in logging entry by key.
func (re *RecordedEntry) FindField(key string) (*log.Field, bool) {
	for _, field := range re.Fields {
		if field.Key == key {
			return &field, true
		}
	}
	return nil, false
}

type recordingEntryWriter struct {
	sync.RWMutex
	Entries []RecordedEntry
}

//nolint:gocritic
func (ew *recordingEntryWriter) WriteEntry(e logf.Entry) {
	ew.Lock()
	defer ew.Unlock()

	allFields := append([]log.Field{}, e.Fields...)
	allFields = append(allFields, e.DerivedFields...)
	me := RecordedEntry{
		LoggerName: e.LoggerName,
		Fields:     allFields,
		Level:      convertLogfLevelToLevel(e.Level),
		Time:       e.Time,
		Text:       e.Text,
	}

	ew.Entries = append(ew.Entries, me)
}

// Recorder is an implementation of log.FieldLogger that
// records all logged entries for later inspection in tests.
type Recorder struct {
	*log.LogfAdapter
	entryWriter *recordingEntryWriter
}

// NewRecorder returns an initialized Recorder.
func NewRecorder() *Recorder {
	ew := &recordingEntryWriter{}
	logger := logf.NewLogger(logf.LevelDebug, ew)
	return &Recorder{&log.LogfAdapter{Logger: logger}, ew}
}

// With returns a new Recorder with the given additional fields.
func (r *Recorder) With(fs ...log.Field) log.FieldLogger {
	return &Recorder{r.LogfAdapter.With(fs...).(*log.LogfAdapter), r.entryWriter}
}

// WithLevel returns a new Recorder with the given additional level check.
// All log messages below ("debug" is a minimal level, "error" - maximal)
// the given AND previously set level will be ignored (i.e. it makes sense to only increase level).
func (r *Recorder) WithLevel(level log.Level) log.FieldLogger {
	return &Recorder{r.LogfAdapter.WithLevel(level).(*log.LogfAdapter), r.entryWriter}
}

// Entries returns all recorded logging entries.
func (r *Recorder) Entries() []RecordedEntry {
	r.entryWriter.RLock()
	defer r.entryWriter.RUnlock()
	return append([]RecordedEntry{}, r.entryWriter.Entries...)
}

// FindEntry tries to find recorded logging entry by message.
func (r *Recorder) FindEntry(msg string) (RecordedEntry, bool) {
	return r.FindEntryByFilter(func(entry RecordedEntry) bool {
		return entry.Text == msg
	})
}

// FindEntryByFilter tries to find recorded logging entry by filter (callback).
func (r *Recorder) FindEntryByFilter(filter func(entry RecordedEntry) bool) (RecordedEntry, bool) {
	r.entryWriter.RLock()
	defer r.entryWriter.RUnlock()
	for _, entry := range r.entryWriter.Entries {
		if filter(entry) {
			return entry, true
		}
	}
	return RecordedEntry{}, false
}

// FindAllEntriesByFilter tries to find all recorded logging entries by filter (callback).
func (r *Recorder) FindAllEntriesByFilter(filter func(entry RecordedEntry) bool) []RecordedEntry {
	r.entryWriter.RLock()
	defer r.entryWriter.RUnlock()
	var foundEntries []RecordedEntry
	for _, entry := range r.entryWriter.Entries {
		if filter(entry) {
			foundEntries = append(foundEntries, entry)
		}
	}
	return foundEntries
}

// Reset resets all recorded logs.
func (r *Recorder) Reset() {
	r.entryWriter.Lock()
	r.entryWriter.Entries = nil
	r.entryWriter.Unlock()
}

func convertLogfLevelToLevel(value logf.Level) log.Level {
	switch value {
	case logf.LevelError:
		return log.LevelError
	case logf.LevelWarn:
		return log.LevelWarn
	case logf.LevelInfo:
		return log.LevelInfo
	case logf.LevelDebug:
		return log.LevelDebug
	}
	return log.LevelInfo
}

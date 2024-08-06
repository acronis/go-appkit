/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/log"
	"git.acronis.com/abc/go-libs/v2/log/logtest"
)

func TestPrefixedLogger(t *testing.T) {
	const prefix = "PREFIX: "
	recorder := logtest.NewRecorder()
	logger := log.NewPrefixedLogger(recorder, prefix)

	checkRecordedLogAndReset := func(wantText string, wantLevel log.Level, wantFields ...log.Field) {
		entries := recorder.Entries()
		require.Len(t, entries, 1)
		require.Equal(t, wantText, entries[0].Text)
		require.Equal(t, wantLevel, entries[0].Level)
		if len(wantFields) != 0 && len(entries[0].Fields) != 0 {
			require.Equal(t, wantFields, entries[0].Fields)
		}
		recorder.Reset()
	}

	logger.Debug("hello world, debug", log.Int("user_id", 42))
	checkRecordedLogAndReset(prefix+"hello world, debug", log.LevelDebug, log.Int("user_id", 42))
	logger.Debugf("hello %s, debugf", "world")
	checkRecordedLogAndReset(prefix+"hello world, debugf", log.LevelDebug)

	logger.Info("hello world, info", log.Int("user_id", 42))
	checkRecordedLogAndReset(prefix+"hello world, info", log.LevelInfo, log.Int("user_id", 42))
	logger.Infof("hello %s, infof", "world")
	checkRecordedLogAndReset(prefix+"hello world, infof", log.LevelInfo)

	logger.Warn("hello world, warn", log.Int("user_id", 42))
	checkRecordedLogAndReset(prefix+"hello world, warn", log.LevelWarn, log.Int("user_id", 42))
	logger.Warnf("hello %s, warnf", "world")
	checkRecordedLogAndReset(prefix+"hello world, warnf", log.LevelWarn)

	logger.Error("hello world, error", log.Int("user_id", 42))
	checkRecordedLogAndReset(prefix+"hello world, error", log.LevelError, log.Int("user_id", 42))
	logger.Errorf("hello %s, errorf", "world")
	checkRecordedLogAndReset(prefix+"hello world, errorf", log.LevelError)

	logger2 := logger.With(log.String("name", "John"))
	logger2.Info("hello")
	checkRecordedLogAndReset(prefix+"hello", log.LevelInfo, log.String("name", "John"))

	logger.AtLevel(log.LevelInfo, func(logFunc log.LogFunc) {
		logFunc("hello", log.String("name", "John"))
	})
	checkRecordedLogAndReset(prefix+"hello", log.LevelInfo, log.String("name", "John"))
}

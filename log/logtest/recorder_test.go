/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package logtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"git.acronis.com/abc/go-libs/v2/log"
)

func TestRecorder(t *testing.T) {
	logRecorder := NewRecorder()
	logRecorder.Warn("message1", log.Int("num", 10), log.String("str", "abc"))
	logRecorder.Info("message2")

	require.Equal(t, 2, len(logRecorder.Entries()))

	_, found := logRecorder.FindEntry("foobar")
	require.False(t, found)

	logEntry, found := logRecorder.FindEntry("message1")
	require.True(t, found)
	require.Equal(t, log.LevelWarn, logEntry.Level)
	require.Equal(t, "message1", logEntry.Text)

	_, found = logRecorder.FindEntry("unknown")
	require.False(t, found)

	logFieldNum, found := logEntry.FindField("num")
	require.True(t, found)
	require.Equal(t, 10, int(logFieldNum.Int))

	logFieldStr, found := logEntry.FindField("str")
	require.True(t, found)
	require.Equal(t, "abc", string(logFieldStr.Bytes))
}

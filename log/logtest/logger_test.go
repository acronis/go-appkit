/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package logtest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// nolint
func TestLogger(t *testing.T) {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	logger := NewLoggerWithOpts(LoggerOpts{Output: w})

	logger.Errorf("test")
	w.Flush()

	var j map[string]string
	require.NoError(t, json.Unmarshal(b.Bytes(), &j))

	require.Equal(t, j["level"], "error")
	require.Equal(t, j["msg"], "test")
}

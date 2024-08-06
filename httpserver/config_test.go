/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpserver

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-libs/config"
)

func TestConfig(t *testing.T) {
	requireDuration := func(wantStr string, got time.Duration) {
		want, err := time.ParseDuration(wantStr)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}

	t.Run("default values", func(t *testing.T) {
		cfgData := bytes.NewBuffer(nil)
		cfg := Config{}
		err := config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.NoError(t, err)
		require.Equal(t, defaultServerAddress, cfg.Address)
		requireDuration(defaultServerTimeoutsWrite, cfg.Timeouts.Write)
		requireDuration(defaultServerTimeoutsRead, cfg.Timeouts.Read)
		requireDuration(defaultServerTimeoutsReadHeader, cfg.Timeouts.ReadHeader)
		requireDuration(defaultServerTimeoutsIdle, cfg.Timeouts.Idle)
		requireDuration(defaultServerTimeoutsShutdown, cfg.Timeouts.Shutdown)
		require.Equal(t, defaultServerLimitsMaxRequests, cfg.Limits.MaxRequests)
		require.False(t, cfg.Log.RequestStart)
	})

	t.Run("read values", func(t *testing.T) {
		cfgData := bytes.NewBuffer([]byte(`
server:
  tls:
    enabled: true
    key: "/test/path"
    cert: "/test/path"
  address: "127.0.0.1:777"
  timeouts:
    write: 1h
    read: 7m
    readheader: 1m
    idle: 20m
    shutdown: 30s
  limits:
    maxrequests: 10
    maxbodysize: 1M
  log:
    requeststart: true
`))
		cfg := Config{}
		err := config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1:777", cfg.Address)
		requireDuration("1h", cfg.Timeouts.Write)
		requireDuration("7m", cfg.Timeouts.Read)
		requireDuration("1m", cfg.Timeouts.ReadHeader)
		requireDuration("20m", cfg.Timeouts.Idle)
		requireDuration("30s", cfg.Timeouts.Shutdown)
		require.Equal(t, 10, cfg.Limits.MaxRequests)
		require.Equal(t, uint64(1024*1024), cfg.Limits.MaxBodySizeBytes)

		require.True(t, cfg.TLS.Enabled)
		require.Equal(t, "/test/path", cfg.TLS.Certificate)
		require.Equal(t, "/test/path", cfg.TLS.Key)

		require.True(t, cfg.Log.RequestStart)
	})

	t.Run("read values, unix socket", func(t *testing.T) {
		cfgData := bytes.NewBuffer([]byte(`
server:
  unixSocketPath: "/var/run/test.sock"
`))
		cfg := Config{}
		err := config.NewDefaultLoader("").LoadFromReader(cfgData, config.DataTypeYAML, &cfg)
		require.NoError(t, err)
		require.Equal(t, "/var/run/test.sock", cfg.UnixSocketPath)
	})
}

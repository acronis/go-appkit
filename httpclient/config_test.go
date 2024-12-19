/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package httpclient

import (
	"bytes"
	"github.com/acronis/go-appkit/config"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConfigWithLoader(t *testing.T) {
	yamlData := []byte(`
retries:
  max_retry_attempts: 30
rate_limits:
  limit: 300
  burst: 3000
  wait_timeout: 3s
timeout: 30s
`)

	actualConfig := &Config{}
	err := config.NewDefaultLoader("").LoadFromReader(bytes.NewReader(yamlData), config.DataTypeYAML, actualConfig)
	require.NoError(t, err, "load configuration")

	expectedConfig := &Config{
		Retries: &RetriesConfig{
			MaxRetryAttempts: 30,
		},
		RateLimits: &RateLimitConfig{
			Limit:       300,
			Burst:       3000,
			WaitTimeout: 3 * time.Second,
		},
		Timeout: 30 * time.Second,
	}

	require.Equal(t, expectedConfig, actualConfig, "configuration does not match expected")
}

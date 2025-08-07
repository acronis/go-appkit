/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
)

type ConfigTestSuite struct {
	suite.Suite
}

func TestConfigTestSuite(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

func (s *ConfigTestSuite) TestConfig_Validation() {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "valid config",
			config: &Config{
				RateLimitZones: map[string]RateLimitZoneConfig{
					"zone1": {
						RateLimit: RateLimitValue{Count: 10, Duration: time.Second},
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{"/test.Service/*"},
						RateLimits:     []RuleRateLimit{{Zone: "zone1"}},
					},
				},
			},
		},
		{
			name: "invalid rate limit zone - zero rate",
			config: &Config{
				RateLimitZones: map[string]RateLimitZoneConfig{
					"zone1": {
						RateLimit: RateLimitValue{Count: 0, Duration: time.Second},
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{"/test.Service/*"},
						RateLimits:     []RuleRateLimit{{Zone: "zone1"}},
					},
				},
			},
			expectedErr: `validate rate limit zone "zone1": rate limit should be >= 1, got 0`,
		},
		{
			name: "invalid in-flight limit zone - zero limit",
			config: &Config{
				InFlightLimitZones: map[string]InFlightLimitZoneConfig{
					"zone1": {
						InFlightLimit: 0,
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{"/test.Service/*"},
						InFlightLimits: []RuleInFlightLimit{{Zone: "zone1"}},
					},
				},
			},
			expectedErr: `validate in-flight limit zone "zone1": in-flight limit should be >= 1, got 0`,
		},
		{
			name: "rule references undefined zone",
			config: &Config{
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{"/test.Service/*"},
						RateLimits:     []RuleRateLimit{{Zone: "undefined_zone"}},
					},
				},
			},
			expectedErr: `validate rule "/test.Service/*": rate limit zone "undefined_zone" is undefined`,
		},
		{
			name: "rule with no service methods",
			config: &Config{
				RateLimitZones: map[string]RateLimitZoneConfig{
					"zone1": {
						RateLimit: RateLimitValue{Count: 10, Duration: time.Second},
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{},
						RateLimits:     []RuleRateLimit{{Zone: "zone1"}},
					},
				},
			},
			expectedErr: `validate rule "": serviceMethods is missing`,
		},
		{
			name: "invalid service method - multiple wildcards",
			config: &Config{
				RateLimitZones: map[string]RateLimitZoneConfig{
					"zone1": {
						RateLimit: RateLimitValue{Count: 10, Duration: time.Second},
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{"/test.Service/*/*"},
						RateLimits:     []RuleRateLimit{{Zone: "zone1"}},
					},
				},
			},
			expectedErr: `validate rule "/test.Service/*/*": serviceMethod "/test.Service/*/*" contains multiple wildcards or non-trailing wildcard`,
		},
		{
			name: "invalid service method - non-trailing wildcard",
			config: &Config{
				RateLimitZones: map[string]RateLimitZoneConfig{
					"zone1": {
						RateLimit: RateLimitValue{Count: 10, Duration: time.Second},
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods: []string{"/test.Service/*/Method"},
						RateLimits:     []RuleRateLimit{{Zone: "zone1"}},
					},
				},
			},
			expectedErr: `validate rule "/test.Service/*/Method": serviceMethod "/test.Service/*/Method" contains multiple wildcards or non-trailing wildcard`,
		},
		{
			name: "invalid excluded service method - multiple wildcards",
			config: &Config{
				RateLimitZones: map[string]RateLimitZoneConfig{
					"zone1": {
						RateLimit: RateLimitValue{Count: 10, Duration: time.Second},
						ZoneConfig: ZoneConfig{
							Key: ZoneKeyConfig{Type: ZoneKeyTypeNoKey},
						},
					},
				},
				Rules: []RuleConfig{
					{
						ServiceMethods:         []string{"/test.Service/*"},
						ExcludedServiceMethods: []string{"/test.Service/*/*"},
						RateLimits:             []RuleRateLimit{{Zone: "zone1"}},
					},
				},
			},
			expectedErr: `validate rule "/test.Service/*": excludedServiceMethod "/test.Service/*/*" contains multiple wildcards or non-trailing wildcard`,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.config.Validate()
			if tt.expectedErr != "" {
				s.Require().Error(err)
				s.Require().Equal(tt.expectedErr, err.Error())
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

const yamlTestConfig = `
rateLimitZones:
  rate_limit_total:
    rateLimit: 6000/m
    burstLimit: 1000
    backlogLimit: 100
    backlogTimeout: 30s
    responseRetryAfter: auto
    excludedKeys: []
    includedKeys: []
    dryRun: false

  rate_limit_identity:
    key:
      type: identity
    maxKeys: 10000
    alg: leaky_bucket
    rateLimit: 50/s
    burstLimit: 100
    responseRetryAfter: 15s
    excludedKeys: ["2801c8de-7b41-4950-94e8-ad8fe8bd6d60", "7ab74f7c-846e-435f-96d4-5a0ce7068ddf"]
    includedKeys: []
    dryRun: true

  rate_limit_identity_window:
    key:
      type: identity
    maxKeys: 10000
    alg: sliding_window
    rateLimit: 500/m
    responseRetryAfter: 10s
    excludedKeys: []
    includedKeys: []
    dryRun: true

inFlightLimitZones:
  in_flight_limit_total:
    inFlightLimit: 5000
    backlogLimit: 10000
    backlogTimeout: 15s
    excludedKeys: []
    includedKeys: []
    dryRun: false

  in_flight_limit_identity:
    key:
      type: identity
    maxKeys: 10000
    inFlightLimit: 32
    backlogLimit: 64
    backlogTimeout: 5s
    responseRetryAfter: 30s
    excludedKeys: []
    includedKeys: ["7ab74f7c-846e-435f-96d4-5a0ce7068ddf", "2801c8de-7b41-4950-94e8-ad8fe8bd6d60"]
    dryRun: true

  in_flight_limit_remote_addr:
    key:
      type: remote_addr
    maxKeys: 5000
    inFlightLimit: 1000
    backlogLimit: 2000
    backlogTimeout: 5s

rules:
  - serviceMethods:
      - "/test.UserService/*"
      - "/test.TenantService/*"
    excludedServiceMethods:
      - "/test.UserService/GetUser"
      - "/test.TenantService/GetTenant"
    rateLimits:
      - zone: rate_limit_identity
      - zone: rate_limit_identity_window
    inFlightLimits:
      - zone: in_flight_limit_identity
      - zone: in_flight_limit_remote_addr
    tags: ["tag1", "tag2"]

  - alias: "limit_batches"
    serviceMethods:
      - "/test.TenantService/CreateTenant"
      - "/test.UserService/CreateUser"
    rateLimits:
      - zone: rate_limit_total
    inFlightLimits:
      - zone: in_flight_limit_total
    tags: tag_a, tag_b
`

const jsonTestConfig = `
{
  "rateLimitZones": {
    "rate_limit_total": {
      "rateLimit": "6000/m",
      "burstLimit": 1000,
      "backlogLimit": 100,
      "backlogTimeout": "30s",
      "responseRetryAfter": "auto",
      "excludedKeys": [],
      "includedKeys": [],
      "dryRun": false
    },
    "rate_limit_identity": {
      "key": {
        "type": "identity"
      },
      "maxKeys": 10000,
      "alg": "leaky_bucket",
      "rateLimit": "50/s",
      "burstLimit": 100,
      "responseRetryAfter": "15s",
      "excludedKeys": [
        "2801c8de-7b41-4950-94e8-ad8fe8bd6d60",
        "7ab74f7c-846e-435f-96d4-5a0ce7068ddf"
      ],
      "includedKeys": [],
      "dryRun": true
    },
    "rate_limit_identity_window": {
      "key": {
        "type": "identity"
      },
      "maxKeys": 10000,
      "alg": "sliding_window",
      "rateLimit": "500/m",
      "responseRetryAfter": "10s",
      "excludedKeys": [],
      "includedKeys": [],
      "dryRun": true
    }
  },
  "inFlightLimitZones": {
    "in_flight_limit_total": {
      "inFlightLimit": 5000,
      "backlogLimit": 10000,
      "backlogTimeout": "15s",
      "excludedKeys": [],
      "includedKeys": [],
      "dryRun": false
    },
    "in_flight_limit_identity": {
      "key": {
        "type": "identity"
      },
      "maxKeys": 10000,
      "inFlightLimit": 32,
      "backlogLimit": 64,
      "backlogTimeout": "5s",
      "responseRetryAfter": "30s",
      "excludedKeys": [],
      "includedKeys": [
        "7ab74f7c-846e-435f-96d4-5a0ce7068ddf",
        "2801c8de-7b41-4950-94e8-ad8fe8bd6d60"
      ],
      "dryRun": true
    },
    "in_flight_limit_remote_addr": {
      "key": {
        "type": "remote_addr"
      },
      "maxKeys": 5000,
      "inFlightLimit": 1000,
      "backlogLimit": 2000,
      "backlogTimeout": "5s"
    }
  },
  "rules": [
    {
      "serviceMethods": [
        "/test.UserService/*",
        "/test.TenantService/*"
      ],
      "excludedServiceMethods": [
        "/test.UserService/GetUser",
        "/test.TenantService/GetTenant"
      ],
      "rateLimits": [
        { "zone": "rate_limit_identity" },
        { "zone": "rate_limit_identity_window" }
      ],
      "inFlightLimits": [
        { "zone": "in_flight_limit_identity" },
        { "zone": "in_flight_limit_remote_addr" }
      ],
      "tags": ["tag1", "tag2"]
    },
    {
      "alias": "limit_batches",
      "serviceMethods": [
        "/test.TenantService/CreateTenant",
        "/test.UserService/CreateUser"
      ],
      "rateLimits": [
        { "zone": "rate_limit_total" }
      ],
      "inFlightLimits": [
        { "zone": "in_flight_limit_total" }
      ],
      "tags": "tag_a, tag_b"
    }
  ]
}
`

func requireTestConfig(t *testing.T, cfg *Config) {
	t.Helper()

	// Check rateLimitZones
	require.Len(t, cfg.RateLimitZones, 3)
	require.Equal(t, RateLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:          ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
			MaxKeys:      10000,
			DryRun:       true,
			ExcludedKeys: []string{"2801c8de-7b41-4950-94e8-ad8fe8bd6d60", "7ab74f7c-846e-435f-96d4-5a0ce7068ddf"},
			IncludedKeys: []string{},
		},
		Alg:                RateLimitAlgLeakyBucket,
		RateLimit:          RateLimitValue{Count: 50, Duration: time.Second},
		BurstLimit:         100,
		ResponseRetryAfter: RateLimitRetryAfterValue{Duration: time.Second * 15},
	}, cfg.RateLimitZones["rate_limit_identity"])
	require.Equal(t, RateLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:          ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
			MaxKeys:      10000,
			DryRun:       true,
			ExcludedKeys: []string{},
			IncludedKeys: []string{},
		},
		Alg:                RateLimitAlgSlidingWindow,
		RateLimit:          RateLimitValue{Count: 500, Duration: time.Minute},
		ResponseRetryAfter: RateLimitRetryAfterValue{Duration: time.Second * 10},
	}, cfg.RateLimitZones["rate_limit_identity_window"])
	require.Equal(t, RateLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:          ZoneKeyConfig{Type: ""},
			MaxKeys:      0,
			DryRun:       false,
			ExcludedKeys: []string{},
			IncludedKeys: []string{},
		},
		RateLimit:          RateLimitValue{Count: 6000, Duration: time.Minute},
		BurstLimit:         1000,
		BacklogLimit:       100,
		BacklogTimeout:     config.TimeDuration(time.Second * 30),
		ResponseRetryAfter: RateLimitRetryAfterValue{IsAuto: true},
	}, cfg.RateLimitZones["rate_limit_total"])

	// Check inFlightLimitZones
	require.Len(t, cfg.InFlightLimitZones, 3)
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:          ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
			MaxKeys:      10000,
			DryRun:       true,
			ExcludedKeys: []string{},
			IncludedKeys: []string{"7ab74f7c-846e-435f-96d4-5a0ce7068ddf", "2801c8de-7b41-4950-94e8-ad8fe8bd6d60"},
		},
		InFlightLimit:      32,
		BacklogLimit:       64,
		BacklogTimeout:     config.TimeDuration(time.Second * 5),
		ResponseRetryAfter: config.TimeDuration(time.Second * 30),
	}, cfg.InFlightLimitZones["in_flight_limit_identity"])
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:          ZoneKeyConfig{Type: ""},
			DryRun:       false,
			ExcludedKeys: []string{},
			IncludedKeys: []string{},
		},
		InFlightLimit:      5000,
		BacklogLimit:       10000,
		BacklogTimeout:     config.TimeDuration(time.Second * 15),
		ResponseRetryAfter: config.TimeDuration(0),
	}, cfg.InFlightLimitZones["in_flight_limit_total"])
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:     ZoneKeyConfig{Type: ZoneKeyTypeRemoteAddr},
			MaxKeys: 5000,
		},
		InFlightLimit:      1000,
		BacklogLimit:       2000,
		BacklogTimeout:     config.TimeDuration(time.Second * 5),
		ResponseRetryAfter: config.TimeDuration(time.Duration(0)),
	}, cfg.InFlightLimitZones["in_flight_limit_remote_addr"])

	// Check rules.
	require.Len(t, cfg.Rules, 2)
	require.Equal(t, []RuleConfig{
		{
			ServiceMethods:         []string{"/test.UserService/*", "/test.TenantService/*"},
			ExcludedServiceMethods: []string{"/test.UserService/GetUser", "/test.TenantService/GetTenant"},
			RateLimits:             []RuleRateLimit{{Zone: "rate_limit_identity"}, {Zone: "rate_limit_identity_window"}},
			InFlightLimits:         []RuleInFlightLimit{{Zone: "in_flight_limit_identity"}, {Zone: "in_flight_limit_remote_addr"}},
			Tags:                   TagsList{"tag1", "tag2"},
		},
		{
			Alias:          "limit_batches",
			ServiceMethods: []string{"/test.TenantService/CreateTenant", "/test.UserService/CreateUser"},
			RateLimits:     []RuleRateLimit{{Zone: "rate_limit_total"}},
			InFlightLimits: []RuleInFlightLimit{{Zone: "in_flight_limit_total"}},
			Tags:           TagsList{"tag_a", "tag_b"},
		},
	}, cfg.Rules)
}

func TestConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfgDataType config.DataType
		cfgData     string
		checkFunc   func(t *testing.T, cfg *Config)
	}{
		{
			name:        "yaml config",
			cfgDataType: config.DataTypeYAML,
			cfgData:     yamlTestConfig,
			checkFunc:   requireTestConfig,
		},
		{
			name:        "json config",
			cfgDataType: config.DataTypeJSON,
			cfgData:     jsonTestConfig,
			checkFunc:   requireTestConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg *Config

			// Load config using config.Loader.
			cfg = NewConfig()
			cfgLoader := config.NewLoader(config.NewViperAdapter())
			err := cfgLoader.LoadFromReader(bytes.NewBuffer([]byte(tt.cfgData)), tt.cfgDataType, cfg)
			require.NoError(t, err)
			requireTestConfig(t, cfg)

			// Load config using viper unmarshal.
			cfg = NewConfig()
			vpr := viper.New()
			vpr.SetConfigType(string(tt.cfgDataType))
			require.NoError(t, vpr.ReadConfig(bytes.NewBuffer([]byte(tt.cfgData))))
			require.NoError(t, vpr.Unmarshal(&cfg, func(decoderConfig *mapstructure.DecoderConfig) {
				decoderConfig.DecodeHook = MapstructureDecodeHook()
			}))
			requireTestConfig(t, cfg)

			// Load config using yaml/json unmarshal.
			cfg = NewConfig()
			switch tt.cfgDataType {
			case config.DataTypeYAML:
				require.NoError(t, yaml.Unmarshal([]byte(tt.cfgData), &cfg))
				requireTestConfig(t, cfg)
			case config.DataTypeJSON:
				require.NoError(t, json.Unmarshal([]byte(tt.cfgData), &cfg))
				requireTestConfig(t, cfg)
			default:
				t.Fatalf("unsupported config data type: %s", tt.cfgDataType)
			}
		})
	}
}

func (s *ConfigTestSuite) TestRuleConfig_Name() {
	tests := []struct {
		name     string
		rule     RuleConfig
		expected string
	}{
		{
			name: "with alias",
			rule: RuleConfig{
				Alias:          "test_rule",
				ServiceMethods: []string{"/test.Service/*"},
			},
			expected: "test_rule",
		},
		{
			name: "without alias - single method",
			rule: RuleConfig{
				ServiceMethods: []string{"/test.Service/Method"},
			},
			expected: "/test.Service/Method",
		},
		{
			name: "without alias - multiple methods",
			rule: RuleConfig{
				ServiceMethods: []string{"/test.Service/Method1", "/test.Service/Method2"},
			},
			expected: "/test.Service/Method1; /test.Service/Method2",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Require().Equal(tt.expected, tt.rule.Name())
		})
	}
}

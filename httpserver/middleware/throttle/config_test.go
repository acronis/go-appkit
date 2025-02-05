/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package throttle

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
	"github.com/acronis/go-appkit/restapi"
)

const yamlTestConfig = `
rateLimitZones:
  rate_limit_total:
    rateLimit: 6000/m
    burstLimit: 1000
    backlogLimit: 100
    backlogTimeout: 30s
    responseStatusCode: 503
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
    responseStatusCode: 429
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
    responseStatusCode: 429
    responseRetryAfter: 10s
    excludedKeys: []
    includedKeys: []
    dryRun: true

inFlightLimitZones:
  in_flight_limit_total:
    inFlightLimit: 5000
    backlogLimit: 10000
    backlogTimeout: 15s
    responseStatusCode: 503
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
    responseStatusCode: 429
    responseRetryAfter: 30s
    excludedKeys: []
    includedKeys: ["7ab74f7c-846e-435f-96d4-5a0ce7068ddf", "2801c8de-7b41-4950-94e8-ad8fe8bd6d60"]
    dryRun: true

  in_flight_limit_tenant:
    key:
      type: header
      headerName: X-Tenant-ID
    maxKeys: 5000
    inFlightLimit: 16
    backlogLimit: 24
    backlogTimeout: 5s
    responseStatusCode: 429
    excludedKeys: []
    includedKeys: []
    dryRun: true

  in_flight_limit_remote_addr:
    key:
      type: remote_addr
    maxKeys: 5000
    inFlightLimit: 1000
    backlogLimit: 2000
    backlogTimeout: 5s
    responseStatusCode: 503

rules:
  - routes:
      - path: /api/2/users
      - path: /api/2/tenants
    excludedRoutes:
      - path: /api/2/users/42
      - path: /api/2/tenants/42
    rateLimits:
      - zone: rate_limit_identity
      - zone: rate_limit_identity_window
    inFlightLimits:
      - zone: in_flight_limit_identity
      - zone: in_flight_limit_remote_addr
    tags: ["tag1", "tag2"]

  - alias: "limit_batches"
    routes:
      - path: "= /api/2/tenants"
        methods: POST, DELETE
      - path: "= /api/2/users"
        methods: ["POST", "DELETE", "PUT"]
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
      "responseStatusCode": 503,
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
      "responseStatusCode": 429,
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
      "responseStatusCode": 429,
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
      "responseStatusCode": 503,
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
      "responseStatusCode": 429,
      "responseRetryAfter": "30s",
      "excludedKeys": [],
      "includedKeys": [
        "7ab74f7c-846e-435f-96d4-5a0ce7068ddf",
        "2801c8de-7b41-4950-94e8-ad8fe8bd6d60"
      ],
      "dryRun": true
    },
    "in_flight_limit_tenant": {
      "key": {
        "type": "header",
        "headerName": "X-Tenant-ID"
      },
      "maxKeys": 5000,
      "inFlightLimit": 16,
      "backlogLimit": 24,
      "backlogTimeout": "5s",
      "responseStatusCode": 429,
      "excludedKeys": [],
      "includedKeys": [],
      "dryRun": true
    },
    "in_flight_limit_remote_addr": {
      "key": {
        "type": "remote_addr"
      },
      "maxKeys": 5000,
      "inFlightLimit": 1000,
      "backlogLimit": 2000,
      "backlogTimeout": "5s",
      "responseStatusCode": 503
    }
  },
  "rules": [
    {
      "routes": [
        { "path": "/api/2/users" },
        { "path": "/api/2/tenants" }
      ],
      "excludedRoutes": [
        { "path": "/api/2/users/42" },
        { "path": "/api/2/tenants/42" }
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
      "routes": [
        { "path": "= /api/2/tenants", "methods": "POST, DELETE" },
        { "path": "= /api/2/users", "methods": ["POST", "DELETE", "PUT"] }
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
			Key:                ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
			MaxKeys:            10000,
			ResponseStatusCode: 429,
			DryRun:             true,
			ExcludedKeys:       []string{"2801c8de-7b41-4950-94e8-ad8fe8bd6d60", "7ab74f7c-846e-435f-96d4-5a0ce7068ddf"},
			IncludedKeys:       []string{},
		},
		Alg:                RateLimitAlgLeakyBucket,
		RateLimit:          RateLimitValue{Count: 50, Duration: time.Second},
		BurstLimit:         100,
		ResponseRetryAfter: RateLimitRetryAfterValue{Duration: time.Second * 15},
	}, cfg.RateLimitZones["rate_limit_identity"])
	require.Equal(t, RateLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:                ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
			MaxKeys:            10000,
			ResponseStatusCode: 429,
			DryRun:             true,
			ExcludedKeys:       []string{},
			IncludedKeys:       []string{},
		},
		Alg:                RateLimitAlgSlidingWindow,
		RateLimit:          RateLimitValue{Count: 500, Duration: time.Minute},
		ResponseRetryAfter: RateLimitRetryAfterValue{Duration: time.Second * 10},
	}, cfg.RateLimitZones["rate_limit_identity_window"])
	require.Equal(t, RateLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:                ZoneKeyConfig{Type: ""},
			MaxKeys:            0,
			ResponseStatusCode: 503,
			DryRun:             false,
			ExcludedKeys:       []string{},
			IncludedKeys:       []string{},
		},
		RateLimit:          RateLimitValue{Count: 6000, Duration: time.Minute},
		BurstLimit:         1000,
		BacklogLimit:       100,
		BacklogTimeout:     config.TimeDuration(time.Second * 30),
		ResponseRetryAfter: RateLimitRetryAfterValue{IsAuto: true},
	}, cfg.RateLimitZones["rate_limit_total"])

	// Check inFlightLimitZones
	require.Len(t, cfg.InFlightLimitZones, 4)
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:                ZoneKeyConfig{Type: ZoneKeyTypeIdentity},
			MaxKeys:            10000,
			ResponseStatusCode: 429,
			DryRun:             true,
			ExcludedKeys:       []string{},
			IncludedKeys:       []string{"7ab74f7c-846e-435f-96d4-5a0ce7068ddf", "2801c8de-7b41-4950-94e8-ad8fe8bd6d60"},
		},
		InFlightLimit:      32,
		BacklogLimit:       64,
		BacklogTimeout:     config.TimeDuration(time.Second * 5),
		ResponseRetryAfter: config.TimeDuration(time.Second * 30),
	}, cfg.InFlightLimitZones["in_flight_limit_identity"])
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:                ZoneKeyConfig{Type: ""},
			ResponseStatusCode: 503,
			DryRun:             false,
			ExcludedKeys:       []string{},
			IncludedKeys:       []string{},
		},
		InFlightLimit:      5000,
		BacklogLimit:       10000,
		BacklogTimeout:     config.TimeDuration(time.Second * 15),
		ResponseRetryAfter: config.TimeDuration(0),
	}, cfg.InFlightLimitZones["in_flight_limit_total"])
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:                ZoneKeyConfig{Type: ZoneKeyTypeHTTPHeader, HeaderName: "X-Tenant-ID"},
			MaxKeys:            5000,
			ResponseStatusCode: 429,
			DryRun:             true,
			ExcludedKeys:       []string{},
			IncludedKeys:       []string{},
		},
		InFlightLimit:      16,
		BacklogLimit:       24,
		BacklogTimeout:     config.TimeDuration(time.Second * 5),
		ResponseRetryAfter: config.TimeDuration(0),
	}, cfg.InFlightLimitZones["in_flight_limit_tenant"])
	require.Equal(t, InFlightLimitZoneConfig{
		ZoneConfig: ZoneConfig{
			Key:                ZoneKeyConfig{Type: ZoneKeyTypeRemoteAddr},
			MaxKeys:            5000,
			ResponseStatusCode: 503,
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
			Routes: []restapi.RouteConfig{
				{Path: mustParseRoutePath("/api/2/users")},
				{Path: mustParseRoutePath("/api/2/tenants")},
			},
			ExcludedRoutes: []restapi.RouteConfig{
				{Path: mustParseRoutePath("/api/2/users/42")},
				{Path: mustParseRoutePath("/api/2/tenants/42")},
			},
			RateLimits:     []RuleRateLimit{{Zone: "rate_limit_identity"}, {Zone: "rate_limit_identity_window"}},
			InFlightLimits: []RuleInFlightLimit{{Zone: "in_flight_limit_identity"}, {Zone: "in_flight_limit_remote_addr"}},
			Tags:           []string{"tag1", "tag2"},
		},
		{
			Alias: "limit_batches",
			Routes: []restapi.RouteConfig{
				{Path: mustParseRoutePath("= /api/2/tenants"), Methods: []string{"POST", "DELETE"}},
				{Path: mustParseRoutePath("= /api/2/users"), Methods: []string{"POST", "DELETE", "PUT"}},
			},
			RateLimits:     []RuleRateLimit{{Zone: "rate_limit_total"}},
			InFlightLimits: []RuleInFlightLimit{{Zone: "in_flight_limit_total"}},
			Tags:           []string{"tag_a", "tag_b"},
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

func TestConfig_Set_WithErrors(t *testing.T) {
	tests := []struct {
		Name             string
		CfgData          string
		WantErrStr       string
		WantErrStrSuffix string
	}{
		{
			Name: "invalid in-flight limit",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    inFlightLimit: 0
rules:
  - routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: ifl_zone
`,
			WantErrStr: `validate in-flight limit zone "ifl_zone": in-flight limit should be >= 1, got 0`,
		},
		{
			Name: "invalid backlog limit",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    inFlightLimit: 1
    backlogLimit: -1
rules:
  - routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: ifl_zone
`,
			WantErrStr: `validate in-flight limit zone "ifl_zone": backlog limit should be >= 0, got -1`,
		},
		{
			Name: "unknown key zone type",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    key:
      type: foobar
    inFlightLimit: 1
rules:
  - routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: ifl_zone
`,
			WantErrStr: `validate in-flight limit zone "ifl_zone": unknown key zone type "foobar"`,
		},
		{
			Name: "empty key zone header name",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    key:
      type: header
    inFlightLimit: 1
rules:
  - routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: ifl_zone
`,
			WantErrStr: `validate in-flight limit zone "ifl_zone": header name should be specified for "header" key zone type`,
		},
		{
			Name: "negative max keys",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    key:
      type: identity
    maxKeys: -1
    inFlightLimit: 1
rules:
  - routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: ifl_zone
`,
			WantErrStr: `validate in-flight limit zone "ifl_zone": maximum keys should be >= 0, got -1`,
		},
		{
			Name: "included and excluded keys cannot be specified at the same time",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    key:
      type: identity
    includedKeys: ["foo"]
    excludedKeys: ["bar"]
    inFlightLimit: 1
rules:
  - routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: ifl_zone
`,
			WantErrStr: `validate in-flight limit zone "ifl_zone": included and excluded lists cannot be specified at the same time`,
		},
		{
			Name: "undefined in-flight limit zone",
			CfgData: `
inFlightLimitZones:
  ifl_zone:
    inFlightLimit: 1
rules:
  - alias: aaa-in-flight-limiting
    routes:
    - path: "/aaa"
    inFlightLimits:
      - zone: mega_zone
`,
			WantErrStr: `validate rule "aaa-in-flight-limiting": in-flight limit zone "mega_zone" is undefined`,
		},
		{
			Name: "unknown rate limit alg",
			CfgData: `
rateLimitZones:
  rl_zone:
    alg: quick_sort
    rateLimit: 1/s
rules:
  - routes:
    - path: "/aaa"
    rateLimits:
      - zone: rl_zone
`,
			WantErrStrSuffix: `unknown rate limit alg "quick_sort"`,
		},
		{
			Name: "invalid rate limit format",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/f
rules:
  - routes:
    - path: "/aaa"
    rateLimits:
      - zone: rl_zone
`,
			WantErrStrSuffix: `incorrect format for rate "1/f", should be N/(s|m|h), for example 10/s, 100/m, 1000/h`,
		},
		{
			Name: "invalid rate limit",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 0/s
rules:
  - routes:
    - path: "/aaa"
    rateLimits:
      - zone: rl_zone
`,
			WantErrStr: `validate rate limit zone "rl_zone": rate limit should be >= 1, got 0`,
		},
		{
			Name: "invalid burst limit",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 10/s
    burstLimit: -1
rules:
  - routes:
    - path: "/aaa"
    rateLimits:
      - zone: rl_zone
`,
			WantErrStr: `validate rate limit zone "rl_zone": burst limit should be >= 0, got -1`,
		},
		{
			Name: "invalid backlog limit",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 10/s
    backlogLimit: -1
rules:
  - routes:
    - path: "/aaa"
    rateLimits:
      - zone: rl_zone
`,
			WantErrStr: `validate rate limit zone "rl_zone": backlog limit should be >= 0, got -1`,
		},
		{
			Name: "undefined rate limit zone",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/s
rules:
  - alias: aaa-rate-limiting
    routes:
    - path: "/aaa"
    rateLimits:
      - zone: mega_zone
`,
			WantErrStr: `validate rule "aaa-rate-limiting": rate limit zone "mega_zone" is undefined`,
		},
		{
			Name: "routes is missing",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/s
rules:
  - alias: aaa-rate-limiting
    rateLimits:
      - zone: rl_zone
`,
			WantErrStr: `validate rule "aaa-rate-limiting": routes is missing`,
		},
		{
			Name: "path is missing",
			CfgData: `
rateLimitZones:
  rl_zone:
    rateLimit: 1/s
rules:
  - alias: aaa-rate-limiting
    routes:
    - methods: POST,PUT,DELETE
    rateLimits:
      - zone: rl_zone
`,
			WantErrStr: `validate rule "aaa-rate-limiting": validate route #1: path is missing`,
		},
	}
	configLoader := config.NewLoader(config.NewViperAdapter())
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			cfg := &Config{}
			err := configLoader.LoadFromReader(bytes.NewReader([]byte(tt.CfgData)), config.DataTypeYAML, cfg)
			if tt.WantErrStr != "" {
				require.EqualError(t, err, tt.WantErrStr)
			} else {
				require.Error(t, err)
				require.True(t, strings.HasSuffix(err.Error(), tt.WantErrStrSuffix),
					"want error %q, got %q", err.Error(), tt.WantErrStrSuffix)
			}
		})
	}
}

func TestRuleConfig_Name(t *testing.T) {
	tests := []struct {
		Name         string
		Rule         RuleConfig
		WantRuleName string
	}{
		{
			Name: "alias",
			Rule: RuleConfig{Alias: "my-rule", Routes: []restapi.RouteConfig{
				{Path: mustParseRoutePath("= /bbb"), Methods: []string{"GET", "POST"}},
			}},
			WantRuleName: "my-rule",
		},
		{
			Name: "no alias, single route",
			Rule: RuleConfig{Routes: []restapi.RouteConfig{
				{Path: mustParseRoutePath("= /bbb"), Methods: []string{"GET", "POST"}},
			}},
			WantRuleName: "GET|POST = /bbb",
		},
		{
			Name: "no alias, multiple routes",
			Rule: RuleConfig{Routes: []restapi.RouteConfig{
				{Path: mustParseRoutePath("/aaa")},
				{Path: mustParseRoutePath("= /bbb"), Methods: []string{"GET", "POST"}},
				{Path: mustParseRoutePath("/ccc"), Methods: []string{"POST", "PUT"}},
			}},
			WantRuleName: "/aaa; GET|POST = /bbb; POST|PUT /ccc",
		},
	}
	for i := range tests {
		tt := tests[i]
		t.Run(tt.Name, func(t *testing.T) {
			require.Equal(t, tt.WantRuleName, tt.Rule.Name())
		})
	}
}

func mustParseRoutePath(s string) restapi.RoutePath {
	rp, err := restapi.ParseRoutePath(s)
	if err != nil {
		panic(err)
	}
	return rp
}

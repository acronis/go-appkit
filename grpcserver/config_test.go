/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package grpcserver

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/suite"
	"gopkg.in/yaml.v3"

	"github.com/acronis/go-appkit/config"
)

type ConfigTestSuite struct {
	suite.Suite
}

func TestConfig(t *testing.T) {
	suite.Run(t, new(ConfigTestSuite))
}

type AppConfig struct {
	GRPCServer *Config `mapstructure:"grpcServer" json:"grpcServer" yaml:"grpcServer"`
}

func (s *ConfigTestSuite) TestConfig() {
	tests := []struct {
		name        string
		cfgDataType config.DataType
		cfgData     string
		expectedCfg func() *Config
	}{
		{
			name:        "yaml config",
			cfgDataType: config.DataTypeYAML,
			cfgData: `
grpcServer:
  address: "127.0.0.1:9090"
  unixSocketPath: "/tmp/grpc.sock"
  timeouts:
    shutdown: 30s
  keepalive:
    time: 5m
    timeout: 1m
    minTime: 30s
  limits:
    maxConcurrentStreams: 100
    maxRecvMessageSize: 8M
    maxSendMessageSize: 8M
  log:
    callStart: true
    excludedMethods:
      - "grpc.health.v1.Health/Check"
      - "grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"
    slowCallThreshold: 2s
  tls:
    enabled: true
    cert: "/test/cert.pem"
    key: "/test/key.pem"
`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Address = "127.0.0.1:9090"
				cfg.UnixSocketPath = "/tmp/grpc.sock"
				cfg.Timeouts.Shutdown = config.TimeDuration(30 * time.Second)
				cfg.Keepalive.Time = config.TimeDuration(5 * time.Minute)
				cfg.Keepalive.Timeout = config.TimeDuration(1 * time.Minute)
				cfg.Keepalive.MinTime = config.TimeDuration(30 * time.Second)
				cfg.Limits.MaxConcurrentStreams = 100
				cfg.Limits.MaxRecvMessageSize = config.ByteSize(8 * 1024 * 1024)
				cfg.Limits.MaxSendMessageSize = config.ByteSize(8 * 1024 * 1024)
				cfg.Log.CallStart = true
				cfg.Log.ExcludedMethods = []string{
					"grpc.health.v1.Health/Check",
					"grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
				}
				cfg.Log.SlowCallThreshold = config.TimeDuration(2 * time.Second)
				cfg.TLS.Enabled = true
				cfg.TLS.Certificate = "/test/cert.pem"
				cfg.TLS.Key = "/test/key.pem"
				return cfg
			},
		},
		{
			name:        "json config",
			cfgDataType: config.DataTypeJSON,
			cfgData: `
{
	"grpcServer": {
		"address": "127.0.0.1:9090",
		"unixSocketPath": "/tmp/grpc.sock",
		"timeouts": {
			"shutdown": "30s"
		},
		"keepalive": {
			"time": "5m",
			"timeout": "1m",
			"minTime": "30s"
		},
		"limits": {
			"maxConcurrentStreams": 100,
			"maxRecvMessageSize": "8M",
			"maxSendMessageSize": "8M"
		},
		"log": {
			"callStart": true,
			"excludedMethods": [
				"grpc.health.v1.Health/Check",
				"grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo"
			],
			"slowCallThreshold": "2s"
		},
		"tls": {
			"enabled": true,
			"cert": "/test/cert.pem",
			"key": "/test/key.pem"
		}
	}
}`,
			expectedCfg: func() *Config {
				cfg := NewDefaultConfig()
				cfg.Address = "127.0.0.1:9090"
				cfg.UnixSocketPath = "/tmp/grpc.sock"
				cfg.Timeouts.Shutdown = config.TimeDuration(30 * time.Second)
				cfg.Keepalive.Time = config.TimeDuration(5 * time.Minute)
				cfg.Keepalive.Timeout = config.TimeDuration(1 * time.Minute)
				cfg.Keepalive.MinTime = config.TimeDuration(30 * time.Second)
				cfg.Limits.MaxConcurrentStreams = 100
				cfg.Limits.MaxRecvMessageSize = config.ByteSize(8 * 1024 * 1024)
				cfg.Limits.MaxSendMessageSize = config.ByteSize(8 * 1024 * 1024)
				cfg.Log.CallStart = true
				cfg.Log.ExcludedMethods = []string{
					"grpc.health.v1.Health/Check",
					"grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
				}
				cfg.Log.SlowCallThreshold = config.TimeDuration(2 * time.Second)
				cfg.TLS.Enabled = true
				cfg.TLS.Certificate = "/test/cert.pem"
				cfg.TLS.Key = "/test/key.pem"
				return cfg
			},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			// Load config using config.Loader.
			appCfg := AppConfig{GRPCServer: NewDefaultConfig()}
			expectedAppCfg := AppConfig{GRPCServer: tt.expectedCfg()}
			cfgLoader := config.NewLoader(config.NewViperAdapter())
			err := cfgLoader.LoadFromReader(bytes.NewBuffer([]byte(tt.cfgData)), tt.cfgDataType, appCfg.GRPCServer)
			s.Require().NoError(err)
			s.Require().Equal(expectedAppCfg, appCfg)

			// Load config using viper unmarshal.
			appCfg = AppConfig{GRPCServer: NewDefaultConfig()}
			expectedAppCfg = AppConfig{GRPCServer: tt.expectedCfg()}
			vpr := viper.New()
			vpr.SetConfigType(string(tt.cfgDataType))
			s.Require().NoError(vpr.ReadConfig(bytes.NewBuffer([]byte(tt.cfgData))))
			s.Require().NoError(vpr.Unmarshal(&appCfg, func(c *mapstructure.DecoderConfig) {
				c.DecodeHook = mapstructure.TextUnmarshallerHookFunc()
			}))
			s.Require().Equal(expectedAppCfg, appCfg)

			// Load config using yaml/json unmarshal.
			appCfg = AppConfig{GRPCServer: NewDefaultConfig()}
			expectedAppCfg = AppConfig{GRPCServer: tt.expectedCfg()}
			switch tt.cfgDataType {
			case config.DataTypeYAML:
				s.Require().NoError(yaml.Unmarshal([]byte(tt.cfgData), &appCfg))
				s.Require().Equal(expectedAppCfg, appCfg)
			case config.DataTypeJSON:
				s.Require().NoError(json.Unmarshal([]byte(tt.cfgData), &appCfg))
				s.Require().Equal(expectedAppCfg, appCfg)
			default:
				s.T().Fatalf("unsupported config data type: %s", tt.cfgDataType)
			}
		})
	}
}

func (s *ConfigTestSuite) TestNewDefaultConfig() {
	var cfg *Config

	// Empty config, all defaults for the data provider should be used
	cfg = NewConfig()
	s.Require().NoError(config.NewDefaultLoader("").LoadFromReader(bytes.NewBuffer(nil), config.DataTypeYAML, cfg))
	s.Require().Equal(NewDefaultConfig(), cfg)

	// viper.Unmarshal
	cfg = NewDefaultConfig()
	vpr := viper.New()
	vpr.SetConfigType("yaml")
	s.Require().NoError(vpr.Unmarshal(&cfg))
	s.Require().Equal(NewDefaultConfig(), cfg)

	// yaml.Unmarshal
	cfg = NewDefaultConfig()
	s.Require().NoError(yaml.Unmarshal([]byte(""), &cfg))
	s.Require().Equal(NewDefaultConfig(), cfg)

	// json.Unmarshal
	cfg = NewDefaultConfig()
	s.Require().NoError(json.Unmarshal([]byte("{}"), &cfg))
	s.Require().Equal(NewDefaultConfig(), cfg)
}

func (s *ConfigTestSuite) TestWithKeyPrefix() {
	s.Run("custom key prefix", func() {
		cfgData := `
customGRPCServer:
  address: "127.0.0.1:9999"
`
		expectedCfg := NewDefaultConfig(WithKeyPrefix("customGRPCServer"))
		expectedCfg.Address = "127.0.0.1:9999"

		cfg := NewConfig(WithKeyPrefix("customGRPCServer"))
		err := config.NewLoader(config.NewViperAdapter()).LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
		s.Require().NoError(err)
		s.Require().Equal(expectedCfg, cfg)
	})

	s.Run("default key prefix, empty struct initialization", func() {
		cfgData := `
grpcServer:
  address: "127.0.0.1:9999"
`
		cfg := &Config{}
		err := config.NewLoader(config.NewViperAdapter()).LoadFromReader(bytes.NewBuffer([]byte(cfgData)), config.DataTypeYAML, cfg)
		s.Require().NoError(err)
		s.Require().Equal("127.0.0.1:9999", cfg.Address)
	})
}

func (s *ConfigTestSuite) TestConfigValidationErrors() {
	tests := []struct {
		name           string
		yamlData       string
		expectedErrMsg string
	}{
		{
			name: "error, invalid address",
			yamlData: `
grpcServer:
  address: []
`,
			expectedErrMsg: `grpcServer.address: unable to cast`,
		},
		{
			name: "error, invalid timeout duration",
			yamlData: `
grpcServer:
  timeouts:
    shutdown: "invalid-duration"
`,
			expectedErrMsg: `grpcServer.timeouts.shutdown: time: invalid duration`,
		},
		{
			name: "error, negative maxConcurrentStreams",
			yamlData: `
grpcServer:
  limits:
    maxConcurrentStreams: -1
`,
			expectedErrMsg: `grpcServer.limits.maxConcurrentStreams: cannot be negative`,
		},
		{
			name: "error, invalid byte size",
			yamlData: `
grpcServer:
  limits:
    maxRecvMessageSize: "invalid-size"
`,
			expectedErrMsg: `grpcServer.limits.maxRecvMessageSize`,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			cfg := NewConfig()
			err := config.NewLoader(config.NewViperAdapter()).LoadFromReader(bytes.NewBuffer([]byte(tt.yamlData)), config.DataTypeYAML, cfg)
			s.Require().ErrorContains(err, tt.expectedErrMsg)
		})
	}
}

func (s *ConfigTestSuite) TestKeyPrefix() {
	cfg := NewConfig()
	s.Require().Equal(cfgDefaultKeyPrefix, cfg.KeyPrefix())

	cfg = NewConfig(WithKeyPrefix("custom"))
	s.Require().Equal("custom", cfg.KeyPrefix())

	// Test empty keyPrefix falls back to default
	cfg = &Config{keyPrefix: ""}
	s.Require().Equal(cfgDefaultKeyPrefix, cfg.KeyPrefix())
}

func (s *ConfigTestSuite) TestNewConfig() {
	cfg := NewConfig()
	s.Require().NotNil(cfg)
	s.Require().Equal(cfgDefaultKeyPrefix, cfg.keyPrefix)

	cfg = NewConfig(WithKeyPrefix("custom"))
	s.Require().NotNil(cfg)
	s.Require().Equal("custom", cfg.keyPrefix)
}

func (s *ConfigTestSuite) TestNewDefaultConfigValues() {
	cfg := NewDefaultConfig()

	s.Require().Equal(defaultServerAddress, cfg.Address)
	s.Require().Equal(config.TimeDuration(defaultServerShutdownTimeout), cfg.Timeouts.Shutdown)
	s.Require().Equal(config.TimeDuration(defaultServerKeepaliveTime), cfg.Keepalive.Time)
	s.Require().Equal(config.TimeDuration(defaultServerKeepaliveTimeout), cfg.Keepalive.Timeout)
	s.Require().Equal(config.ByteSize(defaultServerMaxRecvMessageSize), cfg.Limits.MaxRecvMessageSize)
	s.Require().Equal(config.ByteSize(defaultServerMaxSendMessageSize), cfg.Limits.MaxSendMessageSize)
	s.Require().Equal(config.TimeDuration(defaultSlowCallThreshold), cfg.Log.SlowCallThreshold)

	// Test with custom key prefix
	cfg = NewDefaultConfig(WithKeyPrefix("custom"))
	s.Require().Equal("custom", cfg.keyPrefix)
	s.Require().Equal(defaultServerAddress, cfg.Address)
}

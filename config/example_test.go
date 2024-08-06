/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path"

	"code.cloudfoundry.org/bytefmt"
)

const (
	cfgKeyServerAddr                = "server.addr"
	cfgKeyLogLevel                  = "log.level"
	cfgKeyLogFilePath               = "log.file.path"
	cfgKeyLogFileRotationCompress   = "log.file.rotation.compress"
	cfgKeyLogFileRotationMaxSize    = "log.file.rotation.maxsize"
	cfgKeyLogFileRotationMaxBackups = "log.file.rotation.maxbackups"
)

type serverConfig struct {
	Addr string
}

func (c *serverConfig) UpdateProviderValues(dp DataProvider) {
	dp.Set(cfgKeyServerAddr, c.Addr)
}

func (c *serverConfig) SetProviderDefaults(dp DataProvider) {
	dp.SetDefault(cfgKeyServerAddr, ":8080")
}

func (c *serverConfig) Set(dp DataProvider) error {
	var err error
	if c.Addr, err = dp.GetString(cfgKeyServerAddr); err != nil {
		return err
	}
	return nil
}

type logConfig struct {
	Level string
	File  struct {
		Path     string
		Rotation struct {
			MaxSize    uint64
			MaxBackups int
			Compress   bool
		}
	}
}

func (c *logConfig) SetProviderDefaults(dp DataProvider) {
	dp.SetDefault(cfgKeyLogLevel, "info")
	dp.SetDefault(cfgKeyLogFileRotationCompress, false)
	dp.SetDefault(cfgKeyLogFileRotationMaxSize, bytefmt.ByteSize(100*1024*1024))
	dp.SetDefault(cfgKeyLogFileRotationMaxBackups, 10)
}

func (c *logConfig) Set(dp DataProvider) error {
	var err error

	if c.Level, err = dp.GetStringFromSet(cfgKeyLogLevel, []string{"debug", "info", "warn", "error"}, true); err != nil {
		return err
	}

	if c.File.Path, err = dp.GetString(cfgKeyLogFilePath); err != nil {
		return err
	}
	if c.File.Path == "" {
		return WrapKeyErr(cfgKeyLogFilePath, fmt.Errorf("must not be empty"))
	}

	if c.File.Rotation.MaxSize, err = dp.GetSizeInBytes(cfgKeyLogFileRotationMaxSize); err != nil {
		return err
	}
	if c.File.Rotation.MaxBackups, err = dp.GetInt(cfgKeyLogFileRotationMaxBackups); err != nil {
		return err
	}
	if c.File.Rotation.Compress, err = dp.GetBool(cfgKeyLogFileRotationCompress); err != nil {
		return err
	}

	return nil
}

func Example() {
	const envVarsPrefix = "my_service"

	cfgData := bytes.NewBuffer([]byte(`
log:
  level: info
  file:
    path: my-service.log
    rotation:
      maxsize: 100M
      maxbackups: 10
      compress: false
`))

	// Override some configuration values using environment variables.
	if err := os.Setenv("MY_SERVICE_LOG_FILE_ROTATION_COMPRESS", "true"); err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("MY_SERVICE_LOG_LEVEL", "debug"); err != nil {
		log.Fatal(err)
	}

	serverCfg := serverConfig{}
	logCfg := logConfig{}

	// Load configuration values and set them in serverCfg and logCfg.
	cfgLoader := NewDefaultLoader(envVarsPrefix)
	err := cfgLoader.LoadFromReader(cfgData, DataTypeYAML, &serverCfg, &logCfg) // Use cfgLoader.LoadFromFile() to read from file.
	if err != nil {
		log.Fatal(err)
	}

	// Save a modified config's copy into a file
	fname := path.Join(os.TempDir(), "data.yaml")
	configToModify := serverCfg
	configToModify.Addr = "new.address.com:8888"
	dp := cfgLoader.DataProvider
	UpdateDataProvider(dp, &configToModify)
	err = dp.SaveToFile(fname, DataTypeYAML)
	if err != nil {
		log.Fatal(err)
	}

	// Load config from file
	configFromFile := serverConfig{}
	modifiedConfigLoader := NewDefaultLoader(envVarsPrefix)
	err = modifiedConfigLoader.LoadFromFile(fname, DataTypeYAML, &configFromFile)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(serverCfg.Addr)
	fmt.Printf("%q, %q, %d, %d, %v\n", logCfg.Level, logCfg.File.Path, logCfg.File.Rotation.MaxSize,
		logCfg.File.Rotation.MaxBackups, logCfg.File.Rotation.Compress)
	fmt.Println(configFromFile.Addr)

	// Output:
	// :8080
	// "debug", "my-service.log", 104857600, 10, true
	// new.address.com:8888
}

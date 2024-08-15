/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package log

import (
	"bytes"
	"log"
	"os"
	"time"

	"github.com/acronis/go-appkit/config"
)

/*
Add "// Output:" in the end of Example() function and run:

	$ go test ./log -v -run Example
*/

func Example() {
	cfgData := bytes.NewBuffer([]byte(`
log:
  level: info
  output: file
  file:
    path: my-service-{{starttime}}-{{pid}}.log
    rotation:
      maxsize: 100M
      maxbackups: 10
      compress: false
  error:
    verbosesuffix: _verbose
`))

	cfg := Config{}
	cfgLoader := config.NewLoader(config.NewViperAdapter())
	err := cfgLoader.LoadFromReader(cfgData, config.DataTypeYAML, &cfg) // Use cfgLoader.LoadFromFile() to read from file.
	if err != nil {
		log.Fatal(err)
	}

	logger, cancel := NewLogger(&cfg)
	defer cancel()

	logger = logger.With(Int("pid", os.Getpid()))
	logger.Info("request served", String("request-id", "generatedrequestid"), DurationIn(time.Second, time.Millisecond))
}

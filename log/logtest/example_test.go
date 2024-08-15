/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package logtest

import (
	"fmt"

	"github.com/acronis/go-appkit/log"
)

func Example() {
	f := func(a, b int, logger log.FieldLogger) {
		logger.Info("calculation", log.Int("sum", a+b), log.String("hello", "world"))
	}

	logRecorder := NewRecorder()
	f(40, 2, logRecorder)

	// In real tests we can check that message with right fields were properly logged.

	if logEntry, found := logRecorder.FindEntry("calculation"); found {
		fmt.Printf("[%s] %s\n", logEntry.Level, logEntry.Text)
		if logFieldSum, found := logEntry.FindField("sum"); found {
			fmt.Printf("sum: %d\n", logFieldSum.Int)
		}
		if logFieldHello, found := logEntry.FindField("hello"); found {
			fmt.Printf("hello: %s\n", logFieldHello.Bytes)
		}
	}

	// Output:
	// [info] calculation
	// sum: 42
	// hello: world
}

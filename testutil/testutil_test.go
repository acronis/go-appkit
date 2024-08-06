/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

type MockT struct {
	Failed bool
	Format string
	Args   []interface{}
}

func (t *MockT) FailNow() {
	t.Failed = true
}

func (t *MockT) Errorf(format string, args ...interface{}) {
	t.Format, t.Args = format, args
}

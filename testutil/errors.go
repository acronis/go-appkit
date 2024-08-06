/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"errors"
	"fmt"
	"strings"

	"github.com/stretchr/testify/require"
)

// RequireNoErrorInChannel asserts that there is an error in buffered channel.
func RequireNoErrorInChannel(t require.TestingT, c <-chan error, msgAndArgs ...interface{}) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	var err error
	select {
	case err = <-c:
	default:
	}
	require.NoError(t, err, msgAndArgs...)
}

// RequireErrorIsAny asserts that at least one of the errors in err's chain matches at least one target.
// This is a wrapper for errors.Is.
func RequireErrorIsAny(t require.TestingT, err error, targets []error, msgAndArgs ...interface{}) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	for _, targetErr := range targets {
		if errors.Is(err, targetErr) {
			return
		}
	}
	var expectedErrTexts []string
	for _, targetErr := range targets {
		expectedErrTexts = append(expectedErrTexts, fmt.Sprintf("%q", targetErr.Error()))
	}
	require.FailNow(t, fmt.Sprintf("At least one target error should be in err chain:\n"+
		"expected: [%s]\n"+
		"in chain: %s", strings.Join(expectedErrTexts, "; "), buildErrorChainString(err),
	), msgAndArgs...)
}

func buildErrorChainString(err error) string {
	if err == nil {
		return ""
	}

	e := errors.Unwrap(err)
	chain := fmt.Sprintf("%q", err.Error())
	for e != nil {
		chain += fmt.Sprintf("\n\t%q", e.Error())
		e = errors.Unwrap(e)
	}
	return chain
}

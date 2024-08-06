/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package config

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrapKeyErrIfNeeded(t *testing.T) {
	t.Run("wrap nil", func(t *testing.T) {
		assert.Nil(t, WrapKeyErrIfNeeded("log.output", nil), "nil should not be wrapped")
	})

	t.Run("wrap error", func(t *testing.T) {
		const key = "log.output"
		errInvalidOutput := errors.New("invalid output")
		gotErr := WrapKeyErrIfNeeded(key, errInvalidOutput)
		wantErrMsg := fmt.Sprintf("%s: %v", key, errInvalidOutput)
		assert.EqualError(t, gotErr, wantErrMsg, "texts of errors should be equal")
		assert.Equal(t, errInvalidOutput, errors.Unwrap(gotErr), "original error should be wrapped")
	})
}

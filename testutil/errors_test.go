/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequireErrorIsAny(t *testing.T) {
	targetErrs := []error{
		errors.New("error A"),
		errors.New("error B"),
		errors.New("error C"),
	}

	mockT := &MockT{}

	RequireErrorIsAny(mockT, fmt.Errorf("do something: %w", targetErrs[1]), targetErrs)
	require.False(t, mockT.Failed)

	RequireErrorIsAny(mockT, fmt.Errorf("do something: %w", errors.New("error D")), targetErrs)
	require.True(t, mockT.Failed)

	RequireErrorIsAny(mockT, nil, targetErrs)
	require.True(t, mockT.Failed)
}

func TestRequireNoErrorInChannel(t *testing.T) {
	mockT := &MockT{}
	ch := make(chan error, 1)

	RequireNoErrorInChannel(mockT, ch)
	require.False(t, mockT.Failed)

	ch <- nil
	RequireNoErrorInChannel(mockT, ch)
	require.False(t, mockT.Failed)

	ch <- errors.New("some error")
	RequireNoErrorInChannel(mockT, ch)
	require.True(t, mockT.Failed)
}

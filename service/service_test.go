/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/acronis/go-appkit/log/logtest"
)

func TestService_Start(t *testing.T) {
	logRecorder := logtest.NewRecorder()
	var runningCounter int32
	mockUnit := newMockUnit("srv", &runningCounter, false)
	service := New(logRecorder, mockUnit)
	go func() {
		require.NoError(t, service.Start())
	}()
	require.NoError(t, waitTrue(func() bool { return atomic.LoadInt32(&runningCounter) == 1 }, time.Second*3))
	require.Equal(t, 1, mockUnit.mustRegisterMetricsCalled)
	require.Equal(t, 1, mockUnit.startCalled)

	service.Signals <- os.Interrupt // Sending SIGINT signal to the service.

	require.NoError(t, waitTrue(func() bool { return atomic.LoadInt32(&runningCounter) == 0 }, time.Second*3))
	require.Equal(t, 1, mockUnit.unregisterMetricsCalled)
	require.Equal(t, 1, mockUnit.stopCalled)
	require.Equal(t, 1, mockUnit.stopGracefullyCalled)
}

func TestService_StartContext(t *testing.T) {
	ctx, ctxCancel := context.WithCancel(context.Background())

	logRecorder := logtest.NewRecorder()
	var runningCounter int32
	mockUnit := newMockUnit("srv", &runningCounter, false)
	service := New(logRecorder, mockUnit)
	go func() {
		require.NoError(t, service.StartContext(ctx))
	}()
	require.NoError(t, waitTrue(func() bool { return atomic.LoadInt32(&runningCounter) == 1 }, time.Second*3))

	ctxCancel()

	require.NoError(t, waitTrue(func() bool { return atomic.LoadInt32(&runningCounter) == 0 }, time.Second*3))
	require.Equal(t, 1, mockUnit.stopGracefullyCalled)
}

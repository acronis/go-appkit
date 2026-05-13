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

// controlledUnit allows tests to precisely orchestrate the lifecycle of Start/Stop.
type controlledUnit struct {
	startEntered chan struct{}
	releaseStart chan struct{}
	stopCalled   chan struct{}
}

func newControlledUnit() *controlledUnit {
	return &controlledUnit{
		startEntered: make(chan struct{}),
		releaseStart: make(chan struct{}),
		stopCalled:   make(chan struct{}),
	}
}

func (u *controlledUnit) Start(_ chan<- error) {
	close(u.startEntered)
	<-u.releaseStart
}

func (u *controlledUnit) Stop(_ bool) error {
	close(u.stopCalled)
	return nil
}

func TestService_StartContext_StopTimeout_WaitsForStartToExit(t *testing.T) {
	logRecorder := logtest.NewRecorder()
	unit := newControlledUnit()
	svc := NewWithOpts(logRecorder, unit, Opts{StopTimeout: 5 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	returned := make(chan error, 1)
	go func() {
		returned <- svc.StartContext(ctx)
	}()

	<-unit.startEntered
	cancel()
	<-unit.stopCalled

	// Start is still blocked — StartContext must NOT have returned yet.
	select {
	case err := <-returned:
		t.Fatalf("StartContext returned before Unit.Start exited: err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(unit.releaseStart)

	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("StartContext did not return after Unit.Start exited")
	}
}

func TestService_StartContext_StopTimeout_ExceededReturns(t *testing.T) {
	logRecorder := logtest.NewRecorder()
	unit := newControlledUnit()
	svc := NewWithOpts(logRecorder, unit, Opts{StopTimeout: 100 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	returned := make(chan error, 1)
	go func() {
		returned <- svc.StartContext(ctx)
	}()

	<-unit.startEntered
	cancel()

	startTime := time.Now()
	select {
	case err := <-returned:
		require.NoError(t, err)
		elapsed := time.Since(startTime)
		require.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
		require.Less(t, elapsed, time.Second)
	case <-time.After(time.Second):
		t.Fatal("StartContext did not return within StopTimeout")
	}

	// Release Start to avoid leaking the goroutine across tests.
	close(unit.releaseStart)
}

func TestService_StartContext_ZeroStopTimeout_DoesNotWait(t *testing.T) {
	logRecorder := logtest.NewRecorder()
	unit := newControlledUnit()
	svc := New(logRecorder, unit) // StopTimeout = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	returned := make(chan error, 1)
	go func() {
		returned <- svc.StartContext(ctx)
	}()

	<-unit.startEntered
	cancel()

	// With StopTimeout=0, StartContext must return without waiting on Start.
	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("StartContext did not return with StopTimeout=0")
	}

	close(unit.releaseStart)
}

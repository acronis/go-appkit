/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"git.acronis.com/abc/go-libs/v2/log"
)

func TestWorkerUnit_Start_Stop(t *testing.T) {
	t.Run("start, stop no gracefully", func(t *testing.T) {
		var c atomic.Int32
		periodicWorker := NewPeriodicWorker(WorkerFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				time.Sleep(time.Second)
				c.Store(100)
				return nil
			default:
			}
			c.Inc()
			return nil
		}), time.Millisecond*100, log.NewDisabledLogger())

		unit := NewWorkerUnit(periodicWorker)
		fatalErr := make(chan error)
		go func() {
			unit.Start(fatalErr)
		}()
		time.Sleep(time.Millisecond * 450)
		require.NoError(t, unit.Stop(false))
		require.Equal(t, 5, int(c.Load()))
		close(fatalErr)
		require.NoError(t, <-fatalErr)
	})

	t.Run("start, stop gracefully with timeout", func(t *testing.T) {
		longRunningWorker := WorkerFunc(func(ctx context.Context) error {
			time.Sleep(time.Second * 3) // Emulate long blocking operation.
			return nil
		})
		unit := NewWorkerUnitWithOpts(longRunningWorker, WorkerUnitOpts{GracefulStopTimeout: time.Millisecond * 500})
		fatalErr := make(chan error)
		go func() {
			unit.Start(fatalErr)
		}()
		time.Sleep(time.Millisecond * 100)
		require.ErrorIs(t, unit.Stop(true), ErrWorkerUnitStopTimeoutExceeded)
		close(fatalErr)
		require.NoError(t, <-fatalErr)
	})

	t.Run("start, stop gracefully without timeout", func(t *testing.T) {
		longRunningWorkerAnswer := 0
		longRunningWorker := WorkerFunc(func(ctx context.Context) error {
			time.Sleep(time.Millisecond * 250)
			longRunningWorkerAnswer = 42
			return nil
		})
		unit := NewWorkerUnit(longRunningWorker)
		fatalErr := make(chan error)
		go func() {
			unit.Start(fatalErr)
		}()
		time.Sleep(time.Millisecond * 100)
		require.NoError(t, unit.Stop(true))
		require.Equal(t, 42, longRunningWorkerAnswer)
		close(fatalErr)
		require.NoError(t, <-fatalErr)
	})
}

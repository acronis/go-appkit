/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"git.acronis.com/abc/go-libs/v2/log"

	"github.com/stretchr/testify/require"
)

func TestPeriodicWorker_Run(t *testing.T) {
	t.Run("run and stop by context timeout", func(t *testing.T) {
		const iterations = 5

		c := 0
		periodicWorker := NewPeriodicWorker(WorkerFunc(func(ctx context.Context) error {
			c++
			return nil
		}), time.Millisecond*100, log.NewDisabledLogger())

		ctx, ctxCancel := context.WithTimeout(context.Background(), time.Millisecond*100*iterations)
		defer ctxCancel()

		runErr := make(chan error)
		go func() {
			runErr <- periodicWorker.Run(ctx)
		}()
		require.NoError(t, <-runErr)
		require.GreaterOrEqual(t, c, iterations)
		require.LessOrEqual(t,
			c, iterations+1) // it's possible that the last iteration will be executed after the context is canceled
		require.Error(t, context.DeadlineExceeded, ctx.Err())
	})

	t.Run("run and stop by error", func(t *testing.T) {
		c := 0
		periodicWorker := NewPeriodicWorker(WorkerFunc(func(ctx context.Context) error {
			c++
			if c == 2 {
				return ErrPeriodicWorkerStop
			}
			return nil
		}), time.Millisecond*100, log.NewDisabledLogger())
		ctx, ctxCancel := context.WithTimeout(context.Background(), time.Minute)
		defer ctxCancel()
		runErr := make(chan error)
		go func() {
			runErr <- periodicWorker.Run(ctx)
		}()
		require.Error(t, ErrPeriodicWorkerStop, <-runErr)
		require.Equal(t, 2, c)
		require.NoError(t, ctx.Err())
	})

	t.Run("run (initial delay > 0) and stop by context timeout", func(t *testing.T) {
		c := 0
		periodicWorker := NewPeriodicWorkerWithOpts(WorkerFunc(func(ctx context.Context) error {
			c++
			return nil
		}), time.Millisecond*100, log.NewDisabledLogger(), PeriodicWorkerOpts{InitialDelay: time.Millisecond * 250})

		ctx, ctxCancel := context.WithTimeout(context.Background(), time.Millisecond*500)
		defer ctxCancel()

		runErr := make(chan error)
		go func() {
			runErr <- periodicWorker.Run(ctx)
		}()
		require.NoError(t, <-runErr)
		require.Equal(t, 3, c)
		require.Error(t, ctx.Err(), context.DeadlineExceeded)
	})

	t.Run("run (fail delay > 0), got one error, and stop by context timeout", func(t *testing.T) {
		intervalDelayFunc := func(worker Worker, err error) time.Duration {
			if err != nil {
				return time.Millisecond * 250
			}
			return time.Millisecond * 100
		}
		c := 0
		periodicWorker := NewPeriodicWorkerWithOpts(WorkerFunc(func(ctx context.Context) error {
			c++
			if c == 1 {
				return fmt.Errorf("non-stop error")
			}
			return nil
		}), time.Millisecond*100, log.NewDisabledLogger(), PeriodicWorkerOpts{IntervalDelayFunc: intervalDelayFunc})

		ctx, ctxCancel := context.WithTimeout(context.Background(), time.Millisecond*500)
		defer ctxCancel()

		runErr := make(chan error)
		go func() {
			runErr <- periodicWorker.Run(ctx)
		}()
		require.NoError(t, <-runErr)
		require.Equal(t, 4, c)
		require.Error(t, ctx.Err(), context.DeadlineExceeded)
	})
}

/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package lrucache

import (
	"errors"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSingleFlight(t *testing.T) {
	t.Run("different keys", func(t *testing.T) {
		var sfGroup singleFlightGroup[string, int]
		var callCount int32

		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make([]int, numGoroutines)
		errs := make([]error, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(i int) {
				defer wg.Done()
				res, err := sfGroup.Do("key"+strconv.Itoa(i), func() (int, error) {
					atomic.AddInt32(&callCount, 1)
					time.Sleep(100 * time.Millisecond)
					return (i + 1) * 10, nil
				})
				results[i] = res
				errs[i] = err
			}(i)
		}
		wg.Wait()

		require.Equal(t, int32(numGoroutines), callCount, "expected fn to be called multiple times")
		for i, err := range errs {
			require.NoError(t, err, "goroutine %d: unexpected error", i)
			require.Equal(t, (i+1)*10, results[i], "goroutine %d: unexpected result", i)
		}
	})

	t.Run("same key", func(t *testing.T) {
		var sfGroup singleFlightGroup[string, int]
		var callCount int32

		fn := func() (int, error) {
			atomic.AddInt32(&callCount, 1)
			time.Sleep(100 * time.Millisecond)
			return 42, nil
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make([]int, numGoroutines)
		errs := make([]error, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(i int) {
				defer wg.Done()
				res, err := sfGroup.Do("key", fn)
				results[i] = res
				errs[i] = err
			}(i)
		}
		wg.Wait()

		require.Equal(t, int32(1), callCount, "expected fn to be called only once")
		for i, err := range errs {
			require.NoError(t, err, "goroutine %d: unexpected error", i)
			require.Equal(t, 42, results[i], "goroutine %d: unexpected result", i)
		}
	})

	t.Run("error is returned", func(t *testing.T) {
		var sfGroup singleFlightGroup[string, int]
		var callCount int32
		someErr := errors.New("some error")

		fn := func() (int, error) {
			atomic.AddInt32(&callCount, 1)
			time.Sleep(100 * time.Millisecond)
			return 0, someErr
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make([]int, numGoroutines)
		errs := make([]error, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(i int) {
				defer wg.Done()
				res, err := sfGroup.Do("key", fn)
				results[i] = res
				errs[i] = err
			}(i)
		}
		wg.Wait()

		require.Equal(t, int32(1), callCount, "expected fn to be called only once")
		for i, err := range errs {
			require.Error(t, err, "goroutine %d: expected an error", i)
			require.EqualError(t, err, someErr.Error(), "goroutine %d: unexpected error message", i)
		}
	})

	t.Run("panic", func(t *testing.T) {
		var sfGroup singleFlightGroup[string, int]
		var callCount int32
		panicValue := "boom"

		fn := func() (int, error) {
			atomic.AddInt32(&callCount, 1)
			time.Sleep(100 * time.Millisecond)
			panic(panicValue)
		}

		const numGoroutines = 10
		var wg sync.WaitGroup

		type result struct {
			panicked bool
			err      error
		}
		results := make([]result, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(i int) {
				defer wg.Done()
				var res result
				func() {
					defer func() {
						if r := recover(); r != nil {
							res.panicked = true
						}
					}()
					_, err := sfGroup.Do("key", fn)
					res.err = err
				}()
				results[i] = res
			}(i)
		}
		wg.Wait()

		require.Equal(t, int32(1), callCount, "expected fn to be called only once")

		var panickedCount int
		for i, res := range results {
			if res.panicked {
				panickedCount++
				continue
			}
			require.Error(t, res.err, "goroutine %d: expected error when not panicking", i)
			// Ensure the error is a PanicError wrapping the original panic value.
			pErr, ok := res.err.(*PanicError)
			require.True(t, ok, "goroutine %d: error is not of type PanicError", i)
			require.Equal(t, panicValue, pErr.Value, "goroutine %d: unexpected panic value", i)
		}
		require.Equal(t, 1, panickedCount, "expected exactly one goroutine to re-panic")
	})

	t.Run("runtime.Goexit", func(t *testing.T) {
		var group singleFlightGroup[string, int]
		var callCount int32

		fn := func() (int, error) {
			atomic.AddInt32(&callCount, 1)
			time.Sleep(100 * time.Millisecond)
			runtime.Goexit()
			return 42, nil
		}

		type result struct {
			result   int
			err      error
			finished bool
		}

		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make([]result, numGoroutines)

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(i int) {
				defer wg.Done()
				res, err := group.Do("key", fn)
				results[i] = result{result: res, err: err, finished: true}
			}(i)
		}
		wg.Wait()

		require.Equal(t, int32(1), callCount, "expected fn to be called only once")
		finishedCount := 0
		for i, res := range results {
			if !res.finished {
				continue
			}
			finishedCount++
			require.Error(t, res.err, "goroutine %d: expected an error", i)
			require.EqualError(t, res.err, ErrGoexit.Error(), "goroutine %d: expected ErrGoexit", i)
		}
		require.Equal(t, numGoroutines-1, finishedCount, "expected all but one goroutine to return ErrGoexit")
	})
}

/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockUnit struct {
	name           string
	runningCounter *int32
	stop           chan bool
	stopWithError  bool

	startCalled               int
	stopCalled                int
	stopGracefullyCalled      int
	mustRegisterMetricsCalled int
	unregisterMetricsCalled   int
}

func newMockUnit(name string, runningCounter *int32, stopWithError bool) *mockUnit {
	return &mockUnit{
		name:           name,
		runningCounter: runningCounter,
		stop:           make(chan bool),
		stopWithError:  stopWithError,
	}
}

func (s *mockUnit) Start(fatalError chan<- error) {
	s.startCalled++
	atomic.AddInt32(s.runningCounter, 1)
	<-s.stop
}

func (s *mockUnit) Stop(gracefully bool) error {
	s.stopCalled++
	if gracefully {
		s.stopGracefullyCalled++
	}
	defer func() {
		s.stop <- true
		atomic.AddInt32(s.runningCounter, -1)
	}()
	if s.stopWithError {
		return fmt.Errorf("%s: internal error", s.name)
	}
	return nil
}

func (s *mockUnit) MustRegisterMetrics() {
	s.mustRegisterMetricsCalled++
}

func (s *mockUnit) UnregisterMetrics() {
	s.unregisterMetricsCalled++
}

func waitTrue(trueFunc func() bool, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	for {
		if trueFunc() {
			return nil
		}
		select {
		case <-timer.C:
			return errors.New("waiting true timed out")
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func makeCompositeUnit(n int, runningCounter *int32, stopWithErrorsFunc func(index int) bool) *CompositeUnit {
	if stopWithErrorsFunc == nil {
		stopWithErrorsFunc = func(_ int) bool { return false }
	}
	var units []Unit
	for i := 0; i < n; i++ {
		srv := newMockUnit(fmt.Sprintf("srv#%d", i), runningCounter, stopWithErrorsFunc(i))
		units = append(units, srv)
	}
	return NewCompositeUnit(units...)
}

func TestCompositeUnit_StartAndStop(t *testing.T) {
	t.Run("start w/o error and stop w/o error", func(t *testing.T) {
		const unitsNum = 100
		var runningCounter int32

		compositeUnit := makeCompositeUnit(unitsNum, &runningCounter, nil)

		// Start composite unit.
		startExit := make(chan bool)
		go func() {
			defer func() { startExit <- true }()
			compositeUnit.Start(make(chan error))
		}()

		// Wait until all units starts.
		err := waitTrue(func() bool { return atomic.LoadInt32(&runningCounter) == unitsNum }, time.Millisecond*unitsNum*10)
		require.NoError(t, err, "%d units should be started", unitsNum)

		// Stop group and check there is no running units.
		require.NoError(t, compositeUnit.Stop(true), "there should be no error in stop")
		require.Equal(t, 0, int(runningCounter), "there should be no running units")
		select {
		case <-time.NewTimer(time.Millisecond * unitsNum * 10).C:
			require.Fail(t, "waiting finish of Start() is timed out")
		case <-startExit:
		}
	})

	t.Run("start w/o error and stop with error", func(t *testing.T) {
		var err error

		const unitsStopWithErrorNum = 60
		const unitsStopWOErrorNum = 40
		const unitsNum = unitsStopWithErrorNum + unitsStopWOErrorNum

		var runningCounter int32

		compositeUnit := makeCompositeUnit(unitsNum, &runningCounter,
			func(index int) bool { return index < unitsStopWithErrorNum })

		// Start composite unit.
		startExit := make(chan bool)
		go func() {
			defer func() { startExit <- true }()
			compositeUnit.Start(make(chan error))
		}()

		// Wait until all units starts.
		err = waitTrue(func() bool { return atomic.LoadInt32(&runningCounter) == unitsNum }, time.Millisecond*unitsNum*10)
		require.NoError(t, err, "%d units should be started", unitsNum)

		// Stop group and check there is no running units.
		err = compositeUnit.Stop(true)
		require.Error(t, err, "there should be error in stop")

		sgErr := err.(*CompositeUnitError)
		require.NotNil(t, sgErr)
		require.Equal(t, unitsStopWithErrorNum, len(sgErr.UnitErrors),
			"%d units should be stopped with error", unitsStopWithErrorNum)
		require.Equal(t, 0, int(runningCounter), "there should be no running units")
		select {
		case <-time.NewTimer(time.Millisecond * unitsNum * 10).C:
			require.Fail(t, "waiting finish of Start() is timed out")
		case <-startExit:
		}
	})
}

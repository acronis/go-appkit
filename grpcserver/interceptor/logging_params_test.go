/*
Copyright © 2025 Acronis International GmbH.

Released under MIT license.
*/

package interceptor

import (
	"testing"
	"time"

	"github.com/ssgreg/logf"
	"github.com/stretchr/testify/suite"

	"github.com/acronis/go-appkit/log"
)

// LoggingParamsTestSuite is a test suite for LoggingParams
type LoggingParamsTestSuite struct {
	suite.Suite
}

func TestLoggingParams(t *testing.T) {
	suite.Run(t, &LoggingParamsTestSuite{})
}

func (s *LoggingParamsTestSuite) TestExtendFields() {
	lp := &LoggingParams{}

	// Test adding fields
	lp.ExtendFields(
		log.String("field1", "value1"),
		log.Int("field2", 42),
		log.Bool("field3", true),
	)

	s.Require().Len(lp.fields, 3)
	s.Require().Equal("field1", lp.fields[0].Key)
	s.Require().Equal("value1", string(lp.fields[0].Bytes))
	s.Require().Equal("field2", lp.fields[1].Key)
	s.Require().Equal(int64(42), lp.fields[1].Int)
	s.Require().Equal("field3", lp.fields[2].Key)
	s.Require().True(lp.fields[2].Int != 0)

	// Test adding more fields
	lp.ExtendFields(log.String("field4", "value4"))
	s.Require().Len(lp.fields, 4)
	s.Require().Equal("field4", lp.fields[3].Key)
	s.Require().Equal("value4", string(lp.fields[3].Bytes))
}

func (s *LoggingParamsTestSuite) TestAddTimeSlotInt() {
	lp := &LoggingParams{}

	// Test adding time slots
	lp.AddTimeSlotInt("slot1", 100)
	lp.AddTimeSlotInt("slot2", 200)

	timeSlots := lp.getTimeSlots()
	s.Require().Len(timeSlots, 2)
	s.Require().Equal(int64(100), timeSlots["slot1"])
	s.Require().Equal(int64(200), timeSlots["slot2"])

	// Test adding to existing slot (should accumulate)
	lp.AddTimeSlotInt("slot1", 50)
	timeSlots = lp.getTimeSlots()
	s.Require().Equal(int64(150), timeSlots["slot1"])
	s.Require().Equal(int64(200), timeSlots["slot2"])
}

func (s *LoggingParamsTestSuite) TestAddTimeSlotDurationInMs() {
	lp := &LoggingParams{}

	// Test adding duration-based time slots
	lp.AddTimeSlotDurationInMs("slot1", 1*time.Second)
	lp.AddTimeSlotDurationInMs("slot2", 2*time.Second)

	timeSlots := lp.getTimeSlots()
	s.Require().Len(timeSlots, 2)
	s.Require().Equal(int64(1000), timeSlots["slot1"]) // 1 second = 1000ms
	s.Require().Equal(int64(2000), timeSlots["slot2"]) // 2 seconds = 2000ms

	// Test adding to existing slot (should accumulate)
	lp.AddTimeSlotDurationInMs("slot1", 500*time.Millisecond)
	timeSlots = lp.getTimeSlots()
	s.Require().Equal(int64(1500), timeSlots["slot1"]) // 1000 + 500 = 1500ms
}

func (s *LoggingParamsTestSuite) TestConcurrentAccess() {
	lp := &LoggingParams{}

	// Test concurrent access to time slots
	done := make(chan struct{})

	// Start multiple goroutines that add time slots
	for i := 0; i < 10; i++ {
		go func(val int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				lp.AddTimeSlotInt("concurrent_slot", int64(val))
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that all values were accumulated correctly
	timeSlots := lp.getTimeSlots()
	s.Require().Len(timeSlots, 1)

	// Expected value: sum of (0+1+2+...+9) * 100 = 45 * 100 = 4500
	expectedSum := int64(0)
	for i := 0; i < 10; i++ {
		expectedSum += int64(i) * 100
	}
	s.Require().Equal(expectedSum, timeSlots["concurrent_slot"])
}

func (s *LoggingParamsTestSuite) TestEmptyTimeSlots() {
	lp := &LoggingParams{}

	// Test getting time slots when none have been added
	timeSlots := lp.getTimeSlots()
	s.Require().Nil(timeSlots)

	// Test adding a field and then getting time slots
	lp.ExtendFields(log.String("field1", "value1"))
	timeSlots = lp.getTimeSlots()
	s.Require().Nil(timeSlots)
}

func (s *LoggingParamsTestSuite) TestMixedUsage() {
	lp := &LoggingParams{}

	// Test mixing fields and time slots
	lp.ExtendFields(log.String("field1", "value1"))
	lp.AddTimeSlotInt("slot1", 100)
	lp.ExtendFields(log.Int("field2", 42))
	lp.AddTimeSlotDurationInMs("slot2", 1*time.Second)

	// Check fields
	s.Require().Len(lp.fields, 2)
	s.Require().Equal("field1", lp.fields[0].Key)
	s.Require().Equal("field2", lp.fields[1].Key)

	// Check time slots
	timeSlots := lp.getTimeSlots()
	s.Require().Len(timeSlots, 2)
	s.Require().Equal(int64(100), timeSlots["slot1"])
	s.Require().Equal(int64(1000), timeSlots["slot2"])
}

func (s *LoggingParamsTestSuite) TestLoggableIntMapEncodeLogfObject() {
	lim := loggableIntMap{
		"slot1": 100,
		"slot2": 200,
		"slot3": 300,
	}

	// Test that the map is not nil and has expected values
	s.Require().Len(lim, 3)
	s.Require().Equal(int64(100), lim["slot1"])
	s.Require().Equal(int64(200), lim["slot2"])
	s.Require().Equal(int64(300), lim["slot3"])
}

func (s *LoggingParamsTestSuite) TestLoggableIntMapEncodeLogfObjectEmpty() {
	lim := loggableIntMap{}

	// Test that empty map works as expected
	s.Require().Empty(lim)
}

func (s *LoggingParamsTestSuite) TestIntegration() {
	lp := &LoggingParams{}

	// Simulate a real usage scenario
	lp.ExtendFields(log.String("service", "test-service"))
	lp.AddTimeSlotDurationInMs("db_query", 50*time.Millisecond)
	lp.AddTimeSlotDurationInMs("external_api", 100*time.Millisecond)
	lp.ExtendFields(log.Int("retry_count", 2))
	lp.AddTimeSlotDurationInMs("db_query", 25*time.Millisecond) // Additional query

	// Verify final state
	s.Require().Len(lp.fields, 2)
	s.Require().Equal("service", lp.fields[0].Key)
	s.Require().Equal("test-service", string(lp.fields[0].Bytes))
	s.Require().Equal("retry_count", lp.fields[1].Key)
	s.Require().Equal(int64(2), lp.fields[1].Int)

	timeSlots := lp.getTimeSlots()
	s.Require().Len(timeSlots, 2)
	s.Require().Equal(int64(75), timeSlots["db_query"])      // 50 + 25 = 75ms
	s.Require().Equal(int64(100), timeSlots["external_api"]) // 100ms

	// Test creating the time_slots field for logging
	lp.fields = append(lp.fields, log.Field{
		Key:  "time_slots",
		Type: logf.FieldTypeObject,
		Any:  lp.getTimeSlots(),
	})

	s.Require().Len(lp.fields, 3)
	s.Require().Equal("time_slots", lp.fields[2].Key)
	s.Require().Equal(logf.FieldTypeObject, lp.fields[2].Type)
	s.Require().Equal(lp.getTimeSlots(), lp.fields[2].Any)
}

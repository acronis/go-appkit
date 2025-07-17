package interceptor

import (
	"sync"
	"time"

	"github.com/ssgreg/logf"

	"github.com/acronis/go-appkit/log"
)

// loggableIntMap is a map that can be logged as an object with logf encoder.
type loggableIntMap map[string]int64

// EncodeLogfObject encodes the map as a logf object field.
func (lm loggableIntMap) EncodeLogfObject(e logf.FieldEncoder) error {
	for key, value := range lm {
		e.EncodeFieldInt64(key, value)
	}

	return nil
}

// LoggingParams stores parameters for the gRPC logging interceptor
// that may be modified dynamically by the other underlying interceptors/handlers.
type LoggingParams struct {
	fields      []log.Field
	timeSlots   loggableIntMap
	timeSlotsMu sync.RWMutex
}

// ExtendFields extends list of fields that will be logged by the logging interceptor.
func (lp *LoggingParams) ExtendFields(fields ...log.Field) {
	lp.fields = append(lp.fields, fields...)
}

// AddTimeSlotInt sets (if new) or adds duration value to the element of the time_slots map
func (lp *LoggingParams) AddTimeSlotInt(name string, dur int64) {
	lp.setIntMapFieldValue(name, dur)
}

// AddTimeSlotDurationInMs sets (if new) or adds duration value in milliseconds to the element of the time_slots map
func (lp *LoggingParams) AddTimeSlotDurationInMs(name string, dur time.Duration) {
	lp.AddTimeSlotInt(name, dur.Milliseconds())
}

func (lp *LoggingParams) setIntMapFieldValue(fieldName string, value int64) {
	lp.timeSlotsMu.Lock()
	defer lp.timeSlotsMu.Unlock()
	if lp.timeSlots == nil {
		lp.timeSlots = make(loggableIntMap, 1)
	}
	lp.timeSlots[fieldName] += value
}

func (lp *LoggingParams) getTimeSlots() loggableIntMap {
	lp.timeSlotsMu.RLock()
	defer lp.timeSlotsMu.RUnlock()
	return lp.timeSlots
}

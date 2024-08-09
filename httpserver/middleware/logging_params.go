/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"time"

	"github.com/ssgreg/logf"

	"github.com/acronis/go-libs/log"
)

type loggableIntMap map[string]int64

func (lm loggableIntMap) EncodeLogfObject(e logf.FieldEncoder) error {
	for key, value := range lm {
		e.EncodeFieldInt64(key, value)
	}

	return nil
}

// LoggingParams stores parameters for the Logging middleware
// that may be modified dynamically by the other underlying middlewares/handlers.
type LoggingParams struct {
	fields    []log.Field
	timeSlots loggableIntMap
}

// ExtendFields extends list of fields that will be logged by the Logging middleware.
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
	if lp.timeSlots == nil {
		lp.timeSlots = make(loggableIntMap, 1)
	}
	lp.timeSlots[fieldName] += value
}

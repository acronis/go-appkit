/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package middleware

import (
	"testing"
	"time"

	"github.com/ssgreg/logf"
	"github.com/stretchr/testify/require"

	"github.com/acronis/go-libs/log"
)

func TestLoggingParams_SetTimeSlotDurationMs(t *testing.T) {
	lp := LoggingParams{}

	lp.AddTimeSlotInt("slot1", 100)
	lp.AddTimeSlotInt("slot2", 200)
	lp.fields = append(lp.fields, log.Field{Key: "time_slots", Type: logf.FieldTypeObject, Any: lp.timeSlots})

	expected := LoggingParams{
		fields: []log.Field{
			{
				Key:  "time_slots",
				Type: logf.FieldTypeObject,
				Any: loggableIntMap{
					"slot1": 100,
					"slot2": 200,
				},
			},
		},
	}

	require.ElementsMatch(t, lp.fields, expected.fields)
}

func TestLoggingParams_SetTimeSlotDuration(t *testing.T) {
	lp := LoggingParams{}

	lp.AddTimeSlotDurationInMs("slot1", 1*time.Second)
	lp.AddTimeSlotDurationInMs("slot2", 2*time.Second)
	lp.fields = append(lp.fields, log.Field{Key: "time_slots", Type: logf.FieldTypeObject, Any: lp.timeSlots})

	expected := LoggingParams{
		fields: []log.Field{
			{
				Key:  "time_slots",
				Type: logf.FieldTypeObject,
				Any: loggableIntMap{
					"slot1": 1000,
					"slot2": 2000,
				},
			},
		},
	}
	require.ElementsMatch(t, lp.fields, expected.fields)
}

package logging

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

const (
	// EventTypeDontRecord will tell the logger not to emit a Kubernetes event for this log
	// line
	EventTypeDontRecord = "DontRecord"
)

// WithNoRecord adds a log key that suppresses the recorder
func WithNoRecord(logger logr.Logger) logr.Logger {
	return logger.WithValues("eventType", EventTypeDontRecord)
}

var _ logr.Logger = &EventRecordingLogger{}

func WithEventRecorder(logger logr.Logger, recorder record.EventRecorder, object runtime.Object) logr.Logger {
	return &EventRecordingLogger{
		Logger:   logger,
		recorder: recorder,
		object:   object,
		values:   []interface{}{},
	}
}

type EventRecordingLogger struct {
	logr.Logger
	recorder record.EventRecorder
	object   runtime.Object
	values   []interface{}
}

func (erl *EventRecordingLogger) WithValues(values ...interface{}) logr.Logger {
	return &EventRecordingLogger{
		Logger:   erl.Logger.WithValues(values...),
		recorder: erl.recorder,
		object:   erl.object,
		values:   append(erl.values, values...),
	}
}

func (erl *EventRecordingLogger) WithName(name string) logr.Logger {
	return &EventRecordingLogger{
		Logger:   erl.Logger.WithName(name),
		recorder: erl.recorder,
		object:   erl.object,
		values:   erl.values,
	}
}

func (erl *EventRecordingLogger) Info(msg string, keyvals ...interface{}) {
	erl.Logger.Info(msg, keyvals...)

	// Pop key values from our slice into a map so we can better access each element
	kvs := map[string]string{}
	for len(keyvals) > 0 {
		if k, ok := keyvals[0].(string); ok {
			kvs[k] = fmt.Sprintf("%v", keyvals[1])
		}

		keyvals = keyvals[2:]
	}

	var event, eventType, message string
	var ok bool

	if event, ok = kvs["event"]; !ok {
		return // no event
	}

	if kvs["eventType"] == EventTypeDontRecord {
		return // don't record this event
	}

	if err, hasError := kvs["error"]; hasError {
		eventType = corev1.EventTypeWarning
		message = err
	} else {
		eventType = corev1.EventTypeNormal
		message = msg
	}

	erl.recorder.Event(erl.object, eventType, event, message)
}

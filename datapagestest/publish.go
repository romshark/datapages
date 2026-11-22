package datapagestest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/runtime/subject"
)

// Subject appends escaped subject-field values to a generated EvPrefix.
//
// Use EvSubj directly for events without fields.
// Events with SubjectStateID must be dispatched from a handler.
func Subject(prefix string, values ...string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for i, v := range values {
		if i > 0 {
			b.WriteString(subject.Separator)
		}
		b.WriteString(subject.Encode(v))
	}
	return b.String()
}

// Publish sends event as JSON on subj through p.
// Use it to test pages receiving an external event; test the publisher itself against p.
func Publish(t testing.TB, p messaging.Publisher, subj string, event any) {
	t.Helper()
	b, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encoding the event for %q: %v", subj, err)
	}
	if err := p.Publish(t.Context(), messaging.NoopMetrics{}, subj, b); err != nil {
		t.Fatalf("publishing on %q: %v", subj, err)
	}
}

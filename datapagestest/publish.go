package datapagestest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/runtime/subject"
)

// Subject returns the subject a dispatcher publishes an event with subject fields on:
// prefix, the event's generated EvPrefix constant, followed by values in the order of
// the subject fields, each escaped the way the dispatcher escapes it.
//
// An event without subject fields is published on its generated EvSubj constant.
// An event with a SubjectStateID field has no subject a test can build: dispatch it
// from a handler.
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

// Publish publishes event on subj as JSON through p, the broker the server was
// built with, the way a dispatcher does. It tests the pages that receive an event some
// code outside a handler publishes. To test that code, run it against p instead.
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

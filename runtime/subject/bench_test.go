package subject_test

import (
	"strings"
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/runtime/subject"
)

var (
	GS string
	GI int
)

var benchValues = map[string]string{
	"plain":       "alice",
	"email":       "alice@example.com",
	"dotted":      "first.last@example.com",
	"plain max":   strings.Repeat("a", datapages.MaxUserIDEncodedLen),
	"escaped max": strings.Repeat(".", datapages.MaxUserIDEncodedLen/3),
}

// BenchmarkEncode measures the escaping every dispatched subject value pays for,
// from a value that needs none to one that escapes every byte.
func BenchmarkEncode(b *testing.B) {
	for name, v := range benchValues {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				GS = subject.Encode(v)
			}
		})
	}
}

// BenchmarkEncodedLen measures the sizing pass the callers run before Encode to
// allocate the subject buffer once.
func BenchmarkEncodedLen(b *testing.B) {
	for name, v := range benchValues {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				GI = subject.EncodedLen(v)
			}
		})
	}
}

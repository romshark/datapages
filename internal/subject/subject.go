// Package subject holds the message broker subject rules the parser and the
// generator share: what a value filled into a subject may contain,
// and which subjects an event claims.
package subject

import (
	"strings"

	"github.com/romshark/datapages/runtime/subject"
)

const (
	// Sep separates the tokens of a subject.
	Sep = subject.Sep
	// Wildcards match one token ("*") and everything below (">").
	Wildcards = subject.Wildcards
	// Reserved are the characters a subject token must not contain.
	Reserved = subject.Reserved
)

// IsToken reports whether v may be filled into a subject.
func IsToken(v string) bool { return subject.IsToken(v) }

// Prefix returns the subject prefix an event with subject fields publishes
// under. "messaging.sent" becomes "messaging.sent.".
func Prefix(s string) string { return subject.Prefix(s) }

// Claim is the set of subjects one event occupies. An event with subject fields
// publishes under its subject and a page routes what arrives to it by that prefix,
// claiming everything below. An event without them claims one subject and nothing else.
type Claim struct {
	Subject   string
	HasFields bool
}

// Overlaps reports whether a subject exists that both claims cover.
// Such a subject reaches whichever handler the generated router tests first,
// and the two brokers disagree on how many times it arrives.
func (c Claim) Overlaps(other Claim) bool {
	switch {
	case c.HasFields && other.HasFields:
		return strings.HasPrefix(Prefix(c.Subject), Prefix(other.Subject)) ||
			strings.HasPrefix(Prefix(other.Subject), Prefix(c.Subject))
	case c.HasFields:
		return strings.HasPrefix(other.Subject, Prefix(c.Subject))
	case other.HasFields:
		return strings.HasPrefix(c.Subject, Prefix(other.Subject))
	default:
		// Two plain subjects collide only by being equal,
		// which the duplicate check already refused.
		return false
	}
}

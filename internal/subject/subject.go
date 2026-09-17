// Package subject holds the message broker subject rules the parser and the
// generator share: what a value filled into a subject may contain,
// and which subjects an event claims.
package subject

import (
	"errors"
	"fmt"
	"strings"

	"github.com/romshark/datapages/runtime/subject"
)

const (
	// Separator stands between the tokens of a subject.
	Separator = subject.Separator
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

// AppEvent is one event of one application of a module.
type AppEvent struct {
	// App names the app package, relative to the module root,
	// such as "app/frontend".
	App string
	// TypeName is the name of the event type.
	TypeName string
	// Decl identifies the declaration behind the event.
	// Two applications naming one declaration take part in one event
	// rather than claiming one subject twice.
	Decl string
	Claim
}

// CheckAcrossApps reports two events of different applications of
// one module that claim the same subject.
//
// One module may build several applications, and two of them given the
// same broker is the ordinary deployment. A subject both claim carries each
// application's messages to the other's streams, where the foreign payload is
// decoded into the receiving application's own event type.
//
// The events of one application are checked while it is parsed.
// This is the rest of the rule and needs every parsed model of the module at once.
// Order the input by application and the message names the first claim.
func CheckAcrossApps(events []AppEvent) error {
	var errs []error
	for i, e := range events {
		for _, first := range events[:i] {
			if first.App == e.App || first.Decl == e.Decl {
				continue
			}
			// Equality is tested here and not in [Claim.Overlaps],
			// which leaves it to the duplicate check of one application.
			// Two applications have no such check between them.
			if e.Subject != first.Subject && !e.Overlaps(first.Claim) {
				continue
			}
			errs = append(errs, fmt.Errorf(
				"%s.%s claims subject %q and %s.%s claims %q: "+
					"two applications of one module must not claim the same subject",
				e.App, e.TypeName, e.Subject,
				first.App, first.TypeName, first.Subject,
			))
		}
	}
	return errors.Join(errs...)
}

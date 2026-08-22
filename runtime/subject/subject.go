// Package subject holds the message broker subject rules: what a value filled
// into a subject may contain.
//
// Application code must not import this package.
package subject

import "strings"

const (
	// Sep separates the tokens of a subject.
	Sep = "."
	// Wildcards match one token ("*") and everything below (">").
	Wildcards = "*>"
	// Reserved are the characters a subject token must not contain.
	Reserved = Sep + Wildcards + " \t\r\n"
)

// IsToken reports whether v may be filled into a subject.
// On the subscribe side a wildcard or a separator would widen the subscription
// past the value the client asked for. On the publish side either one produces
// a subject that no subscription matches.
func IsToken(v string) bool {
	return v != "" && !strings.ContainsAny(v, Reserved)
}

// Prefix returns the subject prefix an event with subject fields publishes
// under. "messaging.sent" becomes "messaging.sent.".
func Prefix(s string) string { return s + Sep }

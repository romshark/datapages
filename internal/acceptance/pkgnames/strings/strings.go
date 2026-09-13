// Package strings is named after the standard library package app_gen.go imports.
// Its types reach the generated value readers as path, query and signal field types,
// and each conversion they write names the type.
package strings

import "strings"

// Code is parsed by conversion from the string the request carried.
type Code string

// Count is parsed by strconv and converted to its own type.
type Count int

// Slug parses itself, which is the branch of the value readers
// that calls UnmarshalText rather than converting.
type Slug string

func (s Slug) MarshalText() ([]byte, error) {
	return []byte(strings.ToLower(string(s))), nil
}

func (s *Slug) UnmarshalText(b []byte) error {
	*s = Slug(strings.ToLower(string(b)))
	return nil
}

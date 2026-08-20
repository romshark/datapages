// Package structtag extracts the struct tag values Datapages reads from path,
// query and signals fields. It sits outside the parser because the generator
// reads the same tags.
package structtag

import (
	"reflect"
	"strings"
)

// JSONTagExcluded reports whether the struct tag is `json:"-"`,
// which omits the field. `json:"-,"` instead names the field "-" and is not excluded.
func JSONTagExcluded(tag string) bool {
	return reflect.StructTag(tag).Get("json") == "-"
}

// JSONTagValue extracts the value from a `json:"value"`
// struct tag, stripping options like ",omitempty".
func JSONTagValue(tag string) string {
	v := reflect.StructTag(tag).Get("json")
	name, _, _ := strings.Cut(v, ",")
	return name
}

// ReflectSignalTagValue extracts the value from a `reflectsignal:"value"` struct tag.
func ReflectSignalTagValue(tag string) string {
	return reflect.StructTag(tag).Get("reflectsignal")
}

// PathTagValue extracts the value from a `path:"value"` struct tag.
func PathTagValue(tag string) string {
	return reflect.StructTag(tag).Get("path")
}

// QueryTagValue extracts the value from a `query:"value"` struct tag.
func QueryTagValue(tag string) string {
	return reflect.StructTag(tag).Get("query")
}

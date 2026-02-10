// Package structtag provides struct tag value extraction and
// cross-validation for Datapages handler parameters.
package structtag

import (
	"errors"
	"fmt"
	"go/types"
	"strings"

	"datapages/parser/model"
)

// ErrQueryReflectSignalNotInSignals indicates a reflectsignal
// tag references a signal not present in the signals parameter.
var ErrQueryReflectSignalNotInSignals = errors.New(
	"query reflectsignal tag references signal " +
		"not in signals parameter",
)

// JSONTagValue extracts the value from a `json:"value"`
// struct tag, stripping options like ",omitempty".
func JSONTagValue(tag string) string {
	const prefix = `json:"`
	_, after, ok := strings.Cut(tag, prefix)
	if !ok {
		return ""
	}
	before, _, ok0 := strings.Cut(after, "\"")
	if !ok0 {
		return ""
	}
	if k := strings.IndexByte(before, ','); k >= 0 {
		before = before[:k]
	}
	return before
}

// ReflectSignalTagValue extracts the value from a
// `reflectsignal:"value"` struct tag.
func ReflectSignalTagValue(tag string) string {
	const prefix = `reflectsignal:"`
	_, after, ok := strings.Cut(tag, prefix)
	if !ok {
		return ""
	}
	before, _, ok0 := strings.Cut(after, "\"")
	if !ok0 {
		return ""
	}
	return before
}

// PathTagValue extracts the value from a `path:"value"`
// struct tag.
func PathTagValue(tag string) string {
	const prefix = `path:"`
	_, after, ok := strings.Cut(tag, prefix)
	if !ok {
		return ""
	}
	before, _, ok0 := strings.Cut(after, "\"")
	if !ok0 {
		return ""
	}
	return before
}

// ValidateReflectSignal checks that every reflectsignal tag
// on a query field references a json tag value in the signals
// struct.
func ValidateReflectSignal(
	h *model.Handler, recv, method string,
) error {
	if h.InputQuery == nil || h.InputSignals == nil {
		return nil
	}

	querySt, ok := h.InputQuery.Type.Resolved.Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	sigSt, ok := h.InputSignals.Type.Resolved.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	sigNames := make(map[string]bool, sigSt.NumFields())
	for i := range sigSt.NumFields() {
		if v := JSONTagValue(sigSt.Tag(i)); v != "" {
			sigNames[v] = true
		}
	}

	for i := range querySt.NumFields() {
		rs := ReflectSignalTagValue(querySt.Tag(i))
		if rs == "" {
			continue
		}
		if !sigNames[rs] {
			return fmt.Errorf(
				"%w: %q in %s.%s",
				ErrQueryReflectSignalNotInSignals,
				rs, recv, method,
			)
		}
	}
	return nil
}

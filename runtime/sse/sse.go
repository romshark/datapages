// Package sse implements datapages.SSE on the Datastar generator.
// It keeps datastar out of handler signatures.
//
// Application code must not import this package.
package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/datapages"
)

// New wraps a Datastar generator as a [datapages.SSE].
// Every call of the result writes one event on gen.
func New(g *datastar.ServerSentEventGenerator) datapages.SSE {
	return wrapper{g: g}
}

type wrapper struct {
	g *datastar.ServerSentEventGenerator
}

func (s wrapper) Context() context.Context { return s.g.Context() }

func (s wrapper) PatchElement(c datapages.Component) error {
	return s.g.PatchElementTempl(c)
}

func (s wrapper) PatchElementAt(
	c datapages.Component, selectorCSS string, mode datapages.PatchMode,
) error {
	if err := checkSelector(selectorCSS); err != nil {
		return err
	}
	switch mode {
	case datapages.PatchModeOuter, datapages.PatchModeInner,
		datapages.PatchModeReplace, datapages.PatchModePrepend,
		datapages.PatchModeAppend, datapages.PatchModeBefore,
		datapages.PatchModeAfter:
	default:
		mode = "" // Not a PatchMode constant, patch in the default mode.
	}
	switch {
	case selectorCSS == "" && mode == "":
		return s.g.PatchElementTempl(c)
	case mode == "":
		return s.g.PatchElementTempl(c, datastar.WithSelector(selectorCSS))
	case selectorCSS == "":
		return s.g.PatchElementTempl(
			c, datastar.WithMode(datastar.ElementPatchMode(mode)),
		)
	}
	return s.g.PatchElementTempl(c,
		datastar.WithSelector(selectorCSS),
		datastar.WithMode(datastar.ElementPatchMode(mode)))
}

// checkSelector rejects a selector that would end the event line it goes on.
// datastar writes the selector as given, while it splits the elements on "\n"
// and quotes the redirect URL.
func checkSelector(selectorCSS string) error {
	if strings.ContainsAny(selectorCSS, "\r\n") {
		return fmt.Errorf("%w: %q", datapages.ErrSelectorLineBreak, selectorCSS)
	}
	return nil
}

// removeElementModeDataline is the mode line of a removal event.
const removeElementModeDataline = datastar.ModeDatalineLiteral +
	string(datastar.ElementPatchModeRemove)

func (s wrapper) RemoveElement(selectorCSS string) error {
	if err := checkSelector(selectorCSS); err != nil {
		return err
	}
	return s.g.Send(datastar.EventTypePatchElements, []string{
		datastar.SelectorDatalineLiteral + selectorCSS,
		removeElementModeDataline,
	})
}

func (s wrapper) ExecuteScript(script string) error {
	return s.g.ExecuteScript(script)
}

func (s wrapper) PatchSignals(v any) error {
	j, err := marshalSignals(v)
	if err != nil {
		return err
	}
	return s.g.PatchSignals(j)
}

func (s wrapper) PatchSignalsIfMissing(v any) error {
	j, err := marshalSignals(v)
	if err != nil {
		return err
	}
	return s.g.PatchSignals(j, datastar.WithOnlyIfMissing(true))
}

// marshalSignals encodes v as JSON. A json.RawMessage passes through.
func marshalSignals(v any) ([]byte, error) {
	if raw, ok := v.(json.RawMessage); ok {
		if !json.Valid(raw) {
			return nil, errors.New("signals are not valid JSON")
		}
		return raw, nil
	}
	j, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling signals JSON: %w", err)
	}
	return j, nil
}

func (s wrapper) Redirect(target string) error {
	// The redirect travels as a <script> element. A "</script>" in the target
	// would end that element and leave the rest of it in the DOM as markup,
	// which is why the target goes in JSON-encoded: encoding/json writes
	// "<" as \u003c, and JavaScript reads it back as "<".
	enc, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("encoding redirect target: %w", err)
	}
	return s.g.ExecuteScript(
		"setTimeout(() => window.location.href = " + string(enc) + ")",
	)
}

func (s wrapper) Prefetch(urls ...string) error {
	return s.g.Prefetch(urls...)
}

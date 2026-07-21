package templcheck

import "testing"

func TestIsDatastarActionAttr(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  bool
	}{
		// data-on:<event> — DOM events
		"data-on:click":          {input: "data-on:click", want: true},
		"data-on:submit":         {input: "data-on:submit", want: true},
		"data-on:load":           {input: "data-on:load", want: true},
		"data-on:keydown":        {input: "data-on:keydown", want: true},
		"data-on:custom-event":   {input: "data-on:custom-event", want: true},
		"data-on:click.debounce": {input: "data-on:click.debounce", want: true},

		// data-on-intersect plugin
		"data-on-intersect": {
			input: "data-on-intersect", want: true,
		},
		"data-on-intersect.once": {
			input: "data-on-intersect.once", want: true,
		},
		"data-on-intersect__once__full": {
			input: "data-on-intersect__once__full", want: true,
		},

		// data-on-interval plugin
		"data-on-interval": {
			input: "data-on-interval", want: true,
		},
		"data-on-interval__duration.500ms": {
			input: "data-on-interval__duration.500ms", want: true,
		},

		// data-on-signal-patch plugin
		"data-on-signal-patch": {
			input: "data-on-signal-patch", want: true,
		},
		"data-on-signal-patch__debounce.500ms": {
			input: "data-on-signal-patch__debounce.500ms", want: true,
		},

		// data-init
		"data-init":       {input: "data-init", want: true},
		"data-init.once":  {input: "data-init.once", want: true},
		"data-init__once": {input: "data-init__once", want: true},

		// NOT Datastar action contexts
		"data-only": {
			input: "data-only", want: false,
		},
		"data-onerous": {
			input: "data-onerous", want: false,
		},
		"data-on": {
			input: "data-on", want: false,
		},
		"data-on-somethingelse": {
			input: "data-on-somethingelse", want: false,
		},
		"data-on-": {
			input: "data-on-", want: false,
		},
		"data-initial": {
			input: "data-initial", want: false,
		},
		"data-init-foo": {
			input: "data-init-foo", want: false,
		},
		"data-on-signal-patch-filter": {
			input: "data-on-signal-patch-filter", want: false,
		},
		"href": {
			input: "href", want: false,
		},
		"action": {
			input: "action", want: false,
		},
		"class": {
			input: "class", want: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := isDatastarActionAttr(tc.input)
			if got != tc.want {
				t.Errorf("isDatastarActionAttr(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

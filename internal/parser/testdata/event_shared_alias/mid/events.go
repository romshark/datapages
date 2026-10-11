// Package mid re-exports the events of another package as aliases.
//
// The app package imports this one and never imports deep, which puts the
// declarations 1 package further out than the app package imports.
package mid

import "datapagestest/fixture/event_shared_alias/deep"

type (
	EventDeep  = deep.EventDeep
	EventOther = deep.EventOther
)

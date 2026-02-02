package app

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// EventA is "a"
type EventA struct {
	Payload string `json:"p"`
}

// EventB is "b"
type EventB struct {
	Payload string `json:"p"`
}

// EventC is "c"
type EventC struct {
	Payload string `json:"p"`
}

// EventD is "d"
type EventD struct {
	Payload string `json:"p"`
}

// AbstractLevel1 defines OnEventA and GET
type AbstractLevel1 struct{ App *App }

func (AbstractLevel1) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

func (AbstractLevel1) OnA(
	event EventA, sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// AbstractLevel2 embeds AbstractLevel1 and defines OnEventB.
// It also overrides GET? No, let's keep it.
type AbstractLevel2 struct {
	App *App
	AbstractLevel1
}

func (AbstractLevel2) OnB(
	event EventB, sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageConcrete is /concrete
//
// This page embeds AbstractLevel2.
// It inherits:
// - GET (from Level1)
// - OnA (from Level1)
// - OnB (from Level2)
// It defines OnC.
type PageConcrete struct {
	App *App
	AbstractLevel2
}

func (PageConcrete) OnC(
	event EventC, sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageOverride is /override
//
// This page embeds AbstractLevel1.
// It overrides OnA.
type PageOverride struct {
	App *App
	AbstractLevel1
}

func (PageOverride) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

// Override OnA (same method name)
func (PageOverride) OnA(
	event EventA, sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageOverrideEvent is /override-event
//
// This page embeds AbstractLevel1.
// It handles EventA with a DIFFERENT method name.
// Since PageOverrideEvent handles EventA, proper flattening should ignore Level1's handler for EventA.
type PageOverrideEvent struct {
	App *App
	AbstractLevel1
}

func (PageOverrideEvent) GET(r *http.Request) (body templ.Component, err error) {
	return nil, nil
}

func (PageOverrideEvent) OnNewA(
	event EventA, sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// AbstractLevel3 defines OnD.
type AbstractLevel3 struct{ App *App }

func (AbstractLevel3) OnD(
	event EventD, sse *datastar.ServerSentEventGenerator,
) error {
	return nil
}

// PageMulti is /multi
//
// This page embeds AbstractLevel1 and AbstractLevel3.
// It inherits:
// - GET (from Level1)
// - OnA (from Level1)
// - OnD (from Level3)
type PageMulti struct {
	App *App
	AbstractLevel1
	AbstractLevel3
}

package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

type App struct{}

// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
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

// EmbedLevel1 defines OnA.
type EmbedLevel1 struct{ App *App }

func (EmbedLevel1) OnA(
	event EventA, sse datapages.SSE,
) error {
	return nil
}

// EmbedLevel2 embeds EmbedLevel1 and defines OnB.
type EmbedLevel2 struct {
	App *App
	EmbedLevel1
}

func (EmbedLevel2) OnB(
	event EventB, sse datapages.SSE,
) error {
	return nil
}

// PageConcrete is /concrete
//
// This page embeds EmbedLevel2.
// It inherits:
// - OnA (from Level1)
// - OnB (from Level2)
// It defines GET and OnC.
type PageConcrete struct {
	App *App
	EmbedLevel2
}

func (PageConcrete) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageConcrete) OnC(
	event EventC, sse datapages.SSE,
) error {
	return nil
}

// PageOverride is /override
//
// This page embeds EmbedLevel1.
// It overrides OnA.
type PageOverride struct {
	App *App
	EmbedLevel1
}

func (PageOverride) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// Override OnA (same method name)
func (PageOverride) OnA(
	event EventA, sse datapages.SSE,
) error {
	return nil
}

// PageOverrideEvent is /override-event
//
// This page embeds EmbedLevel1.
// It handles EventA with a DIFFERENT method name.
// Since PageOverrideEvent handles EventA, proper flattening should ignore Level1's handler for EventA.
type PageOverrideEvent struct {
	App *App
	EmbedLevel1
}

func (PageOverrideEvent) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

func (PageOverrideEvent) OnNewA(
	event EventA, sse datapages.SSE,
) error {
	return nil
}

// EmbedLevel3 defines OnD.
type EmbedLevel3 struct{ App *App }

func (EmbedLevel3) OnD(
	event EventD, sse datapages.SSE,
) error {
	return nil
}

// PageMulti is /multi
//
// This page embeds EmbedLevel1 and EmbedLevel3.
// It inherits:
// - OnA (from Level1)
// - OnD (from Level3)
type PageMulti struct {
	App *App
	EmbedLevel1
	EmbedLevel3
}

func (PageMulti) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

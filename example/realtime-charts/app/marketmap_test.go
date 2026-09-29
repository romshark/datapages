// Tests the market map and its stream over HTTP, with the env and the tab of the tests of
// the chart pages.

package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/example/realtime-charts/app"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/realtime-charts/market"
)

// TestMapPage tests the tree, the head and the footer of the market map.
func TestMapPage(t *testing.T) {
	e := newEnv(t)
	resp := e.get(t, views["map"].href(app.ViewQuery{}))
	require.Equal(t, http.StatusOK, resp.Status)
	doc := resp.Document()
	require.Equal(t, "map", resp.Signals(t)["view"])
	marketMap := one(t, doc, "the market map", tag("market-map"))
	require.Equal(t, "10", attrOf(marketMap, "scale"))

	// The tree lists every index once, in its sector.
	var sectors []struct {
		Name    string      `json:"name"`
		Indexes [][2]string `json:"indexes"`
	}
	require.NoError(t, json.Unmarshal([]byte(attrOf(marketMap, "sectors")), &sectors))
	require.Len(t, sectors, len(market.Sectors))
	listed := map[string]bool{}
	for i, sector := range sectors {
		require.Equal(t, market.Sectors[i], sector.Name)
		for _, in := range sector.Indexes {
			index := market.Indexes[indexOf(t, in[0])]
			require.Equal(t, [2]string{index.Symbol, index.Name}, in)
			require.Equal(t, sector.Name, index.Sector, index.Symbol)
			require.False(t, listed[in[0]], "%s listed twice", in[0])
			listed[in[0]] = true
		}
	}
	require.Len(t, listed, len(market.Indexes))

	requireSidebar(t, resp.ElementByID(t, "sidebar"), defaults)
	requireTabs(t, resp.ElementByID(t, "tabs"), "map",
		app.ViewQuery{Type: "candles", Layout: "grid", Refresh: "1s", Range: "15m", Indexes: indexesParam(defaults)})
	one(t, doc, "the ECharts script", tag("script"),
		attr("src", "https://cdn.jsdelivr.net/npm/echarts@6.1.0/dist/echarts.min.js"))
	one(t, doc, "the market map module", tag("script"), attr("src", assets.Path("market-map.js")))
	one(t, doc, "the ECharts credit", tag("a"), attr("href", "https://echarts.apache.org/"))
	require.NotContains(t, resp.Body, "lightweight-charts")
	require.NotContains(t, resp.Body, "uPlot")
}

// TestMapStream tests that the market map receives the tile of every index with every
// push, whichever indexes the sidebar selects.
func TestMapStream(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, views["map"].href(app.ViewQuery{}), `{"view":"map","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"none"}`)
	p := tb.pushes(t, 1)[0]
	require.Equal(t, t0.Unix(), p.Pushed)
	e.requireMap(t, p.Map, t0.Unix())

	e.tick(t, 1)
	p = tb.pushes(t, 2)[1]
	require.Equal(t, t0.Unix()+1, p.Pushed)
	e.requireMap(t, p.Map, t0.Unix()+1)
}

// TestMapPausedPage tests that a market map opened paused at a second shows the tiles of
// that second.
func TestMapPausedPage(t *testing.T) {
	e := newEnv(t)
	e.market.Tick(t0.Add(20 * time.Second))
	p := e.openTab(t, views["map"].href(app.ViewQuery{}), fmt.Sprintf(
		`{"view":"map","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA","paused":%d}`, t0.Unix()+7,
	)).pushes(t, 1)[0]
	require.Equal(t, t0.Unix()+7, p.Pushed)
	e.requireMap(t, p.Map, t0.Unix()+7)
}

// TestMapToggle tests that a toggle on the market map patches the sidebar and the indexes
// signal, which the map outlines, and sends no tiles.
func TestMapToggle(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, views["map"].href(app.ViewQuery{}), `{"view":"map","type":"candles","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA"}`)
	tb.pushes(t, 1)

	evs := tb.toggle(t, "GLWT")
	p := onlyPatch(t, evs)
	require.Equal(t, "AURA-GLWT", p.Indexes)
	require.Nil(t, p.Map)
	requireSidebar(t, mustPatch(t, evs, "sidebar"), []string{"AURA", "GLWT"})
	requireTabs(t, mustPatch(t, evs, "tabs"), "map",
		app.ViewQuery{Type: "candles", Layout: "grid", Refresh: "2s", Range: "15m", Indexes: "AURA-GLWT"})
	one(t, mustPatch(t, evs, "market"), "the market map", tag("market-map"))
}

// requireMap checks that tiles are the tiles of all indexes at the second at: their
// prices and their changes since the open.
func (e *env) requireMap(t *testing.T, tiles [][3]any, at int64) {
	t.Helper()
	require.Len(t, tiles, len(market.Indexes))
	for i, in := range market.Indexes {
		price, ok := e.db.At(in.Symbol, at*1000)
		require.True(t, ok, in.Symbol)
		require.Equal(t, in.Symbol, tiles[i][0])
		require.InDelta(t, price.Value, tiles[i][1], 0.006, in.Symbol)
		require.InDelta(t, (price.Value/e.market.Open(i)-1)*100, tiles[i][2], 0.006, in.Symbol)
	}
}

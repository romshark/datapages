// Tests reuse the chart-page HTTP environment.

package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/romshark/datapages/example/realtime-charts/app"
	"github.com/romshark/datapages/example/realtime-charts/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/realtime-charts/market"
)

// TestMapPage tests the complete initial market map.
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
	e.requireMap(t, tilesOf(t, doc), t0.Unix())
	require.Equal(t, indexesParam(defaults), attrOf(marketMap, "selection"))

	requireSidebar(t, resp.ElementByID(t, "sidebar"), defaults)
	requireTabs(t, resp.ElementByID(t, "tabs"), "map",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "1s",
			Range:   "15m",
			Indexes: indexesParam(defaults),
		})
	one(t, doc,
		"the ECharts script", tag("script"),
		attr("src", "https://cdn.jsdelivr.net/npm/echarts@6.1.0/dist/echarts.min.js"))
	one(t, doc,
		"the market map module", tag("script"), attr("src", assets.Path("market-map.js")))
	one(t, doc,
		"the ECharts credit", tag("a"), attr("href", "https://echarts.apache.org/"))
	require.NotContains(t, resp.Body, "lightweight-charts")
	require.NotContains(t, resp.Body, "uPlot")
}

// TestMapStream tests that every push includes every index.
func TestMapStream(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, views["map"].href(app.ViewQuery{}),
		`{"view":"map","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"none"}`)
	r := tb.renders(t, 1)[0]
	require.Equal(t, t0.Unix(), r.At)
	e.requireMap(t, tilesOf(t, r.Main), t0.Unix())

	e.tick(t, 1)
	r = tb.renders(t, 2)[1]
	require.Equal(t, t0.Unix()+1, r.At)
	e.requireMap(t, tilesOf(t, r.Main), t0.Unix()+1)
}

// TestMapPausedPage tests a shared paused map URL.
func TestMapPausedPage(t *testing.T) {
	e := newEnv(t)
	e.market.Tick(t0.Add(20 * time.Second))
	r := e.openTab(t, views["map"].href(app.ViewQuery{}), fmt.Sprintf(
		`{"view":"map","type":"candles","layout":"grid","refresh":"1s","range":"15m","indexes":"AURA","paused":%d}`, t0.Unix()+7,
	)).renders(t, 1)[0]
	require.Equal(t, t0.Unix()+7, r.At)
	e.requireMap(t, tilesOf(t, r.Main), t0.Unix()+7)
}

// TestMapToggle tests selection across the map, sidebar, tabs and URL signal.
func TestMapToggle(t *testing.T) {
	e := newEnv(t)
	tb := e.openTab(t, views["map"].href(app.ViewQuery{}),
		`{"view":"map","type":"candles","layout":"grid","refresh":"2s","range":"15m","indexes":"AURA"}`)
	tb.renders(t, 1)

	evs := tb.toggle(t, "GLWT")
	require.Equal(t, "AURA-GLWT", indexesOf(t, evs))
	r := onlyRender(t, evs)
	marketMap := one(t, r.Main, "the market map", tag("market-map"))
	require.Equal(t, "AURA-GLWT", attrOf(marketMap, "selection"))
	e.requireMap(t, tilesOf(t, r.Main), t0.Unix())
	requireSidebar(t, mustPatch(t, evs, "sidebar"), []string{"AURA", "GLWT"})
	requireTabs(t, mustPatch(t, evs, "tabs"), "map",
		app.ViewQuery{
			Type:    "candles",
			Layout:  "grid",
			Refresh: "2s",
			Range:   "15m",
			Indexes: "AURA-GLWT",
		})
}

// requireMap checks every index price and change at a second.
func (e *env) requireMap(t *testing.T, tiles [][3]any, at int64) {
	t.Helper()
	require.Len(t, tiles, len(market.Indexes))
	for i, in := range market.Indexes {
		price, ok := e.db.At(in.Symbol, at*1000)
		require.True(t, ok, in.Symbol)
		require.Equal(t, in.Symbol, tiles[i][0])
		require.InDelta(t, price.Value, tiles[i][1], 0.006, in.Symbol)
		require.InDelta(t,
			(price.Value/e.market.Open(i)-1)*100,
			tiles[i][2], 0.006, in.Symbol)
	}
}

func tilesOf(t *testing.T, n *html.Node) [][3]any {
	t.Helper()
	var tiles [][3]any
	marketMap := one(t, n, "the market map", tag("market-map"))
	require.NoError(t, json.Unmarshal([]byte(attrOf(marketMap, "tiles")), &tiles))
	return tiles
}

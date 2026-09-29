package app

import (
	"encoding/json"

	"github.com/romshark/datapages/example/realtime-charts/market"
)

// mapScale is the change since the open in percent that gets the full up or down color on
// the market map. The indexes of this market drift a few percent to a tenth from their
// opening prices.
const mapScale = 10

// mapTile is [symbol, price, change since the open in percent].
type mapTile [3]any

// marketMap returns the tile of every index with its price at the Unix second at, in the
// order of the sidebar.
func (a *App) marketMap(at int64) []mapTile {
	tiles := make([]mapTile, len(market.Indexes))
	for i, in := range market.Indexes {
		p, _ := a.db.At(in.Symbol, at*1000)
		tiles[i] = mapTile{in.Symbol, cents(p.Value), round(percent(p.Value, a.market.Open(i)), 2)}
	}
	return tiles
}

// mapSectors is the tree of the market map as JSON, which does not change: the sectors in
// the order of [market.Sectors], each with the symbols and names of its indexes.
var mapSectors = func() string {
	type sector struct {
		Name    string      `json:"name"`
		Indexes [][2]string `json:"indexes"`
	}
	sectors := make([]sector, len(market.Sectors))
	for i, name := range market.Sectors {
		sectors[i].Name = name
		for _, in := range market.Indexes {
			if in.Sector == name {
				sectors[i].Indexes = append(sectors[i].Indexes, [2]string{in.Symbol, in.Name})
			}
		}
	}
	// Marshaling strings cannot fail.
	b, _ := json.Marshal(sectors)
	return string(b)
}()

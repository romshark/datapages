package app

import (
	"encoding/json"

	"github.com/romshark/datapages/example/realtime-charts/market"
)

// mapScale is the percent change that reaches full color.
const mapScale = 10

// mapTile is [symbol, price, change since the open in percent].
type mapTile [3]any

// marketMap returns every index at the Unix second at, in sidebar order.
func (a *App) marketMap(at int64) []mapTile {
	tiles := make([]mapTile, len(market.Indexes))
	for i, in := range market.Indexes {
		p, _ := a.db.At(in.Symbol, at*1000)
		tiles[i] = mapTile{
			in.Symbol, cents(p.Value), round(percent(p.Value, a.market.Open(i)), 2),
		}
	}
	return tiles
}

// mapSectors is the immutable market tree as JSON, in [market.Sectors] order.
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
				sectors[i].Indexes = append(sectors[i].Indexes,
					[2]string{in.Symbol, in.Name})
			}
		}
	}
	// These values cannot make json.Marshal fail.
	b, _ := json.Marshal(sectors)
	return string(b)
}()

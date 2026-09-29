// <market-map> draws the market map with Apache ECharts: every index a tile of its
// sector, colored by its change since the open.
//
// Apache ECharts
// Copyright 2017-2026 The Apache Software Foundation
//
// This product includes software developed at
// The Apache Software Foundation (https://www.apache.org/).
//
// Licensed under the Apache License 2.0.
//
// The sectors attribute is the tree of the map as JSON: the sectors, each with the
// [symbol, name] of its indexes. The server sends the tiles as signals, rows
// [symbol, price, change in percent], which the page assigns to the data property. The
// page assigns the indexes signal to the selection property, and the map outlines those
// indexes. A change of the scale attribute in percent or more either way gets the full up
// or down color, a smaller one a mix of it with the neutral color.
import { rocket } from "./datastar-rocket.js";

const E = window.echarts;
const font = "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
const scheme = matchMedia("(prefers-color-scheme: dark)");

// colorsOf reads the colors from the custom properties of the page stylesheet, which
// switches them with the color scheme.
const colorsOf = (el) => {
  const css = getComputedStyle(el);
  const color = (name) => css.getPropertyValue(name).trim();
  return {
    text: color("--text"),
    muted: color("--muted"),
    bg: color("--bg"),
    surface: color("--surface"),
    border: color("--border"),
    up: color("--map-up"),
    down: color("--map-down"),
    neutral: color("--map-neutral"),
    outline: color("--map-outline"),
  };
};

// mix returns the color the fraction t of the way from the hex color a to b.
const mix = (a, b, t) => {
  const rgb = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  const [from, to] = [rgb(a), rgb(b)];
  return `rgb(${from.map((v, i) => Math.round(v + (to[i] - v) * t)).join(",")})`;
};

const percent = (v) => `${v > 0 ? "+" : ""}${v.toFixed(2)}%`;
const escape = (s) => String(s).replace(/[&<>"]/g, (c) => `&#${c.charCodeAt(0)};`);

const options = (host, { sectors, scale }, tiles, selection) => {
  const c = colorsOf(host);
  // The tiles of a narrow map are small, and so are their labels.
  const small = host.clientWidth < 500;
  const data = sectors.map((sector) => ({
    name: sector.name,
    // The sector shows between its tiles and behind its name.
    itemStyle: { color: c.surface },
    children: sector.indexes.map(([symbol, name]) => {
      const tile = { ...tiles.get(symbol), name, sector: sector.name };
      const change = tile.change ?? 0;
      return {
        name: symbol,
        value: 1,
        tile,
        itemStyle: {
          color: mix(c.neutral, change < 0 ? c.down : c.up, Math.min(Math.abs(change) / scale, 1)),
          borderColor: c.outline,
          borderWidth: selection.has(symbol) ? 2 : 0,
        },
      };
    }),
  }));
  return {
    aria: {
      enabled: true,
      label: {
        description: `A market map of ${tiles.size} indexes by sector, ` +
          "each colored by its change since the open.",
      },
    },
    textStyle: { fontFamily: font },
    tooltip: {
      // The tooltip stays inside the map, which on a phone is the width of the screen.
      confine: true,
      backgroundColor: c.surface,
      borderColor: c.border,
      textStyle: { color: c.text, fontSize: 12 },
      formatter: ({ data }) => {
        const tile = data?.tile;
        if (tile?.price == null) {
          return escape(data?.name ?? "");
        }
        const color = tile.change < 0 ? c.down : c.up;
        return `<b>${escape(data.name)}</b> ${escape(tile.name)}<br>${escape(tile.sector)}<br>` +
          `${tile.price.toFixed(2)} <span style="color:${color}">${percent(tile.change)}</span>`;
      },
    },
    series: [{
      type: "treemap",
      left: 0,
      top: 0,
      right: 0,
      bottom: 0,
      roam: false,
      nodeClick: false,
      breadcrumb: { show: false },
      animationDurationUpdate: 400,
      label: {
        show: true,
        // A sector has no tile. Its name is the label above its tiles.
        formatter: ({ data }) => {
          if (!data.tile) {
            return data.name;
          }
          return data.tile.change == null
            ? `{symbol|${data.name}}`
            : `{symbol|${data.name}}\n{change|${percent(data.tile.change)}}`;
        },
        rich: {
          symbol: { fontSize: small ? 11 : 13, fontWeight: 700, lineHeight: small ? 14 : 18, color: "#fff" },
          change: { fontSize: small ? 10 : 11, lineHeight: small ? 12 : 15, color: "#fff" },
        },
      },
      upperLabel: { show: true, height: 20, color: c.muted, fontSize: 11, fontWeight: 600 },
      levels: [
        // A node paints the gaps between its children in its border color: the page
        // between the sectors, and between the tiles.
        {
          itemStyle: {
            borderColor: c.bg,
            borderWidth: 0,
            gapWidth: 4
          },
          upperLabel: { show: false }
        },
        { itemStyle: {
          borderColor: c.bg,
          borderWidth: 2,
          gapWidth: 2
        }},
        {},
      ],
      data,
    }],
  };
};

rocket("market-map", {
  props: ({ json, number }) => ({
    sectors: json.default(() => []),
    scale: number.default(10),
  }),
  // A render would empty the box that ECharts draws into.
  renderOnPropChange: false,
  render: ({ html }) => html`
    <style>:host{display:block;position:relative}.box{position:absolute;inset:0}</style>
    <div class="box" data-ref:box></div>
  `,
  onFirstRender: ({ host, props, refs, cleanup, defineHostProp }) => {
    const chart = E.init(refs.box);
    const tiles = new Map();
    let selection = new Set();
    const draw = () => chart.setOption(options(host, props, tiles, selection));
    // A resize can change the size of the labels.
    const resize = new ResizeObserver(() => {
      chart.resize();
      draw();
    });
    resize.observe(host);
    scheme.addEventListener("change", draw);
    cleanup(() => {
      resize.disconnect();
      scheme.removeEventListener("change", draw);
      chart.dispose();
    });
    defineHostProp("data", {
      set(rows) {
        for (const [symbol, price, change] of rows ?? []) {
          tiles.set(symbol, { price, change });
        }
        draw();
      },
    });
    defineHostProp("selection", {
      set(indexes) {
        selection = new Set(typeof indexes === "string" ? indexes.split("-") : []);
        draw();
      },
    });
    draw();
  },
});

// <market-map> draws sector tiles colored by their change since open.
//
// Apache ECharts
// Copyright 2017-2026 The Apache Software Foundation
//
// This product includes software developed at
// The Apache Software Foundation (https://www.apache.org/).
//
// Licensed under the Apache License 2.0.
//
// sectors contains [symbol, name] pairs by sector. tiles contains
// [symbol, price, percent change] rows. selection outlines hyphen-separated symbols.
// scale is the percent change that reaches full color.
import { rocket } from "./datastar-rocket.js";

const E = window.echarts;
const font = "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
const scheme = matchMedia("(prefers-color-scheme: dark)");

// colorsOf reads theme colors from page custom properties.
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

// mix interpolates hex colors by t.
const mix = (a, b, t) => {
  const rgb = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
  const [from, to] = [rgb(a), rgb(b)];
  return `rgb(${from.map((v, i) => Math.round(v + (to[i] - v) * t)).join(",")})`;
};

const percent = (v) => `${v > 0 ? "+" : ""}${v.toFixed(2)}%`;
const escape = (s) => String(s).replace(/[&<>"]/g, (c) => `&#${c.charCodeAt(0)};`);

const options = (host, { sectors, scale, tiles: rows, selection: symbols }) => {
  const c = colorsOf(host);
  // The JSON codec decodes null as {}.
  const tiles = new Map(
    (Array.isArray(rows) ? rows : []).map(([symbol, price, change]) => [symbol, { price, change }]),
  );
  const selection = new Set(symbols.split("-"));
  const small = host.clientWidth < 500;
  const data = sectors.map((sector) => ({
    name: sector.name,
    // The sector color shows in gaps and behind its name.
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
      // Keep phone tooltips within the viewport-width map.
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
        // Sector nodes have no tile and label their children.
        formatter: ({ data }) => {
          if (!data.tile) {
            return data.name;
          }
          return data.tile.change == null
            ? `{symbol|${data.name}}`
            : `{symbol|${data.name}}\n{change|${percent(data.tile.change)}}`;
        },
        rich: {
          symbol: {
            fontSize: small ? 11 : 13,
            fontWeight: 700,
            lineHeight: small ? 14 : 18,
            color: "#fff"
          },
          change: {
            fontSize: small ? 10 : 11,
            lineHeight: small ? 12 : 15,
            color: "#fff"
          },
        },
      },
      upperLabel: {
        show: true,
        height: 20,
        color: c.muted,
        fontSize: 11,
        fontWeight: 600
      },
      levels: [
        // Border colors paint gaps between sectors and tiles.
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
  props: ({ json, number, string }) => ({
    sectors: json.default(() => []),
    scale: number.default(10),
    tiles: json.default(() => []),
    selection: string,
  }),
  // Rendering again would empty ECharts' box.
  renderOnPropChange: false,
  render: ({ html }) => html`
    <style>:host{display:block;position:relative}.box{position:absolute;inset:0}</style>
    <div class="box" data-ref:box></div>
  `,
  onFirstRender: ({ host, props, refs, cleanup, observeProps }) => {
    const chart = E.init(refs.box);
    const draw = () => chart.setOption(options(host, props));
    // Resize also recalculates label sizes.
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
    observeProps(draw, "tiles", "selection");
    draw();
  },
});

// <pf-chart> draws a panel of the portfolio page with uPlot.
//
// uPlot, Copyright (c) Leon Sorokin. https://github.com/leeoniya/uPlot
// Licensed under the MIT License.
//
// kind selects the chart.
//
//   - lines draws 1 line per label.
//   - stacked draws areas totaling 100.
//   - area fills 1 series towards 0.
//   - spark draws 1 small line without axes.
//   - bars draws [label, value] columns.
//   - histogram draws [x, count] columns.
//
// unit formats axes. data contains [Unix second, values...], [label, value] or
// [x, count] rows. Each update replaces all rows. Time charts share their cursor.
import { rocket } from "./datastar-rocket.js";

const U = window.uPlot;
const font = "11px system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
const scheme = matchMedia("(prefers-color-scheme: dark)");

// Hide a stacked legend beyond this number of series.
const maxLegend = 12;

// colorsOf reads theme colors from page custom properties.
const colorsOf = (el) => {
  const css = getComputedStyle(el);
  const color = (name) => css.getPropertyValue(name).trim();
  return {
    text: color("--chart-text"),
    grid: color("--chart-grid"),
    muted: color("--muted"),
    line: color("--line"),
    lineFill: color("--line-fill"),
    up: color("--up"),
    down: color("--down"),
    downFill: color("--down-fill"),
    palette: Array.from({ length: 10 }, (_, i) => color(`--c${i}`)),
  };
};

const formatOf = (unit) => {
  if (unit === "$") {
    return (v) => (v < 0 ? "-$" : "$") + Math.round(Math.abs(v)).toLocaleString("en-US");
  }
  return (v) => `${+v.toFixed(3)}${unit}`;
};

const legendFormatOf = (unit) => (v) =>
  `${v > 0 ? "+" : ""}${v.toFixed(2)}${unit}`;

// timeTicks includes seconds below 1-minute spacing.
const hm = { hour: "2-digit", minute: "2-digit", hourCycle: "h23" };
const hms = { ...hm, second: "2-digit" };
const timeTicks = (u, splits, axis, space, incr) =>
  splits.map((t) => new Date(t * 1000).toLocaleTimeString([], incr < 60 ? hms : hm));

// The JSON codec decodes null as {}.
const rowsOf = (data) => (Array.isArray(data) ? data : []);

const axis = (c, extra) => ({
  stroke: c.text,
  font,
  grid: { stroke: c.grid, width: 1 },
  ticks: { stroke: c.grid, width: 1, size: 4 },
  ...extra,
});

const columns = (kind, labels, rows) => {
  const column = (c) => rows.map((row) => row[c] ?? null);
  switch (kind) {
    case "bars":
      return [
        rows.map((_, i) => i),
        rows.map(([, v]) => (v >= 0 ? v : null)),
        rows.map(([, v]) => (v < 0 ? v : null)),
      ];
    case "stacked": {
      const sums = rows.map(() => 0);
      return [column(0), ...labels.map((_, s) =>
        rows.map((row, i) => (sums[i] += row[s + 1] ?? 0)))];
    }
    case "lines":
      return [column(0), ...labels.map((_, s) => column(s + 1))];
    default:
      return [column(0), column(1)];
  }
};

// current gives formatters the rows held when uPlot draws.
const options = (host, { kind, unit }, labels, current) => {
  const c = colorsOf(host);
  const format = formatOf(unit);
  const values = (u, splits) => splits.map(format);
  // Show the cursor value, or the newest value without a cursor.
  const legendFormat = legendFormatOf(unit);
  const legendValue = (i) => (u, v, s, idx) => {
    const rows = current();
    const value = rows[idx ?? rows.length - 1]?.[i];
    return value == null ? "–" : legendFormat(value);
  };
  const base = {
    width: Math.max(host.clientWidth, 1),
    height: Math.max(host.clientHeight, 1),
    // Leave room for the newest time label.
    padding: [10, 20, 0, 0],
    legend: { show: false },
    cursor: { show: false },
    axes: [axis(c, { size: 30, space: 64, values: timeTicks }), axis(c, { size: 60, values })],
  };
  const bars = (size) => ({ paths: U.paths.bars({ size }), points: { show: false } });
  // Sync only the shared time axis.
  const cursor = (opts) =>
    ({ ...opts, y: false, sync: { key: "portfolio", scales: ["x", null] } });
  const drag = { x: false, y: false };
  switch (kind) {
    case "lines":
      return {
        ...base,
        legend: { show: true },
        cursor: cursor({ drag, points: { size: 6 } }),
        series: [{}, ...labels.map((label, s) => ({
          label,
          value: legendValue(s + 1),
          // The first series is the portfolio; the second is its comparison.
          ...(s === 0 ? { stroke: c.line, width: 2 } : { stroke: c.muted, width: 1.5, dash: [4, 3] }),
        }))],
      };
    case "stacked":
      // Omit values to keep the holdings legend short.
      return {
        ...base,
        legend: { show: labels.length <= maxLegend, live: false },
        cursor: cursor({ drag, points: { show: false } }),
        scales: { y: { range: [0, 100] } },
        // Each area fills the band between its line and the one below.
        bands: labels.slice(1).map((_, s) => ({ series: [s + 2, s + 1] })),
        series: [{}, ...labels.map((label, s) => {
          const color = c.palette[s % c.palette.length];
          return {
            label,
            stroke: color,
            fill: `${color}cc`,
            width: 1,
          };
        })],
      };
    case "area":
      return {
        ...base,
        cursor: cursor({ drag, points: { size: 6 } }),
        scales: { y: { range: (u, min) => [Math.min(min, -0.05) * 1.1, 0] } },
        series: [{}, { label: labels[0], stroke: c.down, fill: c.downFill, width: 1.5 }],
      };
    case "spark":
      return {
        ...base,
        cursor: cursor({ drag, points: { show: false } }),
        padding: [4, 0, 4, 0],
        axes: [{ show: false }, { show: false }],
        series: [{}, {
          stroke: c.line,
          fill: c.lineFill,
          width: 1.5,
          fillTo: (u, s, min) => min,
        }],
      };
    case "bars":
      return {
        ...base,
        scales: {
          x: { time: false, range: (u) => [-0.5, u.data[0].length - 0.5] },
          y: { range: (u, min, max) => U.rangeNum(Math.min(0, min), Math.max(0, max), 0.1, true) },
        },
        axes: [
          axis(c, {
            size: 30,
            splits: (u) => u.data[0],
            // Show only labels that fit.
            values: (u, splits) => {
              const every = Math.ceil(splits.length * 34 / Math.max(u.bbox.width / devicePixelRatio, 1));
              return splits.map((i) => (i % every ? "" : current()[i]?.[0] ?? ""));
            },
          }),
          base.axes[1],
        ],
        series: [
          {},
          { label: "Gain", fill: c.up, stroke: c.up, ...bars([0.6, 48]) },
          { label: "Loss", fill: c.down, stroke: c.down, ...bars([0.6, 48]) },
        ],
      };
    case "histogram":
      return {
        ...base,
        scales: {
          x: {
            time: false,
            range: (u, min, max) => {
              const half = ((u.data[0][1] ?? 1) - (u.data[0][0] ?? 0)) / 2;
              return [min - half, max + half];
            },
          },
        },
        axes: [
          axis(c, { size: 30, values }),
          axis(c, { size: 40, incrs: [1, 2, 5, 10, 20, 50, 100] }),
        ],
        series: [{}, { label: "Bars", fill: c.lineFill, stroke: c.line, ...bars([1, Infinity]) }],
      };
  }
  return base;
};

rocket("pf-chart", {
  props: ({ json, string, oneOf }) => ({
    kind: oneOf("lines", "stacked", "area", "spark", "bars", "histogram"),
    labels: string,
    unit: string,
    data: json.default(() => []),
  }),
  // Rendering again would empty uPlot's box.
  renderOnPropChange: false,
  // Copy uPlot's page stylesheet into the shadow root.
  render: ({ html }) => html`
    ${document.getElementById("uplot-css")?.cloneNode()}
    <style>
      :host{display:block;position:relative}
      .box{position:absolute;inset:0}
      .u-legend{font:${font};color:var(--chart-text);padding-top:2px}
      .u-legend th{font-weight:normal}
      /* The axis already shows the cursor time. */
      .u-legend.u-live .u-series:first-child{display:none}
    </style>
    <div class="box" data-ref:box></div>
  `,
  onFirstRender: ({ host, props, refs, cleanup, observeProps }) => {
    const labels = props.labels.split(",").filter(Boolean);
    let rows = rowsOf(props.data);
    let plot = null;
    // Fit the plot above its legend.
    const fit = () => {
      const legend = plot.root.querySelector(".u-legend");
      const width = host.clientWidth;
      const height = host.clientHeight - (legend?.offsetHeight ?? 0);
      if (width > 0 && height > 0) {
        plot.setSize({ width, height });
      }
    };
    const resize = new ResizeObserver(fit);
    // Rebuild for theme changes, then refit after uPlot lays out the legend.
    const build = () => {
      resize.disconnect();
      plot?.destroy();
      plot = new U(
        options(host, props, labels, () => rows),
        columns(props.kind, labels, rows),
        refs.box
      );
      resize.observe(host);
      const legend = plot.root.querySelector(".u-legend");
      if (legend) {
        resize.observe(legend);
      }
      fit();
    };
    build();
    scheme.addEventListener("change", build);
    cleanup(() => {
      resize.disconnect();
      scheme.removeEventListener("change", build);
      plot.destroy();
    });
    observeProps(() => {
      rows = rowsOf(props.data);
      plot.setData(columns(props.kind, labels, rows));
    }, "data");
  },
});

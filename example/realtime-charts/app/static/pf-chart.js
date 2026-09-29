// <pf-chart> draws a panel of the portfolio page with uPlot.
//
// uPlot, Copyright (c) Leon Sorokin. https://github.com/leeoniya/uPlot
// Licensed under the MIT License.
//
// The kind attribute names the chart:
//
//   - lines: a line for every series, named by the labels attribute;
//   - stacked: the series stacked into areas that add up to 100;
//   - area: one series, filled towards zero;
//   - spark: the first series as a small line without axes;
//   - bars: a column for every [label, value], in the up or down color;
//   - histogram: a column for every [x, count].
//
// The unit attribute, % or $, formats the values of the value axis, and those of the x
// axis of a histogram.
//
// The server decides what a chart shows. It sends the data as signals and the page
// assigns them to the data property with data-effect. A time chart gets rows
// [time, value...], with time in Unix seconds, and merges them into the rows it has: a
// row replaces the one of its time, and the others are added. It keeps the rows of the
// last span seconds, the span attribute. Merging the same rows twice changes nothing. The
// bars and the histogram replace their columns.
//
// The time charts share their cursor: the cursor of one shows its time on all of them.
import { rocket } from "./datastar-rocket.js";

const U = window.uPlot;
const timeKinds = new Set(["lines", "stacked", "area", "spark"]);
const font = "11px system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
const scheme = matchMedia("(prefers-color-scheme: dark)");

// A stacked chart lists its series below the plot up to this many.
const maxLegend = 12;

// colorsOf reads the colors from the custom properties of the page stylesheet, which
// switches them with the color scheme.
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

// A value in the legend shows its sign and two decimals.
const legendFormatOf = (unit) => (v) =>
  `${v > 0 ? "+" : ""}${v.toFixed(2)}${unit}`;

// timeTicks labels the ticks of a time axis in local time, with seconds when they are
// closer than a minute.
const hm = { hour: "2-digit", minute: "2-digit", hourCycle: "h23" };
const hms = { ...hm, second: "2-digit" };
const timeTicks = (u, splits, axis, space, incr) =>
  splits.map((t) => new Date(t * 1000).toLocaleTimeString([], incr < 60 ? hms : hm));

// merge returns the rows of a and b by time, b replacing a row of a with the same time,
// without those older than span seconds before the newest.
const merge = (a, b, span) => {
  const out = [];
  let i = 0;
  let j = 0;
  while (i < a.length || j < b.length) {
    if (j === b.length || (i < a.length && a[i][0] < b[j][0])) {
      out.push(a[i++]);
    } else {
      if (i < a.length && a[i][0] === b[j][0]) {
        i++;
      }
      out.push(b[j++]);
    }
  }
  const newest = out.at(-1)?.[0] ?? 0;
  return out.filter((row) => row[0] > newest - span);
};

const axis = (c, extra) => ({
  stroke: c.text,
  font,
  grid: { stroke: c.grid, width: 1 },
  ticks: { stroke: c.grid, width: 1, size: 4 },
  ...extra,
});

// columns transposes the rows into the arrays uPlot draws, one per series.
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

// The legend and the labels of the bars call rowsOf whenever uPlot draws, which gives
// them the rows the chart holds at that moment.
const options = (host, { kind, unit }, labels, rowsOf) => {
  const c = colorsOf(host);
  const format = formatOf(unit);
  const values = (u, splits) => splits.map(format);
  // The legend shows the value of the series at column i at the cursor, or the newest
  // one.
  const legendFormat = legendFormatOf(unit);
  const legendValue = (i) => (u, v, s, idx) => {
    const rows = rowsOf();
    const value = rows[idx ?? rows.length - 1]?.[i];
    return value == null ? "–" : legendFormat(value);
  };
  const base = {
    width: Math.max(host.clientWidth, 1),
    height: Math.max(host.clientHeight, 1),
    // The padding on the right leaves room for the label of the newest time.
    padding: [10, 20, 0, 0],
    legend: { show: false },
    cursor: { show: false },
    axes: [axis(c, { size: 30, space: 64, values: timeTicks }), axis(c, { size: 60, values })],
  };
  const bars = (size) => ({ paths: U.paths.bars({ size }), points: { show: false } });
  // The cursor of a time chart follows the time under the cursor of the others. Only its
  // vertical line means the same on all of them.
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
          // The first series is the portfolio, the second what it is compared with.
          ...(s === 0 ? { stroke: c.line, width: 2 } : { stroke: c.muted, width: 1.5, dash: [4, 3] }),
        }))],
      };
    case "stacked":
      // The legend names the holdings without values, which keeps it short enough to
      // leave the plot most of the height.
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
            // The axis shows the labels that fit, which with many holdings are only some.
            values: (u, splits) => {
              const every = Math.ceil(splits.length * 34 / Math.max(u.bbox.width / devicePixelRatio, 1));
              return splits.map((i) => (i % every ? "" : rowsOf()[i]?.[0] ?? ""));
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
  props: ({ string, number, oneOf }) => ({
    kind: oneOf("lines", "stacked", "area", "spark", "bars", "histogram"),
    labels: string,
    unit: string,
    span: number,
  }),
  // A render would empty the box that uPlot draws into.
  renderOnPropChange: false,
  // The stylesheet of uPlot does not reach into the shadow root. The page links it with
  // the ID uplot-css, and the chart copies the link.
  render: ({ html }) => html`
    ${document.getElementById("uplot-css")?.cloneNode()}
    <style>
      :host{display:block;position:relative}
      .box{position:absolute;inset:0}
      .u-legend{font:${font};color:var(--chart-text);padding-top:2px}
      .u-legend th{font-weight:normal}
      /* A legend with values starts with the time at the cursor, which the axis shows
         already. */
      .u-legend.u-live .u-series:first-child{display:none}
    </style>
    <div class="box" data-ref:box></div>
  `,
  onFirstRender: ({ host, props, refs, cleanup, defineHostProp }) => {
    const labels = props.labels.split(",").filter(Boolean);
    let rows = [];
    let plot = null;
    // fit sizes the plot to the element, less the legend below it.
    const fit = () => {
      const legend = plot.root.querySelector(".u-legend");
      const width = host.clientWidth;
      const height = host.clientHeight - (legend?.offsetHeight ?? 0);
      if (width > 0 && height > 0) {
        plot.setSize({ width, height });
      }
    };
    const resize = new ResizeObserver(fit);
    // build draws a new plot, which takes the colors of the color scheme. uPlot lays out
    // the legend after the constructor returns. The plot fits the element again once the
    // legend has its height.
    const build = () => {
      resize.disconnect();
      plot?.destroy();
      plot = new U(options(host, props, labels, () => rows), columns(props.kind, labels, rows), refs.box);
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
    defineHostProp("data", {
      set(data) {
        if (!data?.length) {
          return;
        }
        // The signal holds its value in proxies; a copy outlives them.
        const copy = Array.from(data, (row) => [...row]);
        rows = timeKinds.has(props.kind) ? merge(rows, copy, props.span || Infinity) : copy;
        plot.setData(columns(props.kind, labels, rows));
      },
    });
  },
});

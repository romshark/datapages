// <rt-chart> draws the chart of one instrument with TradingView Lightweight Charts.
//
// TradingView Lightweight Charts(TM)
// Copyright (c) 2025 TradingView, Inc. https://www.tradingview.com/
// Licensed under the Apache License 2.0.
//
// The type attribute names the series: candles, bars, line, area, baseline or histogram.
// A baseline chart fills the area between its line and the price in the base attribute.
// The bar attribute is the length of a bar in seconds: the time axis and the crosshair
// show seconds only for bars shorter than a minute. The crosshair shows the date as well,
// since a range of hours can reach into the day before.
//
// The server decides what a chart shows. It sends points as signals and the page assigns
// them to the bars property with data-effect. A point is [time, open, high, low, close]
// for candles and bars and [time, value] for the other types, with time in Unix seconds.
// The element merges the points it receives into the ones it has:
//
//   - a point of another type is skipped: after a change of the chart type the old chart
//     receives the points of the new one until a morph replaces it, and the chart library
//     throws on them;
//   - a point older than the newest one is skipped, it is drawn already;
//   - a point at the time of the newest one replaces it, which is how the bar that is
//     still forming moves;
//   - a newer point is appended.
//
// Merging the same points twice changes nothing: a stream that reconnects sends every bar
// again. A chart that a morph removes drops its series, and the points of the same patch
// then change nothing.
//
// The first points a chart draws fill its width, which shows the time range of the tab
// across the whole chart. New bars then scroll it. Dragging and zooming stop at the first
// bar the chart holds.
//
// A chart takes the wheel, drags and pinches only while it is active, which leaves the
// wheel over the charts to the page. A mouse press activates it before the chart library
// sees the press, which lets the same press drag the chart. On a touch screen a tap
// activates it, which leaves a swipe that starts on the chart to the page. The chart
// library cancels the click of a tap, which is why the tap comes from the pointer events.
// A press or focus anywhere else deactivates the chart. Focus cannot be the activity
// itself: the chart library blurs the focused element on every press on a chart. The
// custom state active marks an active chart, and the crosshair follows the pointer either
// way.
//
// The sync property makes a chart follow the source, the chart the pointer entered last,
// while both have it on. The crosshair of the source moves to the bar of the same time on
// the others, and they show the bars it scrolls or zooms to. Charts hold different
// numbers of bars, one added later fewer, but all of them receive the same bars at the
// same times: the newest bar is the same bar on each. A chart shows the range of the
// source counted back from its own newest bar. A chart that turns sync on or draws its
// first bars takes the range of the source.
import { rocket } from "./datastar-rocket.js";

const LC = window.LightweightCharts;

// A chart keeps at most maxBars. Trimming redraws the whole series, which is why it cuts
// back to keepBars instead of by one bar at a time.
const maxBars = 5000;
const keepBars = 4000;

const hm = { hour: "2-digit", minute: "2-digit", hourCycle: "h23" };
const hms = { ...hm, second: "2-digit" };
const md = { month: "short", day: "numeric" };
const localTime = (t, format) =>
  new Date(t * 1000).toLocaleTimeString([], format);
const localDate = (t, format) =>
  new Date(t * 1000).toLocaleDateString([], format);

// tickMark labels a mark of the time axis. Lightweight Charts puts a mark of a day, a
// month or a year where one begins, which only a window of days reaches.
const tickMark = (t, type) => {
  switch (type) {
    case LC.TickMarkType.Year:
      return localDate(t, { year: "numeric" });
    case LC.TickMarkType.Month:
      return localDate(t, { month: "short" });
    case LC.TickMarkType.DayOfMonth:
      return localDate(t, md);
    case LC.TickMarkType.TimeWithSeconds:
      return localTime(t, hms);
  }
  return localTime(t, hm);
};

const scheme = matchMedia("(prefers-color-scheme: dark)");

// source is the chart the pointer entered last.
const charts = new Set();
let source = null;

// types maps a type to its series definition and to the series options that take the
// colors of the page stylesheet.
const types = {
  candles: [LC.CandlestickSeries, (c) => ({
    upColor: c.up,
    downColor: c.down,
    wickUpColor: c.up,
    wickDownColor: c.down,
    borderVisible: false,
  })],
  bars: [LC.BarSeries, (c) => ({ upColor: c.up, downColor: c.down })],
  line: [LC.LineSeries, (c) => ({ color: c.line, lineWidth: 2 })],
  area: [LC.AreaSeries, (c) => ({
    lineColor: c.line,
    topColor: c.lineFill,
    bottomColor: c.lineFade,
    lineWidth: 2,
  })],
  baseline: [LC.BaselineSeries, (c) => ({
    topLineColor: c.up,
    topFillColor1: c.upFill,
    topFillColor2: c.upFade,
    bottomLineColor: c.down,
    bottomFillColor1: c.downFade,
    bottomFillColor2: c.downFill,
    lineWidth: 2,
  })],
  // The points of a histogram carry their colors, which point sets.
  histogram: [LC.HistogramSeries, () => ({})],
};

// colorsOf reads the colors from the custom properties of the page stylesheet, which
// switches them with the color scheme.
const colorsOf = (el) => {
  const css = getComputedStyle(el);
  const color = (name) => css.getPropertyValue(name).trim();
  return {
    text: color("--chart-text"),
    grid: color("--chart-grid"),
    up: color("--up"),
    down: color("--down"),
    upFill: color("--up-fill"),
    upFade: color("--up-fade"),
    downFill: color("--down-fill"),
    downFade: color("--down-fade"),
    line: color("--line"),
    lineFill: color("--line-fill"),
    lineFade: color("--line-fade"),
  };
};

const chartOptions = (bar) => {
  const crosshairFormat = { ...md, ...(bar < 60 ? hms : hm) };
  return {
    autoSize: true,
    layout: {
      background: { type: LC.ColorType.Solid, color: "transparent" },
      fontSize: 11,
      attributionLogo: false,
    },
    handleScroll: false,
    handleScale: false,
    rightPriceScale: { borderVisible: false },
    timeScale: {
      borderVisible: false,
      timeVisible: true,
      secondsVisible: bar < 60,
      barSpacing: 4,
      rightOffset: 2,
      fixLeftEdge: true,
      // The default formatters show UTC.
      tickMarkFormatter: tickMark,
    },
    localization: {
      timeFormatter: (t) => new Date(t * 1000).toLocaleString([], crosshairFormat),
    },
  };
};

rocket("rt-chart", {
  props: ({ number, oneOf }) => ({
    type: oneOf(...Object.keys(types)),
    bar: number.default(5),
    base: number,
  }),
  // A render would empty the box that the chart library draws into.
  renderOnPropChange: false,
  render: ({ html }) => html`
    <style>:host{display:block;position:relative}.box{position:absolute;inset:0}</style>
    <div class="box" data-ref:box></div>
  `,
  onFirstRender: ({ host, props: { type, bar, base }, refs, cleanup, defineHostProp }) => {
    const chart = LC.createChart(refs.box, chartOptions(bar));
    let series = chart.addSeries(types[type][0],
      type === "baseline" ? { baseValue: { type: "price", price: base } } : {});
    const width = type === "candles" || type === "bars" ? 5 : 2;
    const states = host.attachInternals().states;
    let rows = [];
    let colors = null;
    let fitted = false;
    let updating = false;
    let active = false;
    let tap = null;

    // self is what the other charts reach of this one.
    const self = {
      sync: false,
      // range counts the visible logical range back from the newest bar.
      range() {
        const range = chart.timeScale().getVisibleLogicalRange();
        const newest = rows.length - 1;
        return range && { from: range.from - newest, to: range.to - newest };
      },
      // follow shows a range that range returned on another chart.
      follow(range) {
        if (!range || !series || rows.length === 0) {
          return;
        }
        const newest = rows.length - 1;
        chart.timeScale().setVisibleLogicalRange({
          from: range.from + newest,
          to: range.to + newest,
        });
      },
      // crosshairAt shows the crosshair at the close or the value of the bar of the given
      // time, and hides it when the chart has no such bar.
      crosshairAt(time) {
        const index = time === undefined ? null : chart.timeScale().timeToIndex(time);
        const point = index === null ? null : series.dataByIndex(index);
        if (point) {
          chart.setCrosshairPosition(point.close ?? point.value, time, series);
        } else {
          chart.clearCrosshairPosition();
        }
      },
    };

    // share calls f with every other chart that has sync on, if this one has it on and is
    // the source.
    const share = (f) => {
      if (!self.sync || source !== self) {
        return;
      }
      for (const other of charts) {
        if (other !== self && other.sync) {
          f(other);
        }
      }
    };
    const followSource = () => {
      if (source && source !== self) {
        self.follow(source.range());
      }
    };

    const point = (row) => {
      const [time, a, b, c, d] = row;
      if (row.length === 5) {
        return { time, open: a, high: b, low: c, close: d };
      }
      if (type === "histogram") {
        return { time, value: a, color: a < 0 ? colors.down : colors.up };
      }
      return { time, value: a };
    };

    const draw = () => {
      if (!series) {
        return;
      }
      updating = true;
      series.setData(rows.map(point));
      if (!fitted && rows.length > 0) {
        chart.timeScale().fitContent();
        fitted = true;
        if (self.sync) {
          followSource();
        }
      }
      updating = false;
    };

    const applyTheme = () => {
      colors = colorsOf(host);
      const grid = { color: colors.grid };
      chart.applyOptions({
        layout: { textColor: colors.text },
        grid: { vertLines: grid, horzLines: grid },
      });
      series.applyOptions(types[type][1](colors));
      // A new chart has not drawn its points yet, and the points of a histogram carry the
      // colors of its columns.
      draw();
    };

    // activate gives the wheel, drags and pinches on the chart to the chart library, or
    // leaves them to the page.
    const activate = (on) => {
      if (on === active) {
        return;
      }
      active = on;
      chart.applyOptions({ handleScroll: on, handleScale: on });
      if (on) {
        states.add("active");
      } else {
        states.delete("active");
      }
    };

    chart.subscribeCrosshairMove((param) =>
      share((other) => other.crosshairAt(param.time)));
    // A push moves the range of every chart alike, and the charts do not share that move:
    // a chart that has not drawn the push yet would end up a bar off.
    chart.timeScale().subscribeVisibleLogicalRangeChange(() => {
      if (!updating) {
        const range = self.range();
        share((other) => other.follow(range));
      }
    });

    const listening = new AbortController();
    const on = (target, name, f, capture = false) =>
      target.addEventListener(name, f, { capture, signal: listening.signal });
    on(host, "pointerdown", (e) => {
      tap = e.pointerType === "mouse" ? null : { x: e.clientX, y: e.clientY };
      if (!tap) {
        activate(true);
      }
    });
    on(host, "pointerup", (e) => {
      if (tap && Math.hypot(e.clientX - tap.x, e.clientY - tap.y) < 10) {
        activate(true);
      }
      tap = null;
    });
    on(host, "pointerenter", () => {
      source = self;
    });
    const elsewhere = (e) => {
      if (!e.composedPath().includes(host)) {
        activate(false);
      }
    };
    on(document, "pointerdown", elsewhere, true);
    on(document, "focusin", elsewhere, true);
    on(scheme, "change", applyTheme);
    charts.add(self);
    cleanup(() => {
      listening.abort();
      charts.delete(self);
      if (source === self) {
        source = null;
      }
      chart.remove();
      series = null;
    });
    applyTheme();

    defineHostProp("sync", {
      set(on) {
        on = Boolean(on);
        if (on === self.sync) {
          return;
        }
        self.sync = on;
        if (on) {
          followSource();
        }
      },
    });
    defineHostProp("bars", {
      set(data) {
        if (!data?.length) {
          return;
        }
        const redraw = rows.length === 0;
        const changed = [];
        for (const row of data) {
          if (row.length !== width) {
            continue;
          }
          const last = rows.at(-1);
          if (last && row[0] < last[0]) {
            continue;
          }
          // The signal holds its value in proxies; a copy outlives them.
          const copy = [...row];
          if (last && row[0] === last[0]) {
            rows[rows.length - 1] = copy;
          } else {
            rows.push(copy);
          }
          changed.push(copy);
        }
        if (rows.length > maxBars) {
          rows = rows.slice(-keepBars);
          draw();
        } else if (redraw) {
          draw();
        } else if (series) {
          updating = true;
          for (const row of changed) {
            series.update(point(row));
          }
          updating = false;
        }
      },
    });
  },
});

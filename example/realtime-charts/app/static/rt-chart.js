// <rt-chart> draws the chart of one instrument with TradingView Lightweight Charts.
//
// TradingView Lightweight Charts(TM)
// Copyright (c) 2025 TradingView, Inc. https://www.tradingview.com/
// Licensed under the Apache License 2.0.
//
// type selects the series. base sets the baseline price.
// bar sets bar length in seconds and controls time-axis precision.
//
// bars contains [Unix second, open, high, low, close] or [Unix second, value] rows.
// Incoming rows merge with existing data.
//
//   - A gap replaces all rows.
//   - Older rows are ignored.
//   - The newest row is updated in place.
//   - Later rows are appended.
//
// Repeated rows are idempotent. The chart retains older bars up to maxBars.
//
// Initial data fits the width. Later bars scroll the chart.
//
// Only an active chart handles wheel, drag and pinch gestures.
// Mouse presses and touch taps activate it; focus or a press elsewhere deactivates it.
// Pointer events detect taps because the chart library cancels their click.
//
// sync shares the crosshair and visible range from the last chart under the pointer.
// Ranges are offsets from each chart's newest bar,
// which supports different history lengths.
import { rocket } from "./datastar-rocket.js";

const LC = window.LightweightCharts;

// Trim in batches because each trim redraws the series.
const maxBars = 5000;
const keepBars = 4000;

const hm = { hour: "2-digit", minute: "2-digit", hourCycle: "h23" };
const hms = { ...hm, second: "2-digit" };
const md = { month: "short", day: "numeric" };
const localTime = (t, format) =>
  new Date(t * 1000).toLocaleTimeString([], format);
const localDate = (t, format) =>
  new Date(t * 1000).toLocaleDateString([], format);

// tickMark uses calendar labels at day, month and year boundaries.
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

const charts = new Set();
let source = null;

// types maps names to series constructors and themed options.
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
  // Histogram points carry their colors.
  histogram: [LC.HistogramSeries, () => ({})],
};

// colorsOf reads theme colors from page custom properties.
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
      // Default formatters use UTC.
      tickMarkFormatter: tickMark,
    },
    localization: {
      timeFormatter: (t) => new Date(t * 1000).toLocaleString([], crosshairFormat),
    },
  };
};

rocket("rt-chart", {
  props: ({ bool, json, number, oneOf }) => ({
    type: oneOf(...Object.keys(types)),
    bar: number.default(5),
    base: number,
    sync: bool,
    bars: json.default(() => []),
  }),
  // Rendering again would empty the chart box.
  renderOnPropChange: false,
  render: ({ html }) => html`
    <style>:host{display:block;position:relative}.box{position:absolute;inset:0}</style>
    <div class="box" data-ref:box></div>
  `,
  onFirstRender: ({ host, props, refs, cleanup, observeProps }) => {
    const { type, bar, base } = props;
    const chart = LC.createChart(refs.box, chartOptions(bar));
    let series = chart.addSeries(types[type][0],
      type === "baseline" ? { baseValue: { type: "price", price: base } } : {});
    const states = host.attachInternals().states;
    let rows = [];
    let colors = null;
    let fitted = false;
    let updating = false;
    let active = false;
    let tap = null;

    const self = {
      sync: false,
      // Range offsets are relative to the newest bar.
      range() {
        const range = chart.timeScale().getVisibleLogicalRange();
        const newest = rows.length - 1;
        return range && { from: range.from - newest, to: range.to - newest };
      },
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
      // Hide the crosshair when this chart lacks the source time.
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

    // Share only from the active sync source to other synced charts.
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
      // Redraw to apply histogram point colors and initialize a new chart.
      draw();
    };

    // Activation gives wheel, drag and pinch gestures to the chart.
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
    // Do not share push-driven moves before every chart draws the new bar.
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

    const sync = (on) => {
      if (on === self.sync) {
        return;
      }
      self.sync = on;
      if (on) {
        followSource();
      }
    };
    const merge = (data) => {
      if (!data?.length) {
        return;
      }
      if (rows.length > 0 && data[0][0] > rows.at(-1)[0]) {
        rows = [];
        fitted = false;
      }
      const redraw = rows.length === 0;
      const changed = [];
      for (const row of data) {
        const last = rows.at(-1);
        if (last && row[0] < last[0]) {
          continue;
        }
        if (last && row[0] === last[0]) {
          rows[rows.length - 1] = row;
        } else {
          rows.push(row);
        }
        changed.push(row);
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
    };
    observeProps(() => sync(props.sync), "sync");
    observeProps(() => merge(props.bars), "bars");
    sync(props.sync);
    merge(props.bars);
  },
});

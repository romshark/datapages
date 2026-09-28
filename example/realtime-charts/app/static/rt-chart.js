// <rt-chart> draws the chart of one instrument with TradingView Lightweight
// Charts.
//
// TradingView Lightweight Charts(TM)
// Copyright (c) 2025 TradingView, Inc. https://www.tradingview.com/
// Licensed under the Apache License 2.0.
//
// The type attribute names the series: candles, bars, line, area, baseline
// or histogram. A baseline chart fills the area between its line and the
// price in the base attribute.
//
// The server decides what a chart shows. It sends points as signals and the
// page assigns them to the bars property with data-effect. A point is
// [time, open, high, low, close] for candles and bars and [time, value] for
// the other types, with time in Unix seconds. The element merges the points
// it receives into the ones it has:
//
//   - a point older than the newest one is skipped, it is drawn already;
//   - a point at the time of the newest one replaces it, which is how the bar
//     that is still forming moves;
//   - a newer point is appended.
//
// Merging the same points twice changes nothing. Datastar assigns a signal's
// last value again when a morph moves the element, and a stream that
// reconnects sends every bar again.
//
// Load this script before Datastar applies the page, in a blocking script in
// the head. The elements are then defined when data-effect first assigns to
// them. An assignment to an element that is not defined yet creates a plain
// property, and a later one overwrites it: that element would miss points.
(() => {
  const LC = window.LightweightCharts;
  if (!LC) {
    console.error("rt-chart: TradingView Lightweight Charts did not load");
    return;
  }

  // A chart keeps at most maxBars. Trimming redraws the whole series,
  // which is why it cuts back to keepBars instead of by one bar at a time.
  const maxBars = 5000;
  const keepBars = 4000;

  const hm = { hour: "2-digit", minute: "2-digit", hourCycle: "h23" };
  const hms = { ...hm, second: "2-digit" };
  const localTime = (t, format) =>
    new Date(t * 1000).toLocaleTimeString([], format);

  const scheme = matchMedia("(prefers-color-scheme: dark)");

  // series maps a type to its series definition and to the series options
  // that take the colors of the page stylesheet.
  const series = {
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
    // Histogram points carry their color, see RtChart.#point.
    histogram: [LC.HistogramSeries, () => ({})],
  };

  // colorsOf reads the colors from the custom properties of the page
  // stylesheet, which switches them with the color scheme.
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

  class RtChart extends HTMLElement {
    #type = "candles";
    #rows = [];
    #colors = null;
    #chart = null;
    #series = null;
    #removal = 0;
    #onScheme = () => this.#applyTheme();

    constructor() {
      super();
      this.attachShadow({ mode: "open" }).innerHTML =
        "<style>:host{display:block;position:relative}" +
        "div{position:absolute;inset:0}</style><div></div>";
      if (Object.hasOwn(this, "bars")) {
        const bars = this.bars;
        delete this.bars;
        this.bars = bars;
      }
    }

    connectedCallback() {
      clearTimeout(this.#removal);
      if (this.#chart) {
        return;
      }
      const type = this.getAttribute("type");
      this.#type = Object.hasOwn(series, type) ? type : "candles";
      this.#chart = LC.createChart(this.shadowRoot.querySelector("div"), {
        autoSize: true,
        layout: {
          background: { type: LC.ColorType.Solid, color: "transparent" },
          fontSize: 11,
          attributionLogo: false,
        },
        rightPriceScale: { borderVisible: false },
        timeScale: {
          borderVisible: false,
          timeVisible: true,
          secondsVisible: true,
          barSpacing: 4,
          rightOffset: 2,
          // The default formatters show UTC.
          tickMarkFormatter: (t, type) =>
            localTime(t, type === LC.TickMarkType.TimeWithSeconds ? hms : hm),
        },
        localization: { timeFormatter: (t) => localTime(t, hms) },
      });
      const options = this.#type === "baseline"
        ? { baseValue: { type: "price", price: Number(this.getAttribute("base")) } }
        : {};
      this.#series = this.#chart.addSeries(series[this.#type][0], options);
      this.#applyTheme();
      scheme.addEventListener("change", this.#onScheme);
    }

    disconnectedCallback() {
      // A morph that moves the element disconnects and reconnects it within
      // one task. Only an element that stays removed loses its chart.
      this.#removal = setTimeout(() => {
        scheme.removeEventListener("change", this.#onScheme);
        this.#chart.remove();
        this.#chart = this.#series = null;
      });
    }

    set bars(rows) {
      if (!rows?.length) {
        return;
      }
      const redraw = this.#rows.length === 0;
      const changed = [];
      for (const row of rows) {
        const last = this.#rows.at(-1);
        if (last && row[0] < last[0]) {
          continue;
        }
        // The signal holds its value in proxies; a copy outlives them.
        const copy = [...row];
        if (last && row[0] === last[0]) {
          this.#rows[this.#rows.length - 1] = copy;
        } else {
          this.#rows.push(copy);
        }
        changed.push(copy);
      }
      if (this.#rows.length > maxBars) {
        this.#rows = this.#rows.slice(-keepBars);
        this.#draw();
      } else if (redraw) {
        this.#draw();
      } else if (this.#series) {
        for (const row of changed) {
          this.#series.update(this.#point(row));
        }
      }
    }

    #draw() {
      this.#series?.setData(this.#rows.map((row) => this.#point(row)));
    }

    #point(row) {
      const [time, a, b, c, d] = row;
      if (row.length === 5) {
        return { time, open: a, high: b, low: c, close: d };
      }
      if (this.#type === "histogram") {
        return { time, value: a, color: a < 0 ? this.#colors.down : this.#colors.up };
      }
      return { time, value: a };
    }

    #applyTheme() {
      this.#colors = colorsOf(this);
      const grid = { color: this.#colors.grid };
      this.#chart.applyOptions({
        layout: { textColor: this.#colors.text },
        grid: { vertLines: grid, horzLines: grid },
      });
      this.#series.applyOptions(series[this.#type][1](this.#colors));
      // A new chart has not drawn its points yet, and the points of a
      // histogram carry the colors of its columns.
      this.#draw();
    }
  }

  customElements.define("rt-chart", RtChart);
})();

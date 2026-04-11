// <wc-counter from="10h"> — counts down to zero.
//
// The `from` attribute accepts a simple duration string composed of
// one or more `<number><unit>` pairs, e.g. "10h", "1h30m", "45s",
// "2h15m30s". Supported units: h, m, s.
//
// Rendering is done into Shadow DOM so the host page cannot style or
// mutate the internal markup.

const UNIT_SECONDS: Record<string, number> = {
  h: 3600,
  m: 60,
  s: 1,
};

function parseDuration(input: string): number {
  const src = input.trim().toLowerCase();
  if (src === "") return 0;
  const re = /(\d+)\s*([hms])/g;
  let total = 0;
  let matched = 0;
  let m: RegExpExecArray | null;
  while ((m = re.exec(src)) !== null) {
    total += parseInt(m[1], 10) * UNIT_SECONDS[m[2]];
    matched += m[0].length;
  }
  if (matched === 0) {
    const n = parseInt(src, 10);
    return Number.isFinite(n) ? n : 0;
  }
  return total;
}

function format(remaining: number): {
  hh: string;
  mm: string;
  ss: string;
} {
  const clamped = Math.max(0, Math.floor(remaining));
  const hh = Math.floor(clamped / 3600);
  const mm = Math.floor((clamped % 3600) / 60);
  const ss = clamped % 60;
  const pad = (n: number) => n.toString().padStart(2, "0");
  return { hh: pad(hh), mm: pad(mm), ss: pad(ss) };
}

const TEMPLATE = `
<style>
  :host {
    display: inline-flex;
    gap: 0.5rem;
    font-family: var(--wc-counter-font, ui-monospace, monospace);
    font-variant-numeric: tabular-nums;
    color: var(--wc-counter-color, #fff);
  }
  .cell {
    display: flex;
    flex-direction: column;
    align-items: center;
    min-width: 3.25rem;
    padding: 0.5rem 0.75rem;
    border-radius: 0.75rem;
    background: var(--wc-counter-bg, rgba(0, 0, 0, 0.55));
    box-shadow: 0 1px 0 rgba(255, 255, 255, 0.08) inset,
                0 8px 24px rgba(0, 0, 0, 0.25);
  }
  .num {
    font-size: var(--wc-counter-num-size, 2rem);
    font-weight: 700;
    line-height: 1;
    letter-spacing: -0.02em;
  }
  .lbl {
    margin-top: 0.25rem;
    font-size: 0.65rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    opacity: 0.7;
  }
  :host([done]) .cell {
    background: var(--wc-counter-bg-done, rgba(185, 28, 28, 0.85));
  }
</style>
<div class="cell"><span class="num" data-slot="hh">00</span><span class="lbl">hours</span></div>
<div class="cell"><span class="num" data-slot="mm">00</span><span class="lbl">min</span></div>
<div class="cell"><span class="num" data-slot="ss">00</span><span class="lbl">sec</span></div>
`;

export class WcCounter extends HTMLElement {
  static get observedAttributes() {
    return ["from"];
  }

  private remaining = 0;
  private timerId: number | null = null;
  private readonly hhEl: HTMLElement;
  private readonly mmEl: HTMLElement;
  private readonly ssEl: HTMLElement;

  constructor() {
    super();
    // Shadow DOM must be set up in the constructor so refs exist before
    // attributeChangedCallback runs during upgrade (it fires before
    // connectedCallback when the element is parsed with attributes already set).
    const root = this.attachShadow({ mode: "open" });
    root.innerHTML = TEMPLATE;
    this.hhEl = root.querySelector('[data-slot="hh"]') as HTMLElement;
    this.mmEl = root.querySelector('[data-slot="mm"]') as HTMLElement;
    this.ssEl = root.querySelector('[data-slot="ss"]') as HTMLElement;
  }

  connectedCallback() {
    this.reset();
    this.start();
  }

  disconnectedCallback() {
    this.stop();
  }

  attributeChangedCallback(name: string) {
    if (name !== "from") return;
    this.reset();
    if (this.isConnected) this.start();
  }

  private reset() {
    this.remaining = parseDuration(this.getAttribute("from") ?? "");
    this.render();
    this.toggleAttribute("done", this.remaining <= 0);
  }

  private start() {
    this.stop();
    if (this.remaining <= 0) return;
    this.timerId = window.setInterval(() => this.tick(), 1000);
  }

  private stop() {
    if (this.timerId !== null) {
      window.clearInterval(this.timerId);
      this.timerId = null;
    }
  }

  private tick() {
    this.remaining -= 1;
    if (this.remaining <= 0) {
      this.remaining = 0;
      this.stop();
      this.toggleAttribute("done", true);
      this.dispatchEvent(
        new CustomEvent("wc-counter:done", { bubbles: true, composed: true }),
      );
    }
    this.render();
  }

  private render() {
    const { hh, mm, ss } = format(this.remaining);
    this.hhEl.textContent = hh;
    this.mmEl.textContent = mm;
    this.ssEl.textContent = ss;
  }
}

if (!customElements.get("wc-counter")) {
  customElements.define("wc-counter", WcCounter);
}

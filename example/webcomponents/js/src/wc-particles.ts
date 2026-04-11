// <wc-particles> — firework burst of character sprites. Built with Lit.
//
// Attributes (mirrored as reactive properties):
//   characters — glyphs to pick from (default "✨").
//   particles  — count per emit() call (default 10).
//   radius     — burst radius in px; 0 = auto-scale to slot size.
//
// Usage: call emit() to fire a burst, e.g.
//   <wc-particles data-ref="p" characters="🦆⭐" radius="180">
//     <a data-on:click="$p.emit()">Buy</a>
//   </wc-particles>

import { LitElement, html, css, type PropertyValues } from "lit";

function pick<T>(arr: readonly T[]): T {
  return arr[(Math.random() * arr.length) | 0];
}

function splitChars(s: string): string[] {
  // Spread iterates code points so surrogate pairs (🦆) stay intact.
  return [...s];
}

export class WcParticles extends LitElement {
  static override styles = css`
    :host {
      position: relative;
      display: inline-block;
    }
    .layer {
      position: absolute;
      inset: 0;
      overflow: visible;
      pointer-events: none;
      z-index: 10;
    }
    .p {
      position: absolute;
      left: var(--x0);
      top: var(--y0);
      line-height: 1;
      pointer-events: none;
      user-select: none;
      font-size: var(--size);
      transform-origin: center;
      will-change: transform, opacity;
      filter: drop-shadow(0 0 6px rgba(255, 220, 120, 0.55));
      /* Linear timing; keyframes encode ease-out burst + ease-in fall. */
      animation: wc-particles-firework var(--dur) linear forwards;
    }
    @keyframes wc-particles-firework {
      0% {
        transform: translate(-50%, -50%) scale(0.3) rotate(0deg);
        opacity: 0;
      }
      6% {
        transform: translate(-50%, -50%) scale(1.35) rotate(0deg);
        opacity: 1;
      }
      30% {
        transform:
          translate(calc(-50% + var(--dx) * 0.78), calc(-50% + var(--dy) * 0.78))
          scale(1)
          rotate(calc(var(--rot) * 0.3));
        opacity: 1;
      }
      60% {
        transform:
          translate(calc(-50% + var(--dx)), calc(-50% + var(--dy) + var(--fall) * 0.15))
          scale(0.95)
          rotate(calc(var(--rot) * 0.6));
        opacity: 1;
      }
      100% {
        transform:
          translate(calc(-50% + var(--dx)), calc(-50% + var(--dy) + var(--fall)))
          scale(0.55)
          rotate(var(--rot));
        opacity: 0;
      }
    }
  `;

  static override properties = {
    characters: { type: String },
    particles: { type: Number },
    radius: { type: Number },
  };

  declare characters: string;
  declare particles: number;
  declare radius: number;

  constructor() {
    super();
    this.characters = "✨";
    this.particles = 10;
    this.radius = 0;
  }

  private layer: HTMLElement | null = null;

  override render() {
    return html`
      <slot></slot>
      <div class="layer"></div>
    `;
  }

  override firstUpdated(_changed: PropertyValues) {
    this.layer = this.renderRoot.querySelector(".layer");
  }

  emit(): void {
    const chars = splitChars(this.characters);
    if (chars.length === 0) return;
    if (!this.layer) return;

    const count = Math.max(1, Math.floor(this.particles) || 10);

    const rect = this.getBoundingClientRect();
    const w = rect.width;
    const h = rect.height;
    // Single origin at slot center = reads as a firework, not a sparkle field.
    const originX = w / 2;
    const originY = h / 2;
    // Explicit radius wins; otherwise scale to the slot's longest edge.
    const radius =
      this.radius > 0 ? this.radius : Math.max(70, Math.max(w, h) * 0.9);

    for (let i = 0; i < count; i++) {
      const p = document.createElement("span");
      p.className = "p";
      p.textContent = pick(chars);

      // sqrt biases distance toward the outer ring (cleaner silhouette).
      const angle = Math.random() * Math.PI * 2;
      const dist = radius * (0.55 + Math.sqrt(Math.random()) * 0.7);
      const dx = Math.cos(angle) * dist;
      const dy = Math.sin(angle) * dist;

      const fall = 40 + Math.random() * 60;
      const rot = (Math.random() - 0.5) * 180;
      const dur = 900 + Math.random() * 500;
      const size = 1 + Math.random() * 1.3;

      const style = p.style;
      style.setProperty("--x0", `${originX.toFixed(1)}px`);
      style.setProperty("--y0", `${originY.toFixed(1)}px`);
      style.setProperty("--dx", `${dx.toFixed(1)}px`);
      style.setProperty("--dy", `${dy.toFixed(1)}px`);
      style.setProperty("--fall", `${fall.toFixed(1)}px`);
      style.setProperty("--rot", `${rot.toFixed(1)}deg`);
      style.setProperty("--dur", `${dur.toFixed(0)}ms`);
      style.setProperty("--size", `${size.toFixed(2)}rem`);

      p.addEventListener("animationend", () => p.remove(), { once: true });
      this.layer.appendChild(p);
    }
  }
}

if (!customElements.get("wc-particles")) {
  customElements.define("wc-particles", WcParticles);
}

import Quill from "quill";
import type { Sync } from "./sync";

// RemoteCursor is the cursor of another editor at the last revision received.
export interface RemoteCursor {
  name: string;
  color: string;
  index: number;
  length: number;
}

const { TextBlot } = Quill.import("parchment");

// Cursors draws the cursors and selections of the other editors over the text,
// mapping their positions from the last revision received to the editor.
export class Cursors {
  private list: RemoteCursor[] = [];
  private readonly quill: Quill;
  private readonly sync: Sync;
  private readonly layer: HTMLElement;

  constructor(quill: Quill, sync: Sync) {
    this.quill = quill;
    this.sync = sync;
    this.layer = quill.addContainer("remote-cursors");
    new ResizeObserver(() => this.render()).observe(quill.root);
  }

  set(list: RemoteCursor[]): void {
    this.list = list;
    this.render();
  }

  render(): void {
    const last = this.quill.getLength() - 1;
    const origin = this.quill.container.getBoundingClientRect();
    const nodes: HTMLElement[] = [];
    for (const c of this.list) {
      const start = Math.min(this.sync.toLocal(c.index), last);
      const end = Math.max(start, Math.min(this.sync.toLocal(c.index + c.length), last));
      for (const r of this.textRects(start, end - start)) {
        const highlight = document.createElement("div");
        highlight.className = "remote-selection";
        place(highlight, r.left - origin.left, r.top - origin.top, r.width, r.height);
        highlight.style.backgroundColor = c.color;
        nodes.push(highlight);
      }
      const at = this.quill.getBounds(end);
      if (at === null) {
        continue;
      }
      const caret = document.createElement("div");
      caret.className = "remote-caret";
      place(caret, at.left, at.top, 2, at.height);
      caret.style.backgroundColor = c.color;
      const flag = document.createElement("div");
      flag.className = "remote-flag";
      flag.textContent = c.name;
      flag.style.backgroundColor = c.color;
      caret.append(flag);
      nodes.push(caret);
    }
    this.layer.replaceChildren(...nodes);
  }

  // textRects returns the boxes of the text between index and index+length,
  // one range per text node: a range over whole paragraphs also returns the
  // paragraph boxes.
  private textRects(index: number, length: number): DOMRect[] {
    if (length <= 0) {
      return [];
    }
    const rects: DOMRect[] = [];
    const range = document.createRange();
    for (const leaf of this.quill.scroll.descendants(TextBlot, index, length)) {
      const at = this.quill.getIndex(leaf);
      const from = Math.max(index, at) - at;
      const to = Math.min(index + length, at + leaf.length()) - at;
      if (to <= from) {
        continue;
      }
      range.setStart(leaf.domNode, from);
      range.setEnd(leaf.domNode, to);
      rects.push(...range.getClientRects());
    }
    return rects;
  }
}

function place(
  el: HTMLElement,
  left: number,
  top: number,
  width: number,
  height: number,
) {
  el.style.left = `${left}px`;
  el.style.top = `${top}px`;
  el.style.width = `${width}px`;
  el.style.height = `${height}px`;
}

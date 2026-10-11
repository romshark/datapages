import Quill from "quill";

// maxWidth is the largest width the server accepts in each unit of the width field,
// as widthPattern in richtext/format.go. A width in pixels is a bare number.
const maxWidth = { "%": 100, px: 9999 };
type Unit = keyof typeof maxWidth;

// ImageToolbar shows the controls of the selected image below it. The width field
// sets the width of the image in the chosen unit, an empty field its own width.
// A button with data-align sets the alignment of its line.
export class ImageToolbar {
  private readonly quill: Quill;
  private readonly bar: HTMLElement;
  private readonly width: HTMLInputElement;
  private readonly unit: HTMLSelectElement;
  // img is the image the toolbar is for. It stays while the focus is in the
  // toolbar: focusing a field takes the selection out of the editor.
  private img: HTMLImageElement | null = null;

  constructor(quill: Quill, bar: HTMLElement) {
    this.quill = quill;
    this.bar = bar;
    this.width = bar.querySelector<HTMLInputElement>(".image-width input")!;
    this.unit = bar.querySelector<HTMLSelectElement>(".image-unit")!;
    quill.container.append(bar);
    // Quill puts the cursor next to a clicked image. Selecting the image instead
    // shows the toolbar.
    quill.root.addEventListener("click", (e) => {
      if (!(e.target instanceof HTMLImageElement) || !quill.isEnabled()) {
        return;
      }
      const blot = Quill.find(e.target);
      if (blot && !(blot instanceof Quill)) {
        quill.setSelection(quill.getIndex(blot), 1, "user");
      }
    });
    // The editor keeps the focus, and with it its selection, unless a field takes it.
    bar.addEventListener("mousedown", (e) => {
      if (!(e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement)) {
        e.preventDefault();
      }
    });
    // The focus leaving the toolbar arrives at its target only after focusout.
    bar.addEventListener("focusout", (e) => {
      if (!bar.contains(e.relatedTarget as Node | null)) {
        setTimeout(() => this.update());
      }
    });
    bar.addEventListener("click", (e) => this.align(e));
    this.width.addEventListener("change", () => this.applyWidth());
    this.width.addEventListener("keydown", (e) => this.key(e));
    this.unit.addEventListener("change", () => this.changeUnit());
    new ResizeObserver(() => this.update()).observe(quill.root);
  }

  // update shows the toolbar below the image with its width and alignment,
  // or hides it unless the selection is 1 image or a field has the focus.
  update(): void {
    if (!this.bar.contains(document.activeElement)) {
      const range = this.quill.isEnabled() ? this.quill.getSelection() : null;
      this.img = range?.length === 1 ? this.imageAt(range.index) : null;
    }
    const index = this.index();
    this.bar.hidden = index === null;
    if (index === null) {
      return;
    }
    // A change from another tab leaves alone what is being typed.
    if (document.activeElement !== this.width) {
      this.show(this.widthOf(index));
    }
    const align = this.quill.getFormat(index, 1).align ?? "";
    for (const b of this.bar.querySelectorAll<HTMLButtonElement>("button[data-align]")) {
      b.setAttribute("aria-pressed", String(b.dataset.align === align));
    }
    const at = this.quill.getBounds(index, 1);
    if (!at) {
      return;
    }
    const maxLeft = this.quill.container.clientWidth - this.bar.offsetWidth;
    this.bar.style.left = `${Math.max(0, Math.min(at.left, maxLeft))}px`;
    this.bar.style.top = `${at.bottom + 8}px`;
  }

  // index returns the index of the image of the toolbar, null if there is none
  // or the editor is read-only.
  private index(): number | null {
    if (!this.img?.isConnected || !this.quill.isEnabled()) {
      return null;
    }
    const blot = Quill.find(this.img);
    return blot && !(blot instanceof Quill) ? this.quill.getIndex(blot) : null;
  }

  // imageAt returns the image element at index, null for anything else.
  private imageAt(index: number): HTMLImageElement | null {
    const [leaf] = this.quill.getLeaf(index);
    return leaf?.domNode instanceof HTMLImageElement ? leaf.domNode : null;
  }

  // widthOf returns the width format of the image at index, empty for none.
  private widthOf(index: number): string {
    const width = this.quill.getContents(index, 1).ops[0]?.attributes?.width;
    return typeof width === "string" ? width : "";
  }

  // show puts width into the width field and its unit. An empty width keeps the unit.
  private show(width: string) {
    if (width !== "") {
      this.unit.value = width.endsWith("%") ? "%" : "px";
    }
    this.width.max = String(maxWidth[this.unit.value as Unit]);
    this.width.value = width.replace("%", "");
  }

  // applyWidth gives the image the width in the field, rounded and limited to
  // what the server accepts, or its own width for an empty field.
  private applyWidth() {
    const index = this.index();
    if (index === null) {
      return;
    }
    if (this.width.validity.badInput) {
      this.show(this.widthOf(index));
      return;
    }
    let width = "";
    if (this.width.value !== "") {
      const unit = this.unit.value as Unit;
      const n = Math.min(Math.max(Math.round(this.width.valueAsNumber), 1), maxWidth[unit]);
      width = unit === "%" ? `${n}%` : String(n);
    }
    // Quill reports a format as a change even when the value stays, and the
    // editor would send it to the server.
    if (width !== this.widthOf(index)) {
      this.quill.formatText(index, 1, "width", width || null, "user");
    }
    this.show(width);
  }

  // changeUnit converts the width to the chosen unit from the size the image has
  // on the page, which keeps it. An image of its own width keeps it.
  private changeUnit() {
    const index = this.index();
    if (index === null || this.width.value === "") {
      this.width.max = String(maxWidth[this.unit.value as Unit]);
      return;
    }
    const px = this.img!.getBoundingClientRect().width;
    const n = this.unit.value === "%" ? (px / this.lineWidth(index)) * 100 : px;
    this.width.value = String(Math.round(n));
    this.applyWidth();
  }

  // lineWidth returns the width of the content box of the line at index,
  // to which a width in percent refers.
  private lineWidth(index: number): number {
    const node = this.quill.getLine(index)[0]?.domNode;
    if (!(node instanceof HTMLElement)) {
      return this.quill.root.clientWidth;
    }
    const style = getComputedStyle(node);
    return node.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
  }

  // key applies the width field on Enter and drops its edit on Escape.
  // Both return the focus to the editor with the image selected.
  private key(e: KeyboardEvent) {
    if (e.key !== "Enter" && e.key !== "Escape") {
      return;
    }
    e.preventDefault();
    const index = this.index();
    if (index === null) {
      return;
    }
    if (e.key === "Enter") {
      this.applyWidth();
    } else {
      this.show(this.widthOf(index));
    }
    this.quill.setSelection(index, 1, "user");
  }

  private align(e: MouseEvent) {
    const button = (e.target as Element).closest<HTMLButtonElement>("button[data-align]");
    const index = this.index();
    if (!button || index === null) {
      return;
    }
    this.quill.formatLine(index, 1, "align", button.dataset.align || false, "user");
    this.update();
  }
}

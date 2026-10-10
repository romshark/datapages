// <click-counter step="1">Label</click-counter> counts clicks and dispatches
// count-change with the count in detail.
//
// It shows how a component and the page divide the work: attributes come down
// from the template, events go up to a data-on attribute of the page.
// A counter alone needs no component, a local signal does the same.
class ClickCounter extends HTMLElement {
  // A morph changes step when the server renders another value.
  static observedAttributes = ["step"];
  private count = 0;
  private step = 1;

  connectedCallback() {
    if (this.shadowRoot) {
      return;
    }
    // A morph does not enter the shadow root. The slot shows the label.
    const button = document.createElement("button");
    button.type = "button";
    button.append(document.createElement("slot"));
    button.addEventListener("click", () => {
      this.count += this.step;
      this.dispatchEvent(new CustomEvent("count-change", { detail: { count: this.count } }));
    });
    this.attachShadow({ mode: "open" }).append(button);
  }

  attributeChangedCallback(_name: string, _old: string | null, value: string | null) {
    this.step = Number(value ?? 1);
  }
}

customElements.define("click-counter", ClickCounter);

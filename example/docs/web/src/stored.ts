// <docs-stored> lists the documents this browser stores, for the offline page:
// the newest title of each, and whether the server lacks changes made to it.
import { TabStore } from "./local";

class DocsStored extends HTMLElement {
  async connectedCallback() {
    const tabs = await TabStore.open();
    const docs = new Map<string, { title: string; unsaved: boolean }>();
    for (const r of tabs ? await tabs.list() : []) {
      const doc = docs.get(r.url) ?? { title: r.title, unsaved: false };
      if (r.sent !== null) {
        doc.unsaved = true;
      }
      docs.set(r.url, doc);
    }
    if (docs.size === 0) {
      const empty = document.createElement("p");
      empty.className = "empty";
      empty.textContent = "None yet. Documents opened while online are stored.";
      this.replaceChildren(empty);
      return;
    }
    const list = document.createElement("ul");
    for (const [url, doc] of docs) {
      const link = document.createElement("a");
      link.href = url;
      link.textContent = doc.title;
      const item = document.createElement("li");
      item.append(link);
      if (doc.unsaved) {
        const note = document.createElement("span");
        note.textContent = "Has changes the server lacks";
        item.append(note);
      }
      list.append(item);
    }
    this.replaceChildren(list);
  }
}

customElements.define("docs-stored", DocsStored);

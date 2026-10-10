// <docs-editor client="..." rev="12" url="/doc/..." doc-title="..."> is a document editor.
//
// It holds what Datastar cannot: typed text the server lacks, and the transformation
// that merges the changes of others into it. The page holds the rest. The element sends
// events the page turns into actions. The stream calls receive and stop, the image
// action insertImage, the restore action restored.
//
// It stores its state in the browser after every change, see local.ts, under url,
// the address of the document page, with doc-title for the list of stored documents.
// On load it sends the restore action what closed tabs of the document left unconfirmed.
// <docs-editor offline url="..."> opens the document at url from that storage,
// for the offline page, and sends nothing until the page loads from the server again.
//
// Events:
//   - docs-submit: detail is a Submission for the submit action.
//   - docs-cursor: detail is the cursor for the cursor action.
//   - docs-state: detail is {rev, saving, stopped} for the page signals.
//   - docs-image: detail is {upload, data} for the image action, data in base64.
//   - docs-restore: detail is the Pending of a closed tab with its client, rev,
//     title and content, for the restore action.
//   - docs-offline: detail is {shown, title}, shown "doc" or "list",
//     for the offline page.
//   - docs-reconnect: the server answers again, for the offline page to reload from it.
//   - docs-notice: detail is a message for the page to show.
import Quill, { Delta } from "quill";
import { Cursors, type RemoteCursor } from "./cursors";
import { ImageToolbar } from "./image-toolbar";
import { acquire, lockWait, newClient, TabStore, type TabRecord } from "./local";
import { Sync, type Change, type Pending, type Submission } from "./sync";

// formats are the formats the server accepts, as in richtext/format.go.
// Quill drops others from pasted text.
const formats = [
  "bold",
  "italic",
  "underline",
  "strike",
  "code",
  "link",
  "color",
  "background",
  "header",
  "list",
  "blockquote",
  "code-block",
  "align",
  "indent",
  "image",
];

// imageTypes are the types the server stores, as in images/images.go.
const imageTypes = ["image/png", "image/jpeg", "image/gif", "image/webp"];

// colorPattern copies colorPattern in richtext/format.go.
const colorPattern =
  /^(#[0-9a-fA-F]{3,8}|[a-zA-Z]{1,30}|(rgb|rgba|hsl|hsla)\([0-9a-z.,%/ ]{1,60}\))$/;

// maxLinkLength copies maxLinkLen in richtext/format.go.
const maxLinkLength = 2048;

// widthPattern copies widthPattern in richtext/format.go.
const widthPattern = /^((100|[1-9][0-9]?)%|[1-9][0-9]{0,3})$/;

// Pasted colors and links the server rejects would stop the editor.
// Quill drops a color its attributor cannot add and replaces a link its Link format
// sanitizes away with about:blank. The clipboard reads pasted colors with
// MirroredColor and falls back to these attributors.
for (const name of ["attributors/style/color", "attributors/style/background"]) {
  const attributor = Quill.import(name) as {
    canAdd(node: HTMLElement, value: unknown): boolean;
  };
  const canAdd = attributor.canAdd.bind(attributor);
  attributor.canAdd = (node, value) => colorPattern.test(String(value)) && canAdd(node, value);
}
const Link = Quill.import("formats/link") as { sanitize(url: string): string };
const sanitize = Link.sanitize.bind(Link);
Link.sanitize = (url) =>
  url.length > maxLinkLength || /[\x00-\x1f\x7f]/.test(url) ? "about:blank" : sanitize(url);

// Quill removes an image from a code block. After a remote change put one there, the
// editor would differ from the server without telling it. Code blocks keep images instead.
const CodeBlock = Quill.import("formats/code-block") as { allowedChildren: unknown[] };
CodeBlock.allowedChildren = [...CodeBlock.allowedChildren, Quill.import("formats/image")];

const Parchment = Quill.import("parchment");
const ColorAttributor = (
  Quill.import("attributors/style/color") as InstanceType<typeof Parchment.StyleAttributor>
).constructor as typeof Parchment.StyleAttributor;

// MirroredColor also writes a text or highlight color to a custom property.
// A stylesheet cannot read an inline style but can read a custom property,
// from which dark mode in editor.css derives a readable color.
// The property keeps the color as given.
// The inline style holds the rgb() form the browser converts it to.
class MirroredColor extends ColorAttributor {
  private readonly property: string;

  constructor(attrName: string, keyName: string, property: string) {
    super(attrName, keyName, { scope: Parchment.Scope.INLINE });
    this.property = property;
  }

  override canAdd(node: HTMLElement, value: unknown): boolean {
    return colorPattern.test(String(value)) && super.canAdd(node, value);
  }

  override add(node: HTMLElement, value: unknown): boolean {
    if (!super.add(node, value)) {
      return false;
    }
    node.style.setProperty(this.property, String(value));
    return true;
  }

  override remove(node: HTMLElement): void {
    node.style.removeProperty(this.property);
    super.remove(node);
  }

  override value(node: HTMLElement): string {
    const value = node.style.getPropertyValue(this.property).trim();
    return value !== "" && this.canAdd(node, value) ? value : super.value(node);
  }
}

Quill.register("formats/color", new MirroredColor("color", "color", "--color"), true);
Quill.register(
  "formats/background",
  new MirroredColor("background", "background-color", "--background"),
  true,
);

// resendAfter is how long the editor waits for the stream to return a change
// before resending it. Datastar retries failed connections. This covers a
// response lost after the server applied the change, or a stream that was down.
const resendAfter = 10_000;

// cursorDelay batches the cursor moves of fast typing and selecting.
const cursorDelay = 100;

// saveDelay is how often at most the editor stores its state while it changes.
// A crash loses what the user typed since.
const saveDelay = 500;

// restoreWait is how long the editor holds the lock of the record of a closed tab
// for the restore action to answer. Without an answer the record stays for the
// next page load.
const restoreWait = 30_000;

// probeEvery is how often the offline page checks whether the server answers.
const probeEvery = 5_000;

// ImageBatch is the images of one upload. They go to index in their order, whichever
// the server stores first.
interface ImageBatch {
  index: number;
  inserted: boolean[];
  pending: number;
}

class DocsEditor extends HTMLElement {
  private quill?: Quill;
  private sync?: Sync;
  private cursors?: Cursors;
  private imageToolbar?: ImageToolbar;
  private client = "";
  private stopped = "";
  private resendTimer = 0;
  private cursorTimer = 0;
  private cursorSeq = 0;
  private cursorSent = "";
  private maxImageSize = 0;
  private imagePrefix = "";
  private uploadSeq = 0;
  private uploads = new Map<number, { batch: ImageBatch; n: number }>();
  // batches holds the batches with images on their way. Every change moves their index.
  private batches = new Set<ImageBatch>();
  private started = false;
  private offline = false;
  // url keys the records of the document in the browser storage.
  private url = "";
  private tabs: TabStore | null = null;
  private saveTimer = 0;
  private saveFailed = false;
  // storedTitle is the title of the document on the offline page,
  // which sets no doc-title.
  private storedTitle = "";
  // restoring holds the release of the lock of each closed tab
  // whose changes the restore action has.
  private restoring = new Map<string, () => void>();

  // The offline page sets url through Datastar, which may run after the bundle.
  static observedAttributes = ["url"];

  connectedCallback() {
    if (this.started) {
      return;
    }
    this.started = true;
    this.offline = this.hasAttribute("offline");
    this.maxImageSize = Number(this.getAttribute("max-image-size"));
    this.imagePrefix = this.getAttribute("image-prefix") ?? "";
    if (this.offline) {
      this.openOnce();
      return;
    }
    this.url = this.getAttribute("url") ?? "";
    this.client = this.getAttribute("client") ?? "";
    const script = this.querySelector('script[type="application/json"]');
    const content = new Delta(JSON.parse(script?.textContent ?? "{}").ops);
    this.start(content, Number(this.getAttribute("rev")));
    void this.keep();
  }

  attributeChangedCallback() {
    if (this.offline) {
      this.openOnce();
    }
  }

  // openOnce opens the stored document on the offline page once url is set.
  private openOnce() {
    const url = this.getAttribute("url");
    if (!url || this.url) {
      return;
    }
    this.url = url;
    void this.openStored();
  }

  // start shows content and starts editing.
  // content is the document at revision rev with the changes in pending applied.
  private start(content: Delta, rev: number, pending?: Pending) {
    const container = this.querySelector<HTMLElement>(".ql-container")!;
    // Quill would parse the server markup into a document that setContents
    // replaces anyway.
    container.replaceChildren();
    const quill = new Quill(container, {
      theme: "snow",
      formats,
      modules: {
        toolbar: "#toolbar",
        // Undo reverts only the changes of this editor.
        history: { userOnly: true },
        // The toolbar, dropped files and pasted files all upload through it.
        uploader: {
          mimetypes: imageTypes,
          handler: (range: { index: number }, files: File[]) =>
            this.uploadImages(range.index, files),
        },
        clipboard: { matchers: [["IMG", (node: Node) => this.pastedImage(node)]] },
      },
      placeholder: "Type here",
    });
    quill.setContents(content, "silent");
    quill.history.clear();

    const sync = new Sync(
      rev,
      this.client,
      (s) => this.submit(s),
      (change) => quill.updateContents(change, "api"),
      pending,
    );
    this.quill = quill;
    this.sync = sync;
    this.cursors = new Cursors(quill, sync);
    const bar = this.querySelector<HTMLElement>(".image-toolbar");
    if (bar) {
      this.imageToolbar = new ImageToolbar(quill, bar);
    }
    quill.on("text-change", (change, _, source) => {
      for (const batch of this.batches) {
        batch.index = change.transformPosition(batch.index, true);
      }
      this.imageToolbar?.update();
      if (source !== "user") {
        return;
      }
      sync.local(change);
      this.cursors?.render();
      this.changed();
    });
    quill.on("selection-change", () => {
      this.scheduleCursor();
      this.imageToolbar?.update();
    });
  }

  // keep stores the state of the editor in the browser from now on and sends the
  // restore action what closed tabs of the document left unconfirmed.
  private async keep() {
    const tabs = await TabStore.open();
    if (!tabs) {
      return;
    }
    this.tabs = tabs;
    void acquire(this.client, lockWait);
    this.saveOnHide();
    await this.save();
    for (const r of await tabs.list(this.url)) {
      if (r.client !== this.client) {
        void this.restore(r.client);
      }
    }
  }

  // restore sends the restore action the changes the closed tab of client
  // left unconfirmed. A record without such changes holds content older
  // than the record of this tab, and restore deletes it.
  private async restore(client: string) {
    const release = await acquire(client, lockWait);
    if (!release) {
      return;
    }
    const r = await this.tabs?.get(client);
    if (!r || r.sent === null) {
      await this.tabs?.delete(client);
      release();
      return;
    }
    this.restoring.set(client, release);
    window.setTimeout(() => {
      if (this.restoring.get(client) === release) {
        this.restoring.delete(client);
        release();
      }
    }, restoreWait);
    this.dispatchEvent(
      new CustomEvent("docs-restore", {
        detail: {
          client,
          seq: r.seq,
          rev: r.rev,
          sent: { ops: r.sent },
          buffer: { ops: r.buffer ?? [] },
          title: r.title,
          content: { ops: r.content },
        },
      }),
    );
  }

  // restored deletes the record of the closed tab of client once the restore
  // action put its changes in a document.
  restored(client: string) {
    const release = this.restoring.get(client);
    if (!release) {
      return;
    }
    this.restoring.delete(client);
    void this.tabs
      ?.delete(client)
      .catch(() => {})
      .finally(release);
  }

  // openStored opens the document at url from the browser storage, for the
  // offline page. It continues a closed tab that left changes the server lacks,
  // or starts from the newest content any tab stored.
  private async openStored() {
    const tabs = await TabStore.open();
    const records = tabs ? await tabs.list(this.url) : [];
    let record: TabRecord | undefined;
    let pending: Pending | undefined;
    for (const r of records) {
      if (r.sent === null) {
        continue;
      }
      const release = await acquire(r.client, lockWait);
      // A tab that loaded from the server may have restored it meanwhile.
      const current = release && (await tabs?.get(r.client));
      if (current && current.sent !== null) {
        record = current;
        this.client = current.client;
        pending = {
          seq: current.seq,
          sent: new Delta(current.sent),
          buffer: current.buffer && new Delta(current.buffer),
        };
        break;
      }
      release?.();
    }
    if (!record) {
      record = records.find((r) => r.sent === null);
      this.client = newClient();
    }
    // Without a record to edit, an open tab is editing the newest content.
    const shown = record ?? records[0];
    this.reconnect();
    if (!shown) {
      this.dispatchEvent(
        new CustomEvent("docs-offline", { detail: { shown: "list", title: "" } }),
      );
      return;
    }
    this.storedTitle = shown.title;
    this.dispatchEvent(
      new CustomEvent("docs-offline", { detail: { shown: "doc", title: shown.title } }),
    );
    this.start(new Delta(shown.content), shown.rev, pending);
    if (!record) {
      this.stop("Another tab of this browser is editing this document offline.");
      return;
    }
    this.tabs = tabs;
    if (!pending) {
      void acquire(this.client, lockWait);
    }
    this.saveOnHide();
    await this.save();
  }

  // reconnect dispatches docs-reconnect once the server answers.
  // The page from the server then restores what this tab stored.
  private reconnect() {
    const probe = async () => {
      if (!navigator.onLine) {
        return;
      }
      try {
        // The service worker passes requests other than GET to the network.
        const res = await fetch(this.url, { method: "HEAD", cache: "no-store" });
        if (!res.ok) {
          return;
        }
      } catch {
        return;
      }
      await this.save();
      this.dispatchEvent(new CustomEvent("docs-reconnect"));
    };
    window.addEventListener("online", () => void probe());
    window.setInterval(() => void probe(), probeEvery);
  }

  // receive applies an update from the stream: the revisions after the last
  // one received and the cursors of the others after them.
  receive(update: { changes: Change[]; cursors: RemoteCursor[] }) {
    if (this.stopped || !this.sync) {
      return;
    }
    try {
      this.sync.receive(update.changes);
    } catch (err) {
      this.stop(`This tab lost track of the document: ${(err as Error).message}.`);
      return;
    }
    this.cursors?.set(update.cursors);
    this.changed();
  }

  // stop makes the editor read-only and has the page show message.
  stop(message: string) {
    this.stopped = message;
    this.quill?.enable(false);
    this.imageToolbar?.update();
    window.clearTimeout(this.resendTimer);
    window.clearTimeout(this.cursorTimer);
    this.changed();
  }

  // insertImage inserts the image the server stored for an upload. An empty src means
  // the server refused the file.
  insertImage(u: { upload: number; src: string }) {
    const entry = this.uploads.get(u.upload);
    if (!entry || !this.quill) {
      return;
    }
    this.uploads.delete(u.upload);
    const { batch, n } = entry;
    if (--batch.pending === 0) {
      this.batches.delete(batch);
    }
    if (this.stopped || u.src === "") {
      return;
    }
    const before = batch.inserted.slice(0, n).filter(Boolean).length;
    const index = Math.min(batch.index + before, this.quill.getLength() - 1);
    batch.inserted[n] = true;
    this.quill.updateContents(new Delta().retain(index).insert({ image: u.src }), "user");
  }

  // uploadImages sends files to the server, which calls insertImage for each.
  private uploadImages(index: number, files: File[]) {
    if (this.stopped) {
      return;
    }
    if (this.offline) {
      this.notice("Images can be added when the server is reachable.");
      return;
    }
    const batch: ImageBatch = { index, inserted: [], pending: 0 };
    files.forEach((file, n) => {
      if (file.size > this.maxImageSize) {
        this.notice(`${file.name} is larger than ${this.maxImageSize >> 20} MB.`);
        return;
      }
      const upload = ++this.uploadSeq;
      this.uploads.set(upload, { batch, n });
      batch.pending++;
      const reader = new FileReader();
      reader.onload = () => {
        const url = reader.result as string;
        this.dispatchEvent(
          new CustomEvent("docs-image", {
            detail: { upload, data: url.slice(url.indexOf(",") + 1) },
          }),
        );
      };
      reader.onerror = () => this.insertImage({ upload, src: "" });
      reader.readAsDataURL(file);
    });
    if (batch.pending > 0) {
      this.batches.add(batch);
    }
  }

  // pastedImage keeps a pasted image only if the server stores it, and of its
  // attributes only a width the server accepts.
  private pastedImage(node: Node): Delta {
    const img = node as Element;
    let url: URL;
    try {
      url = new URL(img.getAttribute("src") ?? "", location.href);
    } catch {
      return new Delta();
    }
    if (url.origin !== location.origin || !url.pathname.startsWith(this.imagePrefix)) {
      return new Delta();
    }
    const width = img.getAttribute("width") ?? "";
    return new Delta().insert(
      { image: url.pathname },
      widthPattern.test(width) ? { width } : undefined,
    );
  }

  private notice(message: string) {
    this.dispatchEvent(new CustomEvent("docs-notice", { detail: message }));
  }

  // submit sends a change to the submit action. Offline the change stays pending,
  // for the page from the server to restore.
  private submit(s: Submission) {
    if (this.offline) {
      return;
    }
    this.dispatchEvent(new CustomEvent("docs-submit", { detail: s }));
    window.clearTimeout(this.resendTimer);
    this.resendTimer = window.setTimeout(() => this.sync?.resend(), resendAfter);
  }

  // changed tells the page the revision for the stream to reconnect with,
  // whether a change waits for the server, and why the editor stopped.
  private changed() {
    const sync = this.sync;
    if (!sync) {
      return;
    }
    if (!sync.saving) {
      window.clearTimeout(this.resendTimer);
    }
    this.dispatchEvent(
      new CustomEvent("docs-state", {
        detail: { rev: sync.rev, saving: sync.saving, stopped: this.stopped },
      }),
    );
    this.scheduleCursor();
    this.scheduleSave();
  }

  private scheduleSave() {
    if (this.tabs && !this.saveTimer) {
      this.saveTimer = window.setTimeout(() => void this.save(), saveDelay);
    }
  }

  // save stores the state of the editor in the browser.
  private async save() {
    window.clearTimeout(this.saveTimer);
    this.saveTimer = 0;
    const { tabs, sync, quill } = this;
    if (!tabs || !sync || !quill) {
      return;
    }
    const { seq, sent, buffer } = sync.pending;
    try {
      await tabs.put({
        client: this.client,
        url: this.url,
        title: this.getAttribute("doc-title") ?? this.storedTitle,
        rev: sync.rev,
        seq,
        sent: sent?.ops ?? null,
        buffer: buffer?.ops ?? null,
        content: quill.getContents().ops,
        saved: Date.now(),
      });
    } catch {
      // The browser refuses past its storage quota.
      if (!this.saveFailed) {
        this.saveFailed = true;
        this.notice("This browser could not store the document. A reload loses what the server lacks.");
      }
    }
  }

  // saveOnHide stores the state when the page is hidden or closes.
  // The browser may close the page before the write completes.
  private saveOnHide() {
    document.addEventListener("visibilitychange", () => {
      if (document.hidden) {
        void this.save();
      }
    });
    window.addEventListener("pagehide", () => void this.save());
  }

  private scheduleCursor() {
    window.clearTimeout(this.cursorTimer);
    this.cursorTimer = window.setTimeout(() => this.sendCursor(), cursorDelay);
  }

  // sendCursor reports the selection only while no change waits for the server,
  // when editor positions match the last revision.
  private sendCursor() {
    const sync = this.sync;
    if (this.stopped || this.offline || !sync || sync.saving) {
      return;
    }
    const range = this.quill?.getSelection();
    if (!range) {
      return;
    }
    const key = `${range.index}:${range.length}`;
    if (key === this.cursorSent) {
      return;
    }
    this.cursorSent = key;
    this.dispatchEvent(
      new CustomEvent("docs-cursor", {
        detail: {
          client: this.client,
          seq: ++this.cursorSeq,
          rev: sync.rev,
          index: range.index,
          length: range.length,
        },
      }),
    );
  }
}

customElements.define("docs-editor", DocsEditor);

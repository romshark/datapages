// Every tab of a document stores the state of its editor in IndexedDB after each change.
// What the server has not confirmed survives a reload, a closed tab or a crash,
// and the offline page opens the document from it. A tab holds the Web Lock of
// its record while open: a record whose lock is free belongs to a closed tab.
import type Delta from "quill-delta";

type Ops = Delta["ops"];

// TabRecord is the state of the editor of one tab.
export interface TabRecord {
  client: string;
  // url is the path of the document page.
  url: string;
  title: string;
  // rev is the last revision received. seq, sent and buffer are a Pending.
  rev: number;
  seq: number;
  sent: Ops | null;
  buffer: Ops | null;
  // content is what the editor showed: the document at rev with sent and buffer.
  content: Ops;
  // saved is when the tab wrote the record, in milliseconds since the epoch.
  saved: number;
}

const dbName = "docs";
const tabs = "tabs";

// lockWait is how long a page waits for the lock of a record. A page that is
// reloading may still hold its lock when the new page asks for it.
export const lockWait = 1000;

// TabStore holds the records of all tabs of this origin.
export class TabStore {
  private readonly db: IDBDatabase;

  private constructor(db: IDBDatabase) {
    this.db = db;
  }

  // open returns null where the browser keeps no records. Web Locks,
  // like the service worker, exist only in a secure context: HTTPS or localhost.
  static async open(): Promise<TabStore | null> {
    if (!window.isSecureContext || !navigator.locks || !window.indexedDB) {
      return null;
    }
    const req = indexedDB.open(dbName, 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore(tabs, { keyPath: "client" }).createIndex("url", "url");
    };
    try {
      return new TabStore(await result(req));
    } catch {
      // Private windows of some browsers refuse IndexedDB.
      return null;
    }
  }

  // list returns the records of the document at url,
  // or of all documents, the newest first.
  async list(url?: string): Promise<TabRecord[]> {
    const store = this.db.transaction(tabs).objectStore(tabs);
    const req = url === undefined ? store.getAll() : store.index("url").getAll(url);
    const records: TabRecord[] = await result(req);
    return records.sort((a, b) => b.saved - a.saved);
  }

  get(client: string): Promise<TabRecord | undefined> {
    return result(this.db.transaction(tabs).objectStore(tabs).get(client));
  }

  put(record: TabRecord): Promise<void> {
    return this.write((store) => store.put(record));
  }

  delete(client: string): Promise<void> {
    return this.write((store) => store.delete(client));
  }

  // write resolves once the browser committed the transaction.
  private write(f: (store: IDBObjectStore) => void): Promise<void> {
    const tx = this.db.transaction(tabs, "readwrite");
    f(tx.objectStore(tabs));
    return new Promise((resolve, reject) => {
      tx.oncomplete = () => resolve();
      tx.onerror = tx.onabort = () => reject(tx.error);
    });
  }
}

function result<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}

// acquire takes the lock of the record of client, waiting at most wait milliseconds.
// It resolves to the function that releases the lock, or to null if an open tab holds it.
// A lock never released is released when the page closes.
export function acquire(client: string, wait: number): Promise<(() => void) | null> {
  return new Promise((resolve) => {
    navigator.locks
      .request(
        `docs-tab-${client}`,
        { signal: AbortSignal.timeout(wait) },
        () => new Promise<void>((release) => resolve(release)),
      )
      .catch(() => resolve(null));
  });
}

// newClient returns a client id of the form store.NewClient returns:
// 16 random bytes in unpadded base64url.
export function newClient(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

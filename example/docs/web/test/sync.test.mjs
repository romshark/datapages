// Tests that editors end with the server document however requests and
// streams interleave. Run with: npm test
import assert from "node:assert/strict";
import { test } from "node:test";
import Delta from "quill-delta";
import { Sync } from "../src/sync.ts";

// mulberry32 is a small seeded PRNG: https://stackoverflow.com/a/47593316
function mulberry32(seed) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

// Server transforms and applies submissions as store/submit.go does.
class Server {
  doc = new Delta().insert("\n");
  rev = 0;
  history = [];

  submit(s) {
    const missed = this.history.filter((c) => c.rev > s.rev);
    if (missed.some((c) => c.client === s.client && c.seq === s.seq)) {
      return;
    }
    let change = new Delta(s.delta.ops);
    for (const c of missed) {
      change = c.delta.transform(change, true);
    }
    this.doc = this.doc.compose(change);
    this.history.push({ rev: ++this.rev, delta: change, client: s.client, seq: s.seq });
  }
}

// Client is a tab: its editor, its pending requests and its stream.
class Client {
  constructor(id, server) {
    this.id = id;
    this.server = server;
    this.doc = new Delta().insert("\n");
    this.requests = [];
    // streamed is the last revision the stream sent, as StateDoc.Rev.
    this.streamed = 0;
    this.sync = new Sync(
      0,
      id,
      (s) => this.requests.push(JSON.parse(JSON.stringify(s))),
      (change) => {
        this.doc = this.doc.compose(change);
      },
    );
  }

  edit(random) {
    const length = this.doc.length() - 1;
    const at = Math.floor(random() * (length + 1));
    const n = Math.min(length - at, 1 + Math.floor(random() * 3));
    const change = new Delta().retain(at);
    const r = random();
    if (r < 0.5 || n === 0) {
      change.insert(["a", "bc", "\n", "dé"][Math.floor(random() * 4)]);
    } else if (r < 0.8) {
      change.delete(n);
    } else {
      change.retain(n, random() < 0.5 ? { bold: true } : { bold: null });
    }
    this.doc = this.doc.compose(change);
    this.sync.local(change);
  }

  // stream sends what the server applied since the last send, like
  // OnDocChanged.
  stream() {
    const changes = this.server.history
      .filter((c) => c.rev > this.streamed)
      .map((c) => ({
        rev: c.rev,
        delta: JSON.parse(JSON.stringify(c.delta)),
        ...(c.client === this.id ? { ack: c.seq } : {}),
      }));
    this.streamed = this.server.rev;
    this.sync.receive(changes);
  }

  // reconnect replays changes like StreamOpen, some of which the editor
  // already has.
  reconnect() {
    this.streamed = Math.max(0, this.sync.rev - 2);
    this.stream();
  }
}

for (const seed of [1, 2, 3, 4, 5, 6, 7, 8]) {
  test(`editors converge, seed ${seed}`, () => {
    const random = mulberry32(seed);
    const server = new Server();
    const clients = ["a", "b", "c"].map((id) => new Client(id, server));
    const pick = () => clients[Math.floor(random() * clients.length)];

    for (let step = 0; step < 2000; step++) {
      const c = pick();
      const r = random();
      if (r < 0.4) {
        c.edit(random);
      } else if (r < 0.65) {
        const s = c.requests.shift();
        if (s) {
          server.submit(s);
        }
      } else if (r < 0.9) {
        c.stream();
      } else if (r < 0.95) {
        c.sync.resend();
      } else {
        c.reconnect();
      }
    }

    for (
      let round = 0; clients.some((c) => c.requests.length > 0 || c.sync.saving); round++
    ) {
      assert.ok(round < 100, "the clients never caught up");
      for (const c of clients) {
        while (c.requests.length > 0) {
          server.submit(c.requests.shift());
        }
      }
      for (const c of clients) {
        c.stream();
      }
    }
    for (const c of clients) {
      assert.deepEqual(c.doc.ops, server.doc.ops, `document of ${c.id}`);
      assert.equal(c.sync.rev, server.rev);
    }
  });
}

test("a missing revision stops the editor", () => {
  const sync = new Sync(0, "a", () => {}, () => {});
  assert.throws(() => sync.receive(
    [{ rev: 2, delta: { ops: [] } }]), /revision 1 is missing/,
  );
});

test("an acknowledgment of a change not sent stops the editor", () => {
  const sync = new Sync(0, "a", () => {}, () => {});
  assert.throws(() => sync.receive([{ rev: 1, delta: { ops: [] }, ack: 1 }]), /change 1/);
});

test("an editor continues the changes a closed editor left pending", () => {
  const server = new Server();
  const closed = new Sync(0, "a", (s) => server.submit(s), () => {});
  closed.local(new Delta().insert("Hello"));
  closed.local(new Delta().retain(5).insert(" World"));

  // The offline page stores the operations and makes Deltas of them again.
  const stored = JSON.parse(JSON.stringify(closed.pending));
  const requests = [];
  const editor = new Sync(closed.rev, "a", (s) => requests.push(s), () => {}, {
    seq: stored.seq,
    sent: new Delta(stored.sent.ops),
    buffer: new Delta(stored.buffer.ops),
  });
  const [applied] = server.history;
  editor.receive([{ rev: applied.rev, delta: applied.delta, ack: applied.seq }]);
  assert.deepEqual(
    requests.map((s) => ({ seq: s.seq, rev: s.rev, ops: s.delta.ops })),
    [{ seq: 2, rev: 1, ops: [{ retain: 5 }, { insert: " World" }] }],
  );
});

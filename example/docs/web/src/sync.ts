// The client side of operational transformation with a central server, as in
// ot.js (https://github.com/Operational-Transformation/ot.js): 1 pending
// change at a time, and a buffer for what the user changes meanwhile.
// The server applies changes in 1 order and sends each to every editor,
// its author included. That copy acknowledges the change.
import Delta from "quill-delta";

// Change is a revision the server applied.
export interface Change {
  rev: number;
  delta: { ops: Delta["ops"] };
  // ack is the sequence number this editor sent the change with,
  // absent for changes of others.
  ack?: number;
}

// Submission is a change sent to the server.
export interface Submission {
  client: string;
  seq: number;
  rev: number;
  delta: Delta;
}

// Pending is what the server has not confirmed: sent, submitted with sequence
// number seq, and buffer, made after it. seq is the last number used.
export interface Pending {
  seq: number;
  sent: Delta | null;
  buffer: Delta | null;
}

export class Sync {
  // rev is the last revision received.
  rev: number;
  private readonly client: string;
  private readonly send: (s: Submission) => void;
  private readonly apply: (change: Delta) => void;
  private seq = 0;
  // sent is the pending change, transformed against every change received since,
  // as on the server.
  private sent: Delta | null = null;
  // buffer holds what the user changed after sent.
  private buffer: Delta | null = null;

  // pending continues an editor that stored its state and closed.
  constructor(
    rev: number,
    client: string,
    send: (s: Submission) => void,
    apply: (change: Delta) => void,
    pending?: Pending,
  ) {
    this.rev = rev;
    this.client = client;
    this.send = send;
    this.apply = apply;
    if (pending) {
      this.seq = pending.seq;
      this.sent = pending.sent;
      this.buffer = pending.buffer;
    }
  }

  // saving reports whether the server lacks a change of this editor.
  get saving(): boolean {
    return this.sent !== null;
  }

  get pending(): Pending {
    return { seq: this.seq, sent: this.sent, buffer: this.buffer };
  }

  // local takes a change the user made.
  local(change: Delta): void {
    if (this.sent !== null) {
      this.buffer = this.buffer === null ? change : this.buffer.compose(change);
      return;
    }
    this.sent = change;
    this.seq++;
    this.submit();
  }

  // receive applies revisions in order. It throws on a missing revision or an
  // acknowledgment of a change not sent, which means the document no longer
  // matches the server.
  receive(changes: Change[]): void {
    for (const c of changes) {
      if (c.rev <= this.rev) {
        continue;
      }
      if (c.rev !== this.rev + 1) {
        throw new Error(`revision ${this.rev + 1} is missing`);
      }
      this.rev = c.rev;
      if (c.ack !== undefined) {
        if (this.sent === null || c.ack !== this.seq) {
          throw new Error(`change ${c.ack} was not sent`);
        }
        this.sent = this.buffer;
        this.buffer = null;
        if (this.sent !== null) {
          this.seq++;
          this.submit();
        }
        continue;
      }
      // The server applied change before sent and transformed sent against it,
      // change first. Do the same here.
      let change = new Delta(c.delta.ops);
      if (this.sent !== null) {
        [this.sent, change] = [
          change.transform(this.sent, true),
          this.sent.transform(change, false),
        ];
        if (this.buffer !== null) {
          [this.buffer, change] = [
            change.transform(this.buffer, true),
            this.buffer.transform(change, false),
          ];
        }
      }
      this.apply(change);
    }
  }

  // resend sends the pending change again in case the request failed.
  // The server applies it once, however often it arrives.
  resend(): void {
    if (this.sent !== null) {
      this.submit();
    }
  }

  // toLocal maps a position in the last revision received to the editor,
  // which also shows the changes the server lacks. It stays before an insert
  // of this editor at the position, as on the server once the insert arrives.
  toLocal(index: number): number {
    if (this.sent !== null) {
      index = this.sent.transformPosition(index, true);
    }
    if (this.buffer !== null) {
      index = this.buffer.transformPosition(index, true);
    }
    return index;
  }

  private submit(): void {
    this.send({
      client: this.client,
      seq: this.seq,
      rev: this.rev,
      delta: this.sent!,
    });
  }
}

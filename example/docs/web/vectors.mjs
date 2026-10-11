// Writes the quill-delta results that delta/vectors_test.go checks the Go
// port against. Run it after upgrading quill-delta:
//
//	node vectors.mjs > ../delta/testdata/quill-delta.json
//
// Cases are random with a fixed seed, which keeps the output stable. Changes
// cut documents only between code points: the Go port rejects cuts inside a
// surrogate pair, which quill-delta allows.
import Delta from "quill-delta";

const cases = 100;

// mulberry32 is a small seeded PRNG: https://stackoverflow.com/a/47593316
function mulberry32(seed) {
  return () => {
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const random = mulberry32(1);
const int = (n) => Math.floor(random() * n);
const pick = (xs) => xs[int(xs.length)];

const chars = ["a", "b", "c", " ", "\n", "é", "😀"];
const formats = [
  { bold: true },
  { italic: true },
  { color: "red" },
  { color: "blue" },
  { header: 1 },
  { bold: true, color: "red" },
];
const formatChanges = [
  ...formats,
  { bold: null },
  { color: null },
  { header: 2, bold: null },
];

function text(n) {
  let s = "";
  for (let i = 0; i < n; i++) s += pick(chars);
  return s;
}

const maybe = (p, v) => (random() < p ? v : undefined);

// insertRandom inserts up to maxLen random characters, or 1 time in 5
// an image, formatted with probability p.
function insertRandom(d, maxLen, p) {
  const format = maybe(p, pick(formats));
  if (random() < 0.2) {
    return d.insert({ image: "a.png" }, format);
  }
  return d.insert(text(1 + int(maxLen)), format);
}

function randomDocument() {
  const d = new Delta();
  for (let i = 1 + int(5); i > 0; i--) {
    insertRandom(d, 6, 0.5);
  }
  return d.insert("\n");
}

// cuts returns the positions between the code points and images of doc.
function cuts(doc) {
  const out = [0];
  let pos = 0;
  for (const op of doc.ops) {
    if (typeof op.insert !== "string") {
      out.push(++pos);
      continue;
    }
    for (const ch of op.insert) {
      pos += ch.length;
      out.push(pos);
    }
  }
  return out;
}

function randomChange(doc) {
  const at = cuts(doc);
  const d = new Delta();
  let i = 0;
  for (let step = 0; step < 8 && i < at.length - 1; step++) {
    const r = random();
    if (r < 0.3) {
      insertRandom(d, 3, 0.3);
      continue;
    }
    const j = Math.min(at.length - 1, i + 1 + int(4));
    if (r < 0.65) {
      d.retain(at[j] - at[i], maybe(0.5, pick(formatChanges)));
    } else {
      d.delete(at[j] - at[i]);
    }
    i = j;
  }
  return d.chop();
}

const out = [];
for (let n = 0; n < cases; n++) {
  const doc = randomDocument();
  const a = randomChange(doc);
  const b = randomChange(doc);
  const transformed = a.transform(b, true);
  const positions = [];
  for (let p = 0; p <= doc.length(); p++) {
    positions.push([a.transformPosition(p, true), a.transformPosition(p, false)]);
  }
  out.push({
    doc,
    a,
    b,
    applied: doc.compose(a),
    transformed,
    transformedWithoutPriority: a.transform(b, false),
    composed: a.compose(transformed),
    positions,
  });
}
process.stdout.write(JSON.stringify(out) + "\n");

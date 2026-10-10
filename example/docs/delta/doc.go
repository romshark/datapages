// Package delta implements the Quill [Delta] format and its operational transformation.
//
// [Delta.Compose], [Delta.Transform] and [Delta.TransformPosition] port [quill-delta]
// 5.1.0, which the editor runs in the browser.
// Results must match exactly: server and editor transform the same changes,
// and documents converge only if both agree.
// testdata/quill-delta.json holds reference results from quill-delta.
//
// Lengths and positions are in UTF-16 code units, like JavaScript strings.
// An embed counts 1.
//
// [quill-delta]: https://github.com/slab/delta
package delta

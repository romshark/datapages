// Package store keeps the documents: content, title, change history,
// and the editors with each document open and their cursors.
//
// A document is two files in the store directory: <id>.json, a snapshot,
// and <id>.log, one JSON line per change since. Appending a line per change is
// cheaper than writing the document. A restart replays the log onto the snapshot.
package store

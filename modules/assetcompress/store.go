package assetcompress

import "io"

// Store keeps compressed static assets. Keys contain the source file's SHA-256, coding
// and level. They use lowercase letters, digits and dots and cannot start with a dot.
//
// Implementations must be safe for concurrent use.
type Store interface {
	// Open returns the copy under key or an error wrapping [io/fs.ErrNotExist].
	Open(key string) (io.ReadSeekCloser, error)

	// Put stores r under key atomically. Repeated puts of 1 key contain the
	// same bytes and may keep the first copy.
	Put(key string, r io.Reader) error
}

// BytesStore is a [Store] that keeps copies in memory.
// The compressor reads its copies with Bytes instead of Open.
type BytesStore interface {
	Store

	// Bytes returns the copy under key or an error wrapping [io/fs.ErrNotExist].
	// Callers must not modify the bytes.
	Bytes(key string) ([]byte, error)
}

package assetcompress

import "io"

// Store keeps compressed static assets. Keys contain the source file's SHA-256, coding
// and level. They use lowercase letters, digits and dots and cannot start with a dot.
//
// Implementations must be safe for concurrent use. The compressor never deletes copies.
type Store interface {
	// Open returns the copy under key or an error wrapping [io/fs.ErrNotExist].
	// The compressor calls it on every request for a stored copy. On an error,
	// the request gets the uncompressed file.
	Open(key string) (io.ReadSeekCloser, error)

	// Put stores r under key atomically. Repeated puts of 1 key contain the
	// same bytes and may keep the first copy.
	Put(key string, r io.Reader) error
}

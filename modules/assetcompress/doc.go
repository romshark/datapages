// Package assetcompress compresses static assets and keeps the copies in memory
// or a [Store].
//
// [New] returns a [Compressor] for [github.com/romshark/datapages.WithAssetsCompression].
// [Compressor.Precompress] compresses the embedded files before the first request.
//
// Without a store, the compressor keeps the copies in its memory and
// recompresses files after every restart.
// The [github.com/romshark/datapages/modules/assetcompress/disk] store
// persists them and, at startup, removes copies unused for 7 days by default.
package assetcompress

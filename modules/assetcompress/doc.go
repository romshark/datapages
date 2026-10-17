// Package assetcompress compresses static assets and keeps the copies in a [Store].
//
// [New] returns a [Compressor] for [github.com/romshark/datapages.WithAssetsCompression].
// Without that option, the server uses the defaults of [Config].
// [Compressor.Precompress] compresses the embedded files before the first request.
//
// Stores:
//   - [github.com/romshark/datapages/modules/assetcompress/inmem] is the default.
//     Each restart compresses the files again, which takes 4 to 10 ms
//     per 220 KB file at the default levels.
//   - [github.com/romshark/datapages/modules/assetcompress/disk] keeps the
//     copies across restarts. Use it with brotli quality 11, which takes 143 to
//     213 ms per 220 KB file. It never deletes old copies.
package assetcompress

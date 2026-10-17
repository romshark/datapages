// Package assetcompress compresses static assets and keeps the copies in a [Store].
//
// [New] returns a [Compressor] for [github.com/romshark/datapages.WithAssetsCompression].
// Without that option, the server uses the defaults of [Config].
// [Compressor.Precompress] compresses the embedded files before the first request.
//
// Stores:
//   - [github.com/romshark/datapages/modules/assetcompress/inmem] is the default.
//     It keeps the copies of the files served now and compresses them again
//     after each restart, which takes 4 to 10 ms per 220 KB file at the default levels.
//   - [github.com/romshark/datapages/modules/assetcompress/disk] keeps the
//     copies across restarts, those of earlier builds included.
//     Use it with brotli quality 11, which takes 143 to 213 ms per 220 KB file.
package assetcompress

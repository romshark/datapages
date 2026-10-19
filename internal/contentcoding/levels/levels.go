// Package levels checks the levels of the content codings. It imports no
// encoder, which keeps the root package free of them.
package levels

import "fmt"

// Zstd reports a zstd level out of range, with prefix before the message.
// 0 selects a default and passes.
func Zstd(prefix string, level int) error {
	if level < 0 || level > 22 {
		return outOfRange(prefix, "zstd level", level, 22)
	}
	return nil
}

// Brotli reports a brotli quality out of range, as [Zstd] does.
func Brotli(prefix string, quality int) error {
	if quality < 0 || quality > 11 {
		return outOfRange(prefix, "brotli quality", quality, 11)
	}
	return nil
}

// Gzip reports a gzip level out of range, as [Zstd] does.
func Gzip(prefix string, level int) error {
	if level < 0 || level > 9 {
		return outOfRange(prefix, "gzip level", level, 9)
	}
	return nil
}

// All reports the first level that [Zstd], [Brotli] or [Gzip] refuses.
func All(prefix string, zstd, brotli, gzip int) error {
	if err := Zstd(prefix, zstd); err != nil {
		return err
	}
	if err := Brotli(prefix, brotli); err != nil {
		return err
	}
	return Gzip(prefix, gzip)
}

func outOfRange(prefix, what string, level, highest int) error {
	return fmt.Errorf("%s%s (%d) must be from 1 to %d", prefix, what, level, highest)
}

// Package levels checks the levels of the content codings.
package levels

import "fmt"

// Zstd reports a zstd level out of range. 0 selects a default and passes.
func Zstd(level int) error {
	if level < 0 || level > 22 {
		return outOfRange("zstd level", level, 22)
	}
	return nil
}

// Brotli reports a brotli quality out of range, as [Zstd] does.
func Brotli(quality int) error {
	if quality < 0 || quality > 11 {
		return outOfRange("brotli quality", quality, 11)
	}
	return nil
}

// Gzip reports a gzip level out of range, as [Zstd] does.
func Gzip(level int) error {
	if level < 0 || level > 9 {
		return outOfRange("gzip level", level, 9)
	}
	return nil
}

// All reports the first level that [Zstd], [Brotli] or [Gzip] refuses.
func All(zstd, brotli, gzip int) error {
	if err := Zstd(zstd); err != nil {
		return err
	}
	if err := Brotli(brotli); err != nil {
		return err
	}
	return Gzip(gzip)
}

func outOfRange(what string, level, highest int) error {
	return fmt.Errorf("%s (%d) must be from 1 to %d", what, level, highest)
}

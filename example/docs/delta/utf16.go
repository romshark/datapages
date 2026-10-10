package delta

import "unicode/utf8"

// utf16Len returns the number of UTF-16 code units in s: 2 per rune above
// U+FFFF, 1 per other rune or invalid byte.
func utf16Len(s string) int {
	n := 0
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			n++
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		n += utf16Units(r)
		i += size
	}
	return n
}

// UTF16Len returns the number of UTF-16 code units in s, its JavaScript length.
func UTF16Len(s string) int { return utf16Len(s) }

func utf16Units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// advance returns the byte index n code units after byte i of s.
// Inside a surrogate pair, it returns the index after the pair and false.
func advance(s string, i, n int) (int, bool) {
	for n > 0 && i < len(s) {
		if s[i] < utf8.RuneSelf {
			n--
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		units := utf16Units(r)
		i += size
		if units > n {
			return i, false
		}
		n -= units
	}
	return i, true
}

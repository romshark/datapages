package subject

import "strings"

const (
	// Separator stands between the tokens of a subject.
	Separator = "."
	// Wildcards match one token ("*") and everything below (">").
	Wildcards = "*>"
	// Reserved are the characters a subject token must not contain.
	Reserved = Separator + Wildcards + " \t\r\n"
)

// Escape introduces the two hex digits an escaped byte is written as.
const Escape = '%'

const hexDigits = "0123456789ABCDEF"

// IsToken reports whether v may be filled into a subject as it is,
// which is what [Encode] leaves untouched.
func IsToken(v string) bool {
	return v != "" && !strings.ContainsAny(v, Reserved)
}

// lutEscapedLen is a lookup table of what a byte adds to the encoded length.
var lutEscapedLen = func() (t [256]uint8) {
	for i := range t {
		if b := byte(i); b == Escape || strings.IndexByte(Reserved, b) >= 0 {
			t[i] = 2
		}
	}
	return t
}()

// mustEscape reports whether b cannot stand in a subject token as itself.
func mustEscape(b byte) bool { return lutEscapedLen[b] != 0 }

// EncodedLen returns the length [Encode] gives v, without building it.
func EncodedLen(v string) int {
	n, i := len(v), 0
	for ; i+8 <= len(v); i += 8 {
		s := v[i : i+8]
		n += int(lutEscapedLen[s[0]]) + int(lutEscapedLen[s[1]]) +
			int(lutEscapedLen[s[2]]) + int(lutEscapedLen[s[3]]) +
			int(lutEscapedLen[s[4]]) + int(lutEscapedLen[s[5]]) +
			int(lutEscapedLen[s[6]]) + int(lutEscapedLen[s[7]])
	}
	for ; i < len(v); i++ {
		n += int(lutEscapedLen[v[i]])
	}
	return n
}

// Encode writes subject as one subject token, escaping any byte that would end the
// token or match more than itself. Only ASCII bytes are escaped, which leaves
// multi-byte UTF-8 whole, and a value that needs none comes back unchanged.
//
// Escaping instead of refusing lets a user ID for example to be an email address.
func Encode(subject string) string {
	n := EncodedLen(subject)
	if n == len(subject) {
		return subject
	}
	var b strings.Builder
	b.Grow(n)
	for i := range len(subject) {
		c := subject[i]
		if !mustEscape(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte(Escape)

		// c as two hex digits
		b.WriteByte(hexDigits[c>>4])  // first the top four bits.
		b.WriteByte(hexDigits[c&0xF]) // now the bottom four.
	}
	return b.String()
}

// Prefix returns the subject prefix an event with subject fields publishes under.
//
//	"messaging.sent"
//
// becomes
//
//	"messaging.sent."
func Prefix(s string) string { return s + Separator }

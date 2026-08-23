package csrf

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"hash"
	"io"
	"sync"
)

var (
	_ TokenWriter    = Tokens{}
	_ TokenValidator = Tokens{}
)

// derivationKey is a domain separator, not a secret. The session token is the
// message and is what the derived value rests on.
//
// Keying HMAC by a fixed value rather than by the session token is what lets
// one keyed state serve every session, hence the pool. HMAC extends to no
// other message, which a bare hash over the same two inputs would.
var derivationKey = []byte("github.com/romshark/datapages csrf token v1")

// baseLen is the length of the derived value and of the mask hiding it.
const baseLen = sha256.Size

// tokenLen is the length of the base64 the client receives,
// which carries the mask and the masked value. It is what
// [base64.Encoding.EncodedLen] computes for unpadded base64.
const tokenLen = (2*baseLen*8 + 5) / 6

// derivation is the pooled state one token derivation runs on.
type derivation struct {
	hmac hash.Hash
	// buf receives the string parameters. Converting them at the call site
	// allocates, since the result reaches [hash.Hash.Write] as an interface.
	buf []byte
	sum []byte
	// out is what a token is encoded from, enc is what it is encoded into.
	// Local arrays would escape, through [rand.Read] and through
	// [io.Writer.Write] respectively.
	out []byte
	enc []byte
}

var pool = sync.Pool{New: func() any {
	return &derivation{
		hmac: hmac.New(sha256.New, derivationKey),
		buf:  make([]byte, 0, 128),
		sum:  make([]byte, 0, baseLen),
		out:  make([]byte, 2*baseLen),
		enc:  make([]byte, tokenLen),
	}
}}

// Tokens is the built-in [TokenWriter] and [TokenValidator].
//
// It derives the token from the session token with HMAC-SHA256, which is
// one-way: the client holds the result, never the credential behind it.
// Every generated token carries a fresh random mask, which keeps the value on
// the wire different on every render and defeats BREACH.
//
// The zero value is ready to use and safe for concurrent use.
type Tokens struct{}

// WriteToken writes the value sent to the browser: a fresh random mask
// followed by the derived value XORed with it, which is the masking gorilla/csrf uses.
func (Tokens) WriteToken(w io.Writer, sessionToken string) (n int, err error) {
	if sessionToken == "" {
		return 0, nil // A guest has no session token to derive from.
	}
	d := pool.Get().(*derivation)
	defer pool.Put(d)
	base := d.derive(sessionToken)

	mask := d.out[:baseLen]
	if _, err := rand.Read(mask); err != nil {
		panic(err) // [rand.Read] should never fail on a healthy system.
	}
	for i := range baseLen {
		d.out[baseLen+i] = base[i] ^ mask[i]
	}
	base64.RawURLEncoding.Encode(d.enc, d.out)
	return w.Write(d.enc)
}

// ValidateToken reports whether token carries the value derived from
// sessionToken. The comparison is constant time.
func (Tokens) ValidateToken(sessionToken, token string) bool {
	if sessionToken == "" || len(token) != tokenLen {
		return false
	}
	d := pool.Get().(*derivation)
	defer pool.Put(d)

	// Decoding reads the scratch buffer derive overwrites, hence it runs first.
	var decoded [2 * baseLen]byte
	d.buf = append(d.buf[:0], token...)
	if n, err := base64.RawURLEncoding.Decode(decoded[:], d.buf); err != nil ||
		n != len(decoded) {
		return false
	}
	base := d.derive(sessionToken)

	mask, masked := decoded[:baseLen], decoded[baseLen:]
	var got [baseLen]byte
	for i := range baseLen {
		got[i] = masked[i] ^ mask[i]
	}
	return subtle.ConstantTimeCompare(got[:], base[:]) == 1
}

// derive reduces the session token to the value the CSRF token masks.
// It overwrites d.buf.
func (d *derivation) derive(sessionToken string) (base [baseLen]byte) {
	d.hmac.Reset()
	d.buf = append(d.buf[:0], sessionToken...)
	_, _ = d.hmac.Write(d.buf)
	d.sum = d.hmac.Sum(d.sum[:0])
	copy(base[:], d.sum)
	return base
}

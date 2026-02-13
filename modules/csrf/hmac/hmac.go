// Package hmac provides an HMAC-SHA256 based CSRF token manager
// with BREACH-resistant random masking and sync.Pool-based allocation reuse.
package hmac

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"hash"
	"strconv"
	"sync"

	"github.com/romshark/datapages/modules/csrf"
)

var _ csrf.TokenManager = (*TokenManager)(nil)

// TokenManager implements csrf.TokenManager using HMAC-SHA256
// with BREACH-resistant random masking.
type TokenManager struct {
	pool sync.Pool
}

type generationContext struct {
	hmac         hash.Hash
	decodedToken []byte
	issuedAtHex  []byte
	base         []byte
	expectedBase []byte
	mask         []byte
	out          []byte
}

// New creates a new HMAC-SHA256 based TokenManager.
// secret is used as the HMAC key and must not be empty.
func New(secret []byte) (*TokenManager, error) {
	if len(secret) < 1 {
		return nil, errors.New("empty CSRF secret")
	}
	s := make([]byte, len(secret))
	copy(s, secret)

	tm := &TokenManager{}
	tm.pool.New = func() any {
		return &generationContext{
			hmac:         hmac.New(sha256.New, s),
			decodedToken: make([]byte, 64),
			issuedAtHex:  make([]byte, 16),
			base:         make([]byte, 32),
			expectedBase: make([]byte, 32),
			mask:         make([]byte, 32),
			out:          make([]byte, 64),
		}
	}
	return tm, nil
}

// withCtx derives a stable, per-session base secret value and invokes fn with pooled buffers.
func (tm *TokenManager) withCtx(
	userID string, sessIssuedAtUnix int64,
	fn func(*generationContext),
) {
	gc := tm.pool.Get().(*generationContext)
	defer tm.pool.Put(gc)

	gc.hmac.Reset()
	// Ensures the CSRF token is only valid for this user.
	_, _ = gc.hmac.Write([]byte(userID))
	// Separator avoids ambiguity ("ab"+"12" vs "a"+"b12").
	_, _ = gc.hmac.Write([]byte{0})
	// Bind token to a specific session preventing reuse across re-authentication.
	gc.issuedAtHex = strconv.AppendInt(gc.issuedAtHex[:0], sessIssuedAtUnix, 16)
	_, _ = gc.hmac.Write(gc.issuedAtHex)

	gc.base = gc.hmac.Sum(gc.base[:0])
	fn(gc)
}

// GenerateToken returns the value sent to the browser.
// This uses the same masking technique as gorilla/csrf to prevent BREACH attacks:
//   - A random mask is generated per response.
//   - The real token is XORed with the mask.
//   - The mask is prepended so the server can reverse it.
func (tm *TokenManager) GenerateToken(
	userID string, sessIssuedAtUnix int64,
) (t string) {
	if sessIssuedAtUnix < 0 {
		return ""
	}
	tm.withCtx(userID, sessIssuedAtUnix, func(gc *generationContext) {
		if _, err := rand.Read(gc.mask); err != nil {
			panic(err) // rand.Read should never fail on a healthy system.
		}

		// [ mask | masked_token ]
		copy(gc.out, gc.mask)

		// XOR hides the real token while remaining reversible.
		for i := range 32 {
			gc.out[32+i] = gc.base[i] ^ gc.mask[i]
		}
		t = base64.RawURLEncoding.EncodeToString(gc.out)
	})
	return t
}

// ValidateToken verifies a client-supplied token.
func (tm *TokenManager) ValidateToken(
	userID string, sessIssuedAtUnix int64, token string,
) (ok bool) {
	if len(token) != 86 || sessIssuedAtUnix < 0 {
		return false
	}
	tm.withCtx(userID, sessIssuedAtUnix, func(gc *generationContext) {
		n, err := base64.RawURLEncoding.Decode(gc.decodedToken, []byte(token))
		if err != nil || n != 64 {
			ok = false
			return
		}

		// [ mask | masked_token ]
		mask, enc := gc.decodedToken[:32], gc.decodedToken[32:]
		// Reverse XOR to recover the real token.
		for i := range 32 {
			gc.expectedBase[i] = enc[i] ^ mask[i]
		}
		// Recompute what the token SHOULD be for this session and compare.
		ok = subtle.ConstantTimeCompare(gc.expectedBase, gc.base) == 1
	})
	return ok
}

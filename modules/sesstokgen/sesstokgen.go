// Package sesstokgen provides a default SessionTokenGenerator implementation.
package sesstokgen

import (
	"crypto/rand"
	"encoding/base64"

	"github.com/romshark/datapages/modules/sessmanager"
)

var _ sessmanager.TokenGenerator = Generator{}

// DefaultLength is the number of random bytes used to generate
// session tokens. 32 bytes provides 256 bits of entropy.
const DefaultLength = 32

// Generator generates cryptographically secure session tokens.
type Generator struct {
	// Length is the number of random bytes to generate.
	// Defaults to DefaultLength if zero.
	Length int
}

// Generate returns a new cryptographically random session token
// encoded as URL-safe base64 without padding.
func (g Generator) Generate() (string, error) {
	length := g.Length
	if length <= 24 {
		length = DefaultLength
	}
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Base64url raw encoding (RawURLEncoding) uses only A-Z, a-z, 0-9, '-', '_'.
	// None of NATS KV syntax ('.', '*', '>') appear in that charset.
	return base64.RawURLEncoding.EncodeToString(b), nil
}

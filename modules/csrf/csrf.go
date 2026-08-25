package csrf

import "io"

// TokenWriter issues CSRF tokens writing them to an [io.Writer].
//
// Implementations are safe for concurrent use.
type TokenWriter interface {
	// WriteToken writes a CSRF token bound to the session named by sessionToken to
	// w and reports how many bytes it wrote. Writes nothing for an empty sessionToken.
	//
	// The token is handed to the client, hence it must not be possible to
	// recover sessionToken from it.
	//
	// WARNING: It's written into a JavaScript string literal of the page as it is,
	// quoted with '. Nothing escapes it! Write only characters that cannot
	// end that string, which rules out ', \ and a line break.
	// See the default implementation [Tokens] for reference.
	WriteToken(w io.Writer, sessionToken string) (n int, err error)
}

// TokenValidator checks CSRF tokens against the session they belong to.
//
// Implementations are safe for concurrent use.
type TokenValidator interface {
	// ValidateToken reports whether token is a valid CSRF token for the
	// session named by sessionToken.
	ValidateToken(sessionToken, token string) bool
}

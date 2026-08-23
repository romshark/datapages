// Package csrf defines the interface for CSRF (Cross-Site Request Forgery)
// token generation and validation and provides the built-in implementation.
//
// [Tokens], the built-in implementation, derives the CSRF token from the
// session token. That token is already a cryptographically random bearer
// credential, it already lives in the session store every process reads,
// and it already changes whenever the visitor re-authenticates. Deriving from
// it leaves no secret to configure, no value to keep in sync between replicas
// and nothing a restart invalidates.
package csrf

// Package subject builds the message broker subjects an event is addressed by.
// A subject is the event's own followed by one token per subject field,
// joined by [Separator].
//
// [Encode] turns any value into one such token. A user ID can be an email
// address, and a wildcard a client sends stays a plain value.
//
// Application code must not import this package.
package subject

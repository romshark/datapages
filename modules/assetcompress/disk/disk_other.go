//go:build !unix

package disk

import "os"

// checkPrivate accepts every directory outside Unix.
// Windows controls access with ACLs, which [os.FileMode] doesn't reflect.
func checkPrivate(*os.Root) error { return nil }

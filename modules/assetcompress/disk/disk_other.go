//go:build !unix

package disk

import "os"

// checkPrivate accepts platforms without Unix ownership checks.
func checkPrivate(*os.Root) error { return nil }

//go:build unix

package disk

import (
	"fmt"
	"os"
	"syscall"
)

// checkPrivate rejects world-writable directories and untrusted owners.
// Group-writable directories remain valid.
func checkPrivate(root *os.Root) error {
	st, err := root.Stat(".")
	if err != nil {
		return err
	}
	if st.Mode().Perm()&0o002 != 0 {
		return fmt.Errorf("%s is writable by every user", root.Name())
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if uid := int(sys.Uid); uid != os.Geteuid() && uid != 0 {
		return fmt.Errorf("%s belongs to user %d, neither the server's user nor root",
			root.Name(), uid)
	}
	return nil
}

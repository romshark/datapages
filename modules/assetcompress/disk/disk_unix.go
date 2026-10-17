//go:build unix

package disk

import (
	"fmt"
	"os"
	"syscall"
)

// checkPrivate refuses a directory that every user can write to or that
// neither the server's user nor root owns, because such a user could plant
// a copy under a predictable key. Group write passes for shared directories.
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

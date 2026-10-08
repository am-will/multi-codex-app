//go:build darwin || linux

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// codexHomeInUse reports whether any process has one of this home's own SQLite databases open:
// Codex's databases in the home itself, or the desktop app's in its sqlite folder. SQLite holds
// a shared lock on byte 128 of a database's "-shm" file, its dead-man switch, for as long as the
// database is open. Linked databases keep that file in the owner's folder, so only this profile's
// own databases are checked.
func codexHomeInUse(home string) (bool, error) {
	for _, dir := range []string{home, filepath.Join(home, "sqlite")} {
		entries, e := os.ReadDir(dir)
		if errors.Is(e, os.ErrNotExist) && dir != home {
			continue
		}
		if e != nil {
			return false, e
		}
		for _, entry := range entries {
			if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), "-shm") {
				continue
			}
			f, e := os.Open(filepath.Join(dir, entry.Name()))
			if e != nil {
				return false, e
			}
			lock := syscall.Flock_t{Type: syscall.F_WRLCK, Start: 128, Len: 1}
			e = syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lock)
			f.Close()
			if e != nil {
				return false, e
			}
			if lock.Type != syscall.F_UNLCK {
				return true, nil
			}
		}
	}
	return false, nil
}

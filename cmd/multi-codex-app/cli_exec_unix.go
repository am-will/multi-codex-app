//go:build !windows

package main

import "syscall"

// execCLI replaces this process with the CLI, so the terminal, signals and exit code are the
// CLI's own.
func execCLI(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}

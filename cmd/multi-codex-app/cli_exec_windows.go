//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

func execCLI(path string, argv, env []string) error {
	cmd := exec.Command(path, argv[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	e := cmd.Run()
	var exit *exec.ExitError
	if errors.As(e, &exit) {
		os.Exit(exit.ExitCode())
	}
	return e
}

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The codex and claude command-line tools read the same account folders as the desktop apps,
// so each profile can also be used from a terminal.

func cliCommand(p Profile) string {
	if p.isClaude() {
		return "claude"
	}
	return "codex"
}

// cliEnvironment replaces any inherited account variables with the profile's own.
func cliEnvironment(p Profile, base []string) []string {
	env := []string{}
	for _, v := range base {
		k, _, _ := strings.Cut(v, "=")
		if k == "CODEX_HOME" || k == "CLAUDE_CONFIG_DIR" {
			continue
		}
		env = append(env, v)
	}
	if name, value := accountVariable(p); value != "" {
		env = append(env, name+"="+value)
	}
	return env
}

// runCommand runs the codex or claude CLI as one profile: run ID [--] [ARGS...]
func runCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: multi-codex-app run ID [-- ARGS...]")
	}
	c, e := readConfig(stateRoot())
	if e != nil {
		return e
	}
	p, e := profile(c, args[0])
	if e != nil {
		return e
	}
	rest := args[1:]
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	name := cliCommand(p)
	path, e := exec.LookPath(name)
	if e != nil {
		return fmt.Errorf("%s was not found on your PATH", name)
	}
	if _, value := accountVariable(p); value != "" {
		if e = os.MkdirAll(value, 0700); e != nil {
			return e
		}
	}
	return execCLI(path, append([]string{name}, rest...), cliEnvironment(p, os.Environ()))
}

// envCommand prints the line that points a shell at one profile's account.
func envCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: multi-codex-app env ID")
	}
	c, e := readConfig(stateRoot())
	if e != nil {
		return e
	}
	p, e := profile(c, args[0])
	if e != nil {
		return e
	}
	name, value := accountVariable(p)
	switch {
	case runtime.GOOS == "windows" && value == "":
		fmt.Printf("Remove-Item Env:%s -ErrorAction SilentlyContinue\n", name)
	case runtime.GOOS == "windows":
		fmt.Printf("$env:%s = %s\n", name, psQuote(value))
	case value == "":
		fmt.Printf("unset %s\n", name)
	default:
		fmt.Printf("export %s=%s\n", name, shQuote(value))
	}
	return nil
}

// shellInit prints one shell function per profile, such as codex-work or claude-personal, for
// zsh and bash: eval "$(multi-codex-app shell-init)"
func shellInit(c Config, cli string) string {
	var b strings.Builder
	b.WriteString("# Multi Codex profile commands. Load with: eval \"$(multi-codex-app shell-init)\"\n")
	seen := map[string]bool{"codex": true, "claude": true}
	for _, p := range c.Profiles {
		name := shellName(p)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		// The function keyword keeps an existing alias of the same name from breaking this line.
		fmt.Fprintf(&b, "unalias %s 2>/dev/null\nfunction %s { %s run %s -- \"$@\"; }\n", name, name, shQuote(cli), shQuote(p.ID))
	}
	return b.String()
}

// shellName turns "Codex (Work)" into codex-work. Only a-z, 0-9 and dashes reach the shell.
func shellName(p Profile) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(launcherName(p)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	name := strings.TrimRight(b.String(), "-")
	if name == "" || name[0] < 'a' || name[0] > 'z' || len(name) > 64 {
		return ""
	}
	return name
}

func shellInitCommand(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: multi-codex-app shell-init")
	}
	c, e := readConfig(stateRoot())
	if e != nil {
		return e
	}
	cli := c.CLIPath
	if cli == "" {
		cli, _ = os.Executable()
	}
	fmt.Print(shellInit(c, filepath.Clean(cli)))
	return nil
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

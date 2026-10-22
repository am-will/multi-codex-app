package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func claudeTestConfig(t *testing.T) (Config, string) {
	t.Helper()
	root := t.TempDir()
	c := Config{Version: 1, AppPath: filepath.Join(root, "ChatGPT.app"), CLIPath: filepath.Join(root, "cli"),
		Profiles: []Profile{{ID: "1", Name: "Primary", CodexHome: filepath.Join(root, ".codex")}}}
	return c, root
}

func TestClaudeProfilesKeepTheOriginalAccountFirst(t *testing.T) {
	c, root := claudeTestConfig(t)
	for _, id := range []string{"2", "3"} {
		if e := appendClaudeProfile(&c, id, root); e != nil {
			t.Fatal(e)
		}
	}
	original, second := c.Profiles[1], c.Profiles[2]
	if !original.isClaude() || original.UserDataDir != "" || original.ClaudeConfigDir != "" || launcherName(original) != "Claude Primary" {
		t.Fatalf("first Claude profile should use the app's own account: %+v", original)
	}
	if second.UserDataDir != filepath.Join(root, "profiles", "3", "desktop") || second.ClaudeConfigDir != filepath.Join(root, "profiles", "3", "claude") {
		t.Fatalf("later Claude profiles need private folders: %+v", second)
	}
	if e := validateConfig(c); e != nil {
		t.Fatal(e)
	}
	if e := saveConfig(root, c); e != nil {
		t.Fatal(e)
	}
	if got, e := readConfig(root); e != nil || !slices.Equal(got.Profiles, c.Profiles) {
		t.Fatal("Claude profiles did not survive a save", e)
	}
}

func TestClaudeProfileValidation(t *testing.T) {
	c, root := claudeTestConfig(t)
	_ = appendClaudeProfile(&c, "2", root)
	for name, change := range map[string]func(*Config){
		"two original accounts": func(c *Config) { c.Profiles = append(c.Profiles, Profile{ID: "9", Name: "Other", App: appClaude}) },
		"sharing chats":         func(c *Config) { c.Profiles[1].ShareChats = true },
		"sharing owner":         func(c *Config) { c.SharingOwner = "2" },
		"codex folder":          func(c *Config) { c.Profiles[1].CodexHome = root },
		"relative config dir":   func(c *Config) { c.Profiles[1].ClaudeConfigDir = "relative" },
		"unknown app":           func(c *Config) { c.Profiles[1].App = "other" },
		"relative Claude app":   func(c *Config) { c.ClaudeAppPath = "Claude.app" },
	} {
		bad := c
		bad.Profiles = slices.Clone(c.Profiles)
		change(&bad)
		if validateConfig(bad) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if checkSharingChange(c, c.Profiles[1]) == nil && sharingSupported() == nil {
		t.Fatal("a Claude profile was allowed to share Codex chats")
	}
}

func TestAccountVariablesReplaceInheritedOnes(t *testing.T) {
	codex := Profile{ID: "2", CodexHome: "/p/2/codex"}
	original := Profile{ID: "3", App: appClaude}
	claude := Profile{ID: "4", App: appClaude, ClaudeConfigDir: "/p/4/claude"}
	base := []string{"PATH=/bin", "CODEX_HOME=/stray", "CLAUDE_CONFIG_DIR=/stray"}
	for _, tc := range []struct {
		p    Profile
		want []string
	}{
		{codex, []string{"PATH=/bin", "CODEX_HOME=/p/2/codex"}},
		{original, []string{"PATH=/bin"}},
		{claude, []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=/p/4/claude"}},
	} {
		if got := cliEnvironment(tc.p, base); !slices.Equal(got, tc.want) {
			t.Errorf("profile %s: got %v want %v", tc.p.ID, got, tc.want)
		}
	}
	if cliCommand(claude) != "claude" || cliCommand(codex) != "codex" {
		t.Fatal("wrong CLI for profile")
	}
}

func TestLaunchersNeedTheirDesktopApp(t *testing.T) {
	c, _ := claudeTestConfig(t)
	claude := Profile{ID: "2", App: appClaude}
	if hasLauncher(c, claude) || !hasLauncher(c, c.Profiles[0]) {
		t.Fatal("Claude launchers need the Claude app; Codex launchers always exist")
	}
	c.ClaudeAppPath = "/Applications/Claude.app"
	if !hasLauncher(c, claude) {
		t.Fatal("Claude launcher skipped although the app is configured")
	}
	if _, e := desktopApp(Config{}, claude); e == nil {
		t.Fatal("launching Claude without the app should explain how to configure it")
	}
}

func TestShellNames(t *testing.T) {
	for p, want := range map[Profile]string{
		{Name: "Work"}:                       "codex-work",
		{Name: "Codex (Work & Research)"}:    "codex-work-research",
		{Name: "Personal", App: appClaude}:   "claude-personal",
		{Name: "Claude 2", App: appClaude}:   "claude-2",
		{Name: "Ünïcode ok", App: appClaude}: "claude-n-code-ok",
	} {
		if got := shellName(p); got != want {
			t.Errorf("%q: got %q want %q", p.Name, got, want)
		}
	}
}

func TestShellInitDefinesQuotedFunctions(t *testing.T) {
	c := Config{Profiles: []Profile{{ID: "1", Name: "Primary"}, {ID: "2", Name: "Work", App: appClaude}, {ID: "3", Name: "work", App: appClaude}}}
	script := shellInit(c, "/Apps/it's here/multi-codex-app")
	if !strings.Contains(script, `function codex-primary { '/Apps/it'\''s here/multi-codex-app' run '1' -- "$@"; }`) {
		t.Fatal("missing or unquoted function:\n" + script)
	}
	if strings.Count(script, "function claude-work ") != 1 {
		t.Fatal("duplicate shell names must be skipped:\n" + script)
	}
	if runtime.GOOS == "windows" {
		return
	}
	// An existing alias with the same name must not break the definitions.
	for _, shell := range []string{"bash", "zsh"} {
		path, e := exec.LookPath(shell)
		if e != nil {
			continue
		}
		prelude := ""
		if shell == "bash" {
			prelude = "shopt -s expand_aliases\n"
		}
		out, e := exec.Command(path, "-c", prelude+"alias codex-primary='echo OLD'\neval \"$1\"\ncodex-primary hi", "sh", shellInit(c, "/bin/echo")).CombinedOutput()
		if e != nil || strings.TrimSpace(string(out)) != "run 1 -- hi" {
			t.Errorf("%s: %v %q", shell, e, out)
		}
	}
}

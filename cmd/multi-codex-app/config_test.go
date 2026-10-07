package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilesAddWithoutChangingExisting(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	c := Config{Version: 1, AppPath: filepath.Join(root, "ChatGPT.app"), CLIPath: filepath.Join(root, "bin", "multi-codex-app")}
	if e := ensureProfiles(&c, 2, root, home); e != nil {
		t.Fatal(e)
	}
	c.Profiles[1].Name = "Work"
	old := c.Profiles[1]
	if e := ensureProfiles(&c, 6, root, home); e != nil {
		t.Fatal(e)
	}
	if c.Profiles[1] != old {
		t.Fatal("existing profile changed")
	}
	if c.Profiles[5].ID != "6" {
		t.Fatal(c.Profiles)
	}
	if e := ensureProfiles(&c, 3, root, home); e == nil {
		t.Fatal("setup removed profiles")
	}
	if e := saveConfig(root, c); e != nil {
		t.Fatal(e)
	}
	got, e := readConfig(root)
	if e != nil || got.Profiles[1] != old {
		t.Fatal(got, e)
	}
}
func TestManifestRejectsTraversalAndDuplicateIDs(t *testing.T) {
	r := t.TempDir()
	c := Config{Version: 1, AppPath: filepath.Join(r, "app"), CLIPath: filepath.Join(r, "cli"), Profiles: []Profile{{ID: "../../bad", Name: "Work", CodexHome: r, UserDataDir: r}}}
	if validateConfig(c) == nil {
		t.Fatal("traversal accepted")
	}
	c.Profiles[0].ID = "2"
	c.Profiles = append(c.Profiles, c.Profiles[0])
	if validateConfig(c) == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestURIValidationPreservesOAuth(t *testing.T) {
	valid := []string{"codex://connector/oauth_callback?code=a%2Bb&state=c%2Fd&returnTo=%2Fsettings", "codex://settings/connections"}
	for _, u := range valid {
		if e := validateURI(u); e != nil {
			t.Fatal(e)
		}
	}
	for _, u := range []string{"https://example.com", "codex://user@connector/oauth_callback", "codex://connector:123/callback", "codex://connector/oauth_callback\nsecret"} {
		if validateURI(u) == nil {
			t.Fatal("invalid URI accepted")
		}
	}
}
func TestNoForeignHomeEnvironmentLeak(t *testing.T) {
	t.Setenv("CODEX_HOME", "wrong-account")
	t.Setenv("CODEX_ELECTRON_USER_DATA_PATH", "wrong-profile")
	t.Setenv("CODEX_DESKTOP_RELAUNCH_OPEN_EVENTS", "old-event")
	env := appEnvironment(Profile{CodexHome: "intended"})
	s := strings.Join(env, "\n")
	if strings.Contains(s, "wrong-account") || strings.Contains(s, "wrong-profile") || strings.Contains(s, "old-event") {
		t.Fatal("profile environment leaked")
	}
	if !strings.Contains(s, "CODEX_HOME=intended") {
		t.Fatal("intended home missing")
	}
}
func TestAtomicWriteAndSymlinkOwnership(t *testing.T) {
	r := t.TempDir()
	p := filepath.Join(r, "config.json")
	if e := atomicWrite(p, []byte("one"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := atomicWrite(p, []byte("two"), 0600); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "two" {
		t.Fatal(e)
	}
	os.MkdirAll(filepath.Join(r, ".local", "bin"), 0755)
	os.WriteFile(filepath.Join(r, ".local", "bin", "multi-codex-app"), []byte("foreign"), 0600)
	if installCLILink(r, filepath.Join(r, "owned-cli")) == nil {
		t.Fatal("overwrote unrelated file")
	}
}
func TestNoShellInterpolation(t *testing.T) {
	s := "name'; $(touch /tmp/bad) \"%"
	if !strings.Contains(psQuote(s), "''") {
		t.Fatal("PowerShell quote missing")
	}
	if !strings.Contains(desktopQuote(s), "\\$") || !strings.Contains(desktopQuote(s), "%%") {
		t.Fatal("desktop escaping missing")
	}
	if plistEscape("<&") != "&lt;&amp;" {
		t.Fatal("plist escaping missing")
	}
}

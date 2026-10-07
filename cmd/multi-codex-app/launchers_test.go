package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLauncherNames(t *testing.T) {
	for name, want := range map[string]string{"Primary": "Codex Primary", "Personal": "Codex Personal", "Codex (Work)": "Codex (Work)", "Codex Personal": "Codex Personal", "Work & Research": "Codex Work & Research"} {
		if got := launcherName(name); got != want {
			t.Fatalf("%q: got %q want %q", name, got, want)
		}
	}
}
func TestNamesRejectCollisionsAndUnsafePaths(t *testing.T) {
	for _, name := range []string{"../Other", "Work/Other", "Work\\Other", "Work:Other", "Work\nOther", "Codex", strings.Repeat("A", 81)} {
		if validateNames([]Profile{{Name: name}}) == nil {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	if validateNames([]Profile{{Name: "Personal"}, {Name: "Codex personal"}}) == nil {
		t.Fatal("equivalent launcher names accepted")
	}
}
func TestLauncherIdentityAndMetadata(t *testing.T) {
	root := t.TempDir()
	p := Profile{ID: "3", Name: "Codex (Work & Research)", CodexHome: root, UserDataDir: root}
	c := Config{Version: 1, AppPath: filepath.Join(root, "ChatGPT.app"), CLIPath: filepath.Join(root, "cli"), Profiles: []Profile{p}}
	app := filepath.Join(root, "launcher.app")
	os.MkdirAll(filepath.Join(app, "Contents"), 0755)
	os.WriteFile(filepath.Join(app, "Contents/Info.plist"), []byte(helperPlist(c, root, p.ID)), 0644)
	v, e := bundleValues(app)
	if e != nil {
		t.Fatal(e)
	}
	if v["CFBundleName"] != p.Name || v["CFBundleDisplayName"] != p.Name {
		t.Fatal("launcher metadata lost exact name")
	}
	if !ownsLauncher(app, root, "3") || ownsLauncher(app, root, "2") || ownsLauncher(app, filepath.Join(root, "other"), "3") {
		t.Fatal("ownership guard incorrect")
	}
}
func TestLauncherPathPersistsWithoutChangingProfileIdentity(t *testing.T) {
	root := t.TempDir()
	p := Profile{ID: "3", Name: "Personal", CodexHome: filepath.Join(root, "auth"), UserDataDir: filepath.Join(root, "desktop"), LauncherPath: filepath.Join(root, "Codex Personal.app")}
	c := Config{Version: 1, AppPath: filepath.Join(root, "app"), CLIPath: filepath.Join(root, "cli"), Profiles: []Profile{p}}
	if e := saveConfig(root, c); e != nil {
		t.Fatal(e)
	}
	got, e := readConfig(root)
	if e != nil || got.Profiles[0] != p {
		t.Fatal("launcher naming changed profile identity", e)
	}
}

func TestLauncherCollisionLeavesUnrelatedAppUntouched(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	target := filepath.Join(home, "Applications", "Codex Personal.app")
	sentinel := filepath.Join(target, "do-not-replace")
	if e := os.MkdirAll(target, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(sentinel, []byte("foreign app"), 0600); e != nil {
		t.Fatal(e)
	}
	c := Config{Profiles: []Profile{{ID: "3", Name: "Personal"}}}
	if installMacLaunchers(root, &c, home, "no-helper", false) == nil {
		t.Fatal("accepted foreign app collision")
	}
	if b, e := os.ReadFile(sentinel); e != nil || string(b) != "foreign app" {
		t.Fatal("foreign app was changed")
	}
}

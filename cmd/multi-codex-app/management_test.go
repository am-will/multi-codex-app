package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func managementFixture(t *testing.T) (string, Config) {
	t.Helper()
	root := t.TempDir()
	c := Config{Version: 1, AppPath: filepath.Join(root, "app"), CLIPath: filepath.Join(root, "bin/cli")}
	if e := ensureProfiles(&c, 3, root, root); e != nil {
		t.Fatal(e)
	}
	for _, p := range c.Profiles {
		os.MkdirAll(p.CodexHome, 0700)
		if p.UserDataDir != "" {
			os.MkdirAll(p.UserDataDir, 0700)
		}
	}
	if e := saveConfig(root, c); e != nil {
		t.Fatal(e)
	}
	return root, c
}
func TestWizardManagementRoutesAndBack(t *testing.T) {
	_, c := managementFixture(t)
	for input, want := range map[string]string{"1\n": "setup", "2\n": "edit", "3\n1\n": "add", "3\n2\n": "remove", "3\n0\n1\n": "setup", "4\n": "share", "0\n": "quit"} {
		var out bytes.Buffer
		w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader(input))}
		got, e := w.managementAction(c)
		if e != nil || got != want {
			t.Fatalf("%q: %s %v", input, got, e)
		}
	}
}
func TestWizardSelectedEditOnlyAsksChosenFieldsAndCanCancel(t *testing.T) {
	root, c := managementFixture(t)
	before, _ := os.ReadFile(filepath.Join(root, "config.json"))
	var out bytes.Buffer
	w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader("2\n1\nWork\nno\n"))}
	if e := w.editExisting(root, c); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "Icon color (name or") || !strings.Contains(out.String(), "Name [Secondary]") {
		t.Fatal("asked unrelated fields", out.String())
	}
	if strings.Contains(out.String(), "PURPLE") || strings.Contains(out.String(), "TEAL") {
		t.Fatal("name-only edit displayed the color palette", out.String())
	}
	after, _ := os.ReadFile(filepath.Join(root, "config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("cancel changed saved configuration")
	}
	if c.Profiles[1].Name != "Secondary" {
		t.Fatal("edit mutated the caller's profiles before apply")
	}
}
func TestWizardColorEditsShowPalette(t *testing.T) {
	for _, input := range []string{"2\n2\npurple\nno\n", "2\n3\nWork\npurple\nno\n"} {
		root, c := managementFixture(t)
		var out bytes.Buffer
		w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader(input))}
		if e := w.editExisting(root, c); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(out.String(), "TEAL") || !strings.Contains(out.String(), "Icon color (name or") {
			t.Fatal("color edit omitted the palette or color prompt", out.String())
		}
	}
}
func TestRemovalArchivesIdentityAndNeverReusesData(t *testing.T) {
	root, c := managementFixture(t)
	old := c.Profiles[2]
	marker := filepath.Join(old.CodexHome, "test-private-state")
	os.WriteFile(marker, []byte("saved sign-in marker"), 0600)
	next, removed, e := archivedConfig(c, "3")
	if e != nil {
		t.Fatal(e)
	}
	if removed != old || len(next.Profiles) != 2 || next.RemovedProfiles[0] != old || len(c.Profiles) != 3 {
		t.Fatal("removal changed identity or caller")
	}
	if e = saveConfig(root, next); e != nil {
		t.Fatal(e)
	}
	restored, e := readConfig(root)
	if e != nil || restored.RemovedProfiles[0] != old {
		t.Fatal("removed identity was lost", e)
	}
	if e = appendProfile(&next, "3", root, root); e == nil {
		t.Fatal("reused retained number")
	}
	if e = appendProfile(&next, "1", root, root); e == nil {
		t.Fatal("reused active number")
	}
	if e = ensureProfiles(&next, 3, root, root); e != nil {
		t.Fatal(e)
	}
	if next.Profiles[2].ID != "4" || next.Profiles[2].CodexHome == old.CodexHome {
		t.Fatal("addition reused old private data")
	}
	b, e := os.ReadFile(marker)
	if e != nil || string(b) != "saved sign-in marker" {
		t.Fatal("retained data changed")
	}
	// The final removal remains readable so wizard can add/restore later.
	for len(restored.Profiles) > 0 {
		restored, _, e = archivedConfig(restored, restored.Profiles[0].ID)
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = saveConfig(root, restored); e != nil {
		t.Fatal(e)
	}
	if _, e = readConfig(root); e != nil {
		t.Fatal("empty active configuration cannot be reopened", e)
	}
}
func TestNumbersProtectUnlistedDirectoriesAndNumericNames(t *testing.T) {
	root, c := managementFixture(t)
	os.MkdirAll(filepath.Join(root, "profiles/9"), 0700)
	if e := appendProfile(&c, "9", root, root); e == nil {
		t.Fatal("reused an unlisted directory")
	}
	if id, e := nextProfileID(c, root); e != nil || id != "10" {
		t.Fatal(id, e)
	}
	c.Profiles[0].Name = "3"
	got, e := profile(c, "3")
	if e != nil || got.ID != "3" {
		t.Fatal("numeric name shadowed profile ID")
	}
	c.Profiles[0].Name = "Profile 10"
	if e = ensureProfiles(&c, 4, root, root); e != nil {
		t.Fatal(e)
	}
	if e = validateNames(c.Profiles); e != nil {
		t.Fatal("auto-generated name blocked the wizard", e)
	}
}
func TestOnlyChangedProfileIsSelected(t *testing.T) {
	_, before := managementFixture(t)
	after := before
	after.Profiles = append([]Profile(nil), before.Profiles...)
	after.Profiles[1].Name = "Work"
	after.Profiles[1].IconColor = "teal"
	if got := changedProfileIDs(before, after); !reflect.DeepEqual(got, []string{"2"}) {
		t.Fatal(got)
	}
	if got := changedProfileIDs(before, before); len(got) != 0 {
		t.Fatal("unchanged profiles selected", got)
	}
	archived, primary, e := archivedConfig(before, "1")
	if e != nil || primaryProfile(archived) != primary {
		t.Fatal("normal links lost original Primary", e)
	}
}

func TestSingleAddChoosesNumberNameColorWithoutApplyingOnCancel(t *testing.T) {
	root, c := managementFixture(t)
	before, _ := os.ReadFile(filepath.Join(root, "config.json"))
	app := filepath.Join(root, "desktop")
	if runtime.GOOS == "darwin" {
		app += ".app"
		os.MkdirAll(filepath.Join(app, "Contents/MacOS"), 0755)
	} else {
		os.WriteFile(app, []byte("test executable"), 0755)
	}
	var out bytes.Buffer
	w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader("2\n8\nWork Test\n5\nno\n"))}
	if e := w.addSingle(root, c, app, false); e != nil {
		t.Fatal(e, out.String())
	}
	if !strings.Contains(out.String(), "already used") || !strings.Contains(out.String(), "#8") || !strings.Contains(out.String(), "TEAL") {
		t.Fatal("missing single-add choices", out.String())
	}
	after, _ := os.ReadFile(filepath.Join(root, "config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("cancel applied configuration")
	}
	if _, e := os.Stat(filepath.Join(root, "profiles/8")); !os.IsNotExist(e) {
		t.Fatal("cancel created profile state")
	}
}

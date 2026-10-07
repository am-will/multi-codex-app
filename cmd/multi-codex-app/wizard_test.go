package main

import (
	"bufio"
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestWizardNamesColorsAndInvalidAnswers(t *testing.T) {
	var out bytes.Buffer
	// Reject count, unsafe name, duplicate normalized launcher name, and invalid color.
	w := wizardPrompt{reader: bufio.NewReader(strings.NewReader("0\n3\n../bad\nSecondary\nWork\nred\nPURPLE\nCodex (Personal)\nblue\n\nyellow\n")), out: &out}
	if n, e := w.count("Total", 2); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	root := t.TempDir()
	c := Config{Version: 1, AppPath: filepath.Join(root, "app"), CLIPath: filepath.Join(root, "cli")}
	if e := ensureProfiles(&c, 3, root, root); e != nil {
		t.Fatal(e)
	}
	original := append([]Profile(nil), c.Profiles...)
	if e := w.appearance(&c, 0); e != nil {
		t.Fatal(e, out.String())
	}
	for i, p := range c.Profiles {
		if p.ID != original[i].ID || p.CodexHome != original[i].CodexHome || p.UserDataDir != original[i].UserDataDir {
			t.Fatal("wizard changed profile data identity")
		}
	}
	if c.Profiles[0].Name != "Work" || c.Profiles[0].IconColor != "purple" || c.Profiles[1].Name != "Codex (Personal)" || c.Profiles[2].IconColor != "yellow" {
		t.Fatal(c.Profiles)
	}
	if !strings.Contains(out.String(), "another profile") || !strings.Contains(out.String(), "Choose white") {
		t.Fatal("invalid answers were not explained")
	}
}
func TestWizardAddOnlyEditsNewProfilesAndCanCancel(t *testing.T) {
	r := t.TempDir()
	c := Config{}
	if e := ensureProfiles(&c, 3, r, r); e != nil {
		t.Fatal(e)
	}
	c.Profiles[0].IconColor = "teal"
	original := append([]Profile(nil), c.Profiles[:2]...)
	var out bytes.Buffer
	w := wizardPrompt{reader: bufio.NewReader(strings.NewReader("Research\nblue\nno\n")), out: &out}
	if e := w.appearance(&c, 2); e != nil {
		t.Fatal(e)
	}
	for i, p := range original {
		if c.Profiles[i] != p {
			t.Fatal("adding overwrote existing appearance")
		}
	}
	apply, e := w.review(c, false)
	if e != nil || apply {
		t.Fatal("cancel ignored", e)
	}
	if _, e = w.ask("Another question", "default"); e == nil {
		t.Fatal("EOF treated as approval")
	}
}

func TestWizardAcceptsNumberedIconChoices(t *testing.T) {
	root := t.TempDir()
	c := Config{}
	if e := ensureProfiles(&c, 3, root, root); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	w := wizardPrompt{reader: bufio.NewReader(strings.NewReader("\n1\n\n2\n\n3\n")), out: &out}
	if e := w.appearance(&c, 0); e != nil {
		t.Fatal(e)
	}
	for i, color := range []string{"white", "yellow", "blue"} {
		if c.Profiles[i].IconColor != color {
			t.Fatal("wrong numbered color", c.Profiles)
		}
	}
}

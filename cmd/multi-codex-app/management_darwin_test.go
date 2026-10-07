package main

import (
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMacSelectedLauncherEditAndRemovalPreserveOtherProfile(t *testing.T) {
	if _, e := assets.ReadFile("assets/macos/MultiCodexHelper"); e != nil {
		t.Skip("compile the native helper first to run this integration check")
	}
	root, home := t.TempDir(), t.TempDir()
	c := Config{Version: 1, AppPath: filepath.Join(root, "Official.app"), CLIPath: filepath.Join(root, "cli")}
	for _, id := range []string{"77771", "77772"} {
		if e := appendProfile(&c, id, root, home); e != nil {
			t.Fatal(e)
		}
	}
	for i := range c.Profiles {
		c.Profiles[i].Name = "Integration Test " + c.Profiles[i].ID
		os.MkdirAll(c.Profiles[i].CodexHome, 0700)
		os.MkdirAll(c.Profiles[i].UserDataDir, 0700)
	}
	if e := installIcons(root); e != nil {
		t.Fatal(e)
	}
	if e := extractAsset("macos/MultiCodexHelper", helperPath(root), 0755); e != nil {
		t.Fatal(e)
	}
	if e := installMacLaunchers(root, &c, home, helperPath(root), false); e != nil {
		t.Fatal(e)
	}
	defer func() {
		for _, p := range c.Profiles {
			exec.Command(lsregister, "-u", p.LauncherPath).Run()
		}
	}()
	untouched := c.Profiles[1]
	path := filepath.Join(untouched.LauncherPath, "Contents/Info.plist")
	before, _ := os.ReadFile(path)
	info, _ := os.Stat(path)
	c.Profiles[0].Name = "Integration Test Renamed"
	c.Profiles[0].IconColor = "blue"
	if e := installMacLaunchers(root, &c, home, helperPath(root), false, c.Profiles[0].ID); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(path)
	nextInfo, _ := os.Stat(path)
	if sha256.Sum256(before) != sha256.Sum256(after) || !info.ModTime().Equal(nextInfo.ModTime()) || c.Profiles[1] != untouched {
		t.Fatal("editing one recreated or changed another launcher")
	}
	// Adding one should also leave both existing launchers in place.
	other := c.Profiles[0]
	otherFile := filepath.Join(other.LauncherPath, "Contents/Info.plist")
	otherInfo, _ := os.Stat(otherFile)
	if e := appendProfile(&c, "77773", root, home); e != nil {
		t.Fatal(e)
	}
	c.Profiles[2].Name = "Integration Test Added"
	os.MkdirAll(c.Profiles[2].CodexHome, 0700)
	os.MkdirAll(c.Profiles[2].UserDataDir, 0700)
	if e := installMacLaunchers(root, &c, home, helperPath(root), false, "77773"); e != nil {
		t.Fatal(e)
	}
	afterOther, _ := os.Stat(otherFile)
	afterUntouched, _ := os.Stat(path)
	if !otherInfo.ModTime().Equal(afterOther.ModTime()) || !info.ModTime().Equal(afterUntouched.ModTime()) {
		t.Fatal("adding one recreated existing launchers")
	}
	for _, p := range c.Profiles {
		os.WriteFile(filepath.Join(p.CodexHome, "test-private-state"), []byte("retained "+p.ID), 0600)
	}
	removed := c.Profiles[0]
	if e := removeProfileAt(root, c, removed.ID, home); e != nil {
		t.Fatal(e)
	}
	next, e := readConfig(root)
	if e != nil || len(next.Profiles) != 2 || next.Profiles[0] != untouched || len(next.RemovedProfiles) != 1 {
		t.Fatal("wrong profile removed", e)
	}
	if _, e = os.Stat(removed.LauncherPath); !os.IsNotExist(e) {
		t.Fatal("removed launcher still present")
	}
	for _, p := range c.Profiles {
		b, e := os.ReadFile(filepath.Join(p.CodexHome, "test-private-state"))
		if e != nil || string(b) != "retained "+p.ID {
			t.Fatal("private state changed", e)
		}
	}
}

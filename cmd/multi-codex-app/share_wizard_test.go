//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func runSharingWizard(t *testing.T, root, input string) string {
	t.Helper()
	var out bytes.Buffer
	w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader(input))}
	if e := w.shareSettings(root); e != nil {
		t.Fatal(e, out.String())
	}
	return out.String()
}

func TestWizardSharingChangesOneProfileAndCanCancel(t *testing.T) {
	root, _, _ := sharingFixture(t)
	// Pick the owner first (refused), then #2, chats only, but cancel.
	out := runSharingWizard(t, root, "1\n1\n2\n2\nno\n0\n")
	if !strings.Contains(out, "#1 holds the shared chats and memories") || !strings.Contains(out, "own chats · own memories  →  shared chats · own memories") {
		t.Fatal(out)
	}
	if c, _ := readConfig(root); c.Profiles[1].ShareChats {
		t.Fatal("cancelled change was applied")
	}
	out = runSharingWizard(t, root, "1\n2\n2\nyes\n0\n")
	if c, _ := readConfig(root); !c.Profiles[1].ShareChats || c.Profiles[1].ShareMemories {
		t.Fatal("chats-only sharing was not applied", out)
	}
	if !strings.Contains(out, "now uses the shared chats") || !strings.Contains(out, "shared chats · own memories") {
		t.Fatal(out)
	}
	// Choosing the current mode again changes nothing.
	out = runSharingWizard(t, root, "1\n2\n2\n0\n")
	if !strings.Contains(out, "Nothing changed.") {
		t.Fatal(out)
	}
	// The owner cannot change while #2 shares.
	out = runSharingWizard(t, root, "4\n0\n")
	if !strings.Contains(out, "Stop sharing for every profile first") {
		t.Fatal(out)
	}
}

func TestWizardSharesEveryProfileThenStopsAndChangesOwner(t *testing.T) {
	root, _, _ := sharingFixture(t)
	runSharingWizard(t, root, "2\nyes\n0\n")
	c, _ := readConfig(root)
	for _, p := range c.Profiles[1:] {
		if !p.ShareChats || !p.ShareMemories {
			t.Fatal("not every profile shares", p)
		}
	}
	report := inspectInstallation(root, c, root, runtime.GOOS, "")
	var out bytes.Buffer
	(&wizardPrompt{out: &out}).report(report)
	if !strings.Contains(out.String(), "holds the shared chats and memories") || !strings.Contains(out.String(), "shared chats · shared memories") {
		t.Fatal(out.String())
	}
	out2 := runSharingWizard(t, root, "3\nyes\n0\n")
	if !strings.Contains(out2, "Chats started while sharing stay") {
		t.Fatal(out2)
	}
	if c, _ = readConfig(root); len(sharingProfiles(c)) != 0 {
		t.Fatal("sharing was not stopped")
	}
	runSharingWizard(t, root, "4\n3\nyes\n0\n")
	if c, _ = readConfig(root); sharingOwner(c).ID != "3" {
		t.Fatal("owner was not changed")
	}
	if out := runSharingWizard(t, root, "0\n"); !strings.Contains(out, "Codex Profile 3's folder (the owner)") {
		t.Fatal(out)
	}
}

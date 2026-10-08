//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveAndUninstallWaitUntilSharingIsOff(t *testing.T) {
	root, _, _ := sharingFixture(t)
	if e := applySharing(root, "2", true, false); e != nil {
		t.Fatal(e)
	}
	c, _ := readConfig(root)
	if e := removeProfileAt(root, c, "2", root); e == nil || !strings.Contains(e.Error(), "multi-codex-app share 2 off") {
		t.Fatal("removed a sharing profile", e)
	}
	if e := removeProfileAt(root, c, "1", root); e == nil || !strings.Contains(e.Error(), "holds the shared") {
		t.Fatal("removed the owner while #2 shares", e)
	}
	// Uninstall is checked directly: a real uninstall in a test would touch this machine's helper.
	if e := checkUninstallWhileSharing(c); e == nil || !strings.Contains(e.Error(), "multi-codex-app share 2 off") {
		t.Fatal("uninstall allowed while #2 shares", e)
	}
	var out bytes.Buffer
	w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader("2\n"))}
	if e := w.removeExisting(root, c); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "share 2 off") || strings.Contains(out.String(), "Remove this profile?") {
		t.Fatal(out.String())
	}
	if after, _ := readConfig(root); len(after.Profiles) != 3 {
		t.Fatal("a profile was removed")
	}
	if e := applySharing(root, "2", false, false); e != nil {
		t.Fatal(e)
	}
	c, _ = readConfig(root)
	if checkUninstallWhileSharing(c) != nil || checkRemovalWhileSharing(c, c.Profiles[0]) != nil || checkRemovalWhileSharing(c, c.Profiles[1]) != nil {
		t.Fatal("still blocked after sharing was turned off")
	}
}

func TestShareCommandModesOwnerAndStatus(t *testing.T) {
	root, _, home := sharingFixture(t)
	t.Setenv("MULTI_CODEX_ROOT", root)
	if e := run([]string{"share", "2", "chats"}); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"share", "2", "everything"}); e == nil || !strings.Contains(e.Error(), "all, chats, memories, or off") {
		t.Fatal("unknown mode accepted", e)
	}
	if e := run([]string{"share", "owner", "3"}); e == nil {
		t.Fatal("owner changed while #2 shares")
	}
	c, _ := readConfig(root)
	var out bytes.Buffer
	printSharing(&out, c)
	if !strings.Contains(out.String(), "Codex Primary") || !strings.Contains(out.String(), "owner") || !strings.Contains(out.String(), "shared chats · own memories") || !strings.Contains(out.String(), "own chats · own memories") {
		t.Fatal(out.String())
	}
	result := sharingResult(c, c.Profiles[1], false, false)
	if !strings.Contains(result, "back on its own") || !strings.Contains(result, "started while sharing stay") {
		t.Fatal(result)
	}
	if !strings.Contains(sharingResult(c, c.Profiles[1], true, false), filepath.Join(home, privateDirName)) {
		t.Fatal("result does not say where own data waits")
	}

	if e := doctorSharing(c); e != nil {
		t.Fatal(e)
	}
	os.Remove(filepath.Join(home, "sessions"))
	if e := doctorSharing(c); e == nil {
		t.Fatal("doctor missed a removed link")
	}
	if e := run([]string{"share", "2", "chats"}); e != nil {
		t.Fatal(e)
	}
	if e := doctorSharing(c); e != nil {
		t.Fatal("rerunning the same mode did not repair the link", e)
	}

	if e := run([]string{"share", "2", "off"}); e != nil {
		t.Fatal(e)
	}
	if e := run([]string{"share", "owner", "3"}); e != nil {
		t.Fatal(e)
	}
	c, _ = readConfig(root)
	if c.SharingOwner != "3" {
		t.Fatal("owner not saved")
	}
	if e := run([]string{"share", "owner", "1"}); e != nil {
		t.Fatal(e)
	}
	if c, _ = readConfig(root); c.SharingOwner != "" {
		t.Fatal("default owner should not be written to the configuration")
	}
}

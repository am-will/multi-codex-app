//go:build darwin || linux

package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
}
func readText(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func assertLink(t *testing.T, path, target string) {
	t.Helper()
	got, e := os.Readlink(path)
	if e != nil || got != target {
		t.Fatalf("%s links to %q (%v), want %q", path, got, e, target)
	}
}
func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, e := os.Lstat(path); e == nil {
		t.Fatalf("%s should not exist", path)
	}
}

// sharingFixture gives the owner (#1) and #2 their own chats and memories.
func sharingFixture(t *testing.T) (root, owner, home string) {
	t.Helper()
	root, c := managementFixture(t)
	owner, home = c.Profiles[0].CodexHome, c.Profiles[1].CodexHome
	writeFile(t, filepath.Join(owner, "sessions", "a.jsonl"), "owner chat")
	writeFile(t, filepath.Join(owner, "state_5.sqlite"), "owner db")
	writeFile(t, filepath.Join(owner, "memories", "memory_summary.md"), "owner memory")
	writeFile(t, filepath.Join(home, "sessions", "b.jsonl"), "own chat")
	writeFile(t, filepath.Join(home, "state_5.sqlite"), "own db")
	writeFile(t, filepath.Join(home, "state_5.sqlite-wal"), "own wal")
	writeFile(t, filepath.Join(home, "session_index.jsonl"), "own index")
	writeFile(t, filepath.Join(home, "memories", "memory_summary.md"), "own memory")
	writeFile(t, filepath.Join(home, "memories_1.sqlite"), "own memory db")
	writeFile(t, filepath.Join(home, "logs_2.sqlite"), "own logs")
	return root, owner, home
}

func TestShareChatsLinksOwnerCopyAndOffRestoresOwnData(t *testing.T) {
	root, owner, home := sharingFixture(t)
	private := filepath.Join(home, privateDirName)
	if e := applySharing(root, "2", true, false); e != nil {
		t.Fatal(e)
	}
	assertLink(t, filepath.Join(home, "sessions"), filepath.Join(owner, "sessions"))
	assertLink(t, filepath.Join(home, "state_5.sqlite"), filepath.Join(owner, "state_5.sqlite"))
	assertLink(t, filepath.Join(home, "thread-writer-locks"), filepath.Join(owner, "thread-writer-locks"))
	if readText(t, filepath.Join(home, "sessions", "a.jsonl")) != "owner chat" {
		t.Fatal("shared chats are not the owner's")
	}
	assertMissing(t, filepath.Join(home, "state_5.sqlite-wal"))
	if readText(t, filepath.Join(private, "own", "state_5.sqlite-wal")) != "own wal" || readText(t, filepath.Join(private, "own", "sessions", "b.jsonl")) != "own chat" {
		t.Fatal("own chats were not set aside with their write-ahead log")
	}
	// Codex builds memories from the chats a profile sees, so its own memories wait aside too.
	assertMissing(t, filepath.Join(home, "memories"))
	assertMissing(t, filepath.Join(home, "session_index.jsonl"))
	if readText(t, filepath.Join(private, "own", "memories", "memory_summary.md")) != "own memory" {
		t.Fatal("own memories were not set aside")
	}
	if readText(t, filepath.Join(home, "logs_2.sqlite")) != "own logs" || readText(t, filepath.Join(home, "memories_1.sqlite")) != "own memory db" {
		t.Fatal("private databases moved")
	}
	if !isDir(filepath.Join(owner, "thread-writer-locks")) || !regularFile(filepath.Join(owner, "goals_1.sqlite")) {
		t.Fatal("owner items were not created for links")
	}
	if c, _ := readConfig(root); !c.Profiles[1].ShareChats || c.Profiles[1].ShareMemories {
		t.Fatal("sharing mode not saved", c.Profiles[1])
	}

	// Use the profile while it shares, then turn sharing off.
	writeFile(t, filepath.Join(home, "sessions", "c.jsonl"), "made while sharing")
	writeFile(t, filepath.Join(home, "memories", "memory_summary.md"), "built from shared chats")
	writeFile(t, filepath.Join(home, "session_index.jsonl"), "sharing index")
	if e := applySharing(root, "2", false, false); e != nil {
		t.Fatal(e)
	}
	if readText(t, filepath.Join(home, "sessions", "b.jsonl")) != "own chat" || readText(t, filepath.Join(home, "state_5.sqlite")) != "own db" || readText(t, filepath.Join(home, "state_5.sqlite-wal")) != "own wal" {
		t.Fatal("own chats were not restored")
	}
	assertMissing(t, filepath.Join(home, "sessions", "c.jsonl"))
	if readText(t, filepath.Join(owner, "sessions", "c.jsonl")) != "made while sharing" {
		t.Fatal("chats made while sharing left the shared history")
	}
	if readText(t, filepath.Join(home, "memories", "memory_summary.md")) != "own memory" || readText(t, filepath.Join(home, "session_index.jsonl")) != "own index" {
		t.Fatal("own memories or chat names were not restored")
	}
	if readText(t, filepath.Join(private, "while-sharing", "memories", "memory_summary.md")) != "built from shared chats" {
		t.Fatal("memories built while sharing were not kept for next time")
	}
	assertMissing(t, filepath.Join(private, "own"))
	assertMissing(t, filepath.Join(private, "detached"))

	// Sharing again brings back what the profile built while sharing before.
	if e := applySharing(root, "2", true, false); e != nil {
		t.Fatal(e)
	}
	if readText(t, filepath.Join(home, "memories", "memory_summary.md")) != "built from shared chats" || readText(t, filepath.Join(home, "session_index.jsonl")) != "sharing index" {
		t.Fatal("memories built while sharing did not return")
	}
}

func TestShareMemoriesIndependentlyOfChats(t *testing.T) {
	root, owner, home := sharingFixture(t)
	private := filepath.Join(home, privateDirName)
	if e := applySharing(root, "2", false, true); e != nil {
		t.Fatal(e)
	}
	assertLink(t, filepath.Join(home, "memories"), filepath.Join(owner, "memories"))
	assertLink(t, filepath.Join(home, "memories_v2"), filepath.Join(owner, "memories_v2"))
	if readText(t, filepath.Join(home, "memories", "memory_summary.md")) != "owner memory" {
		t.Fatal("shared memories are not the owner's")
	}
	if readText(t, filepath.Join(home, "sessions", "b.jsonl")) != "own chat" || readText(t, filepath.Join(home, "session_index.jsonl")) != "own index" {
		t.Fatal("memories-only sharing touched chats")
	}
	if readText(t, filepath.Join(private, "own", "memories", "memory_summary.md")) != "own memory" {
		t.Fatal("own memories were not set aside")
	}

	if e := applySharing(root, "2", true, true); e != nil {
		t.Fatal(e)
	}
	assertLink(t, filepath.Join(home, "memories"), filepath.Join(owner, "memories"))
	assertLink(t, filepath.Join(home, "sessions"), filepath.Join(owner, "sessions"))

	if e := applySharing(root, "2", false, false); e != nil {
		t.Fatal(e)
	}
	if readText(t, filepath.Join(home, "memories", "memory_summary.md")) != "own memory" || readText(t, filepath.Join(home, "sessions", "b.jsonl")) != "own chat" {
		t.Fatal("own data was not restored")
	}
	if readText(t, filepath.Join(owner, "memories", "memory_summary.md")) != "owner memory" {
		t.Fatal("owner memories changed")
	}
	// Nothing is left aside, so the private folder is tidied away.
	assertMissing(t, private)
}

func TestShareRepairsReplacedLinksAndReportsNewerDatabases(t *testing.T) {
	root, owner, home := sharingFixture(t)
	private := filepath.Join(home, privateDirName)
	if e := applySharing(root, "2", true, true); e != nil {
		t.Fatal(e)
	}
	c, _ := readConfig(root)
	if problems := sharingProblems(c, c.Profiles[1]); len(problems) != 0 {
		t.Fatal(problems)
	}
	link := filepath.Join(home, "state_5.sqlite")
	os.Remove(link)
	writeFile(t, link, "written while unlinked")
	if problems := sharingProblems(c, c.Profiles[1]); len(problems) != 1 || !strings.Contains(problems[0], "state_5.sqlite") {
		t.Fatal(problems)
	}
	if e := applySharing(root, "2", true, true); e != nil {
		t.Fatal(e)
	}
	assertLink(t, link, filepath.Join(owner, "state_5.sqlite"))
	found, _ := filepath.Glob(filepath.Join(private, "detached", "*", "codex-home", "state_5.sqlite"))
	if len(found) != 1 || readText(t, found[0]) != "written while unlinked" {
		t.Fatal("replaced link data was not kept", found)
	}

	// A newer chat database only this profile has means Codex here is newer than the owner's.
	writeFile(t, filepath.Join(home, "state_6.sqlite"), "newer")
	if problems := sharingProblems(c, c.Profiles[1]); len(problems) != 1 || !strings.Contains(problems[0], "state_6.sqlite is newer") {
		t.Fatal(problems)
	}
	writeFile(t, filepath.Join(home, "state_4.sqlite"), "older leftover")
	os.Remove(filepath.Join(home, "state_6.sqlite"))
	if problems := sharingProblems(c, c.Profiles[1]); len(problems) != 0 {
		t.Fatal("an older leftover database was reported", problems)
	}
}

func TestShareResumesAfterInterruption(t *testing.T) {
	root, owner, home := sharingFixture(t)
	private := filepath.Join(home, privateDirName)
	// The first item moved before the change stopped; the saved mode is still off.
	m := newSharingMove(home, owner)
	if e := m.loadState(); e != nil {
		t.Fatal(e)
	}
	if e := m.transition(chatItems[0], ownData, sharedLink); e != nil {
		t.Fatal(e)
	}
	m.state[chatItems[0].name] = sharedLink
	if e := m.saveState(); e != nil {
		t.Fatal(e)
	}
	if e := applySharing(root, "2", true, false); e != nil {
		t.Fatal(e)
	}
	assertMissing(t, filepath.Join(private, "detached"))
	if readText(t, filepath.Join(private, "own", "sessions", "b.jsonl")) != "own chat" {
		t.Fatal("own chats were lost after an interrupted change")
	}
	// Interrupt the way back as well: restore one item, then finish.
	m = newSharingMove(home, owner)
	m.loadState()
	m.transition(chatItems[0], sharedLink, ownData)
	m.state[chatItems[0].name] = ownData
	m.saveState()
	if e := applySharing(root, "2", false, false); e != nil {
		t.Fatal(e)
	}
	assertMissing(t, filepath.Join(private, "detached"))
	if readText(t, filepath.Join(home, "sessions", "b.jsonl")) != "own chat" || readText(t, filepath.Join(home, "state_5.sqlite")) != "own db" {
		t.Fatal("own chats were not restored after an interrupted change")
	}
}

func TestShareRefusesOwnerSqliteHomeAndOwnerChangesWhileShared(t *testing.T) {
	root, _, home := sharingFixture(t)
	if e := applySharing(root, "1", true, true); e == nil || !strings.Contains(e.Error(), "holds the shared") {
		t.Fatal("owner shared with itself", e)
	}
	writeFile(t, filepath.Join(home, "config.toml"), "model = \"gpt-5\"\nsqlite_home = \"/elsewhere\"\n")
	if e := applySharing(root, "2", true, false); e == nil || !strings.Contains(e.Error(), "sqlite_home") {
		t.Fatal("separate sqlite_home was accepted", e)
	}
	os.Remove(filepath.Join(home, "config.toml"))
	if e := applySharing(root, "2", true, false); e != nil {
		t.Fatal(e)
	}
	if e := setSharingOwner(root, "3"); e == nil || !strings.Contains(e.Error(), "#2") {
		t.Fatal("owner changed while #2 shares", e)
	}
	if e := applySharing(root, "2", false, false); e != nil {
		t.Fatal(e)
	}
	if e := setSharingOwner(root, "3"); e != nil {
		t.Fatal(e)
	}
	c, _ := readConfig(root)
	if sharingOwner(c).ID != "3" {
		t.Fatal("owner not saved")
	}
	c.Profiles[2].ShareChats = true
	if validateConfig(c) == nil {
		t.Fatal("owner allowed to share with itself")
	}
	if e := applySharing(root, "1", true, false); e != nil {
		t.Fatal("profile 1 could not share with a new owner", e)
	}
}

func TestHoldSQLiteLockHelper(t *testing.T) {
	path := os.Getenv("MULTI_CODEX_HOLD_LOCK")
	if path == "" {
		t.Skip("helper process for TestCodexHomeInUseSeesAnotherProcess")
	}
	f, e := os.OpenFile(path, os.O_RDWR, 0)
	if e != nil {
		os.Exit(2)
	}
	lock := syscall.Flock_t{Type: syscall.F_RDLCK, Start: 128, Len: 1}
	if syscall.FcntlFlock(f.Fd(), syscall.F_SETLK, &lock) != nil {
		os.Exit(3)
	}
	os.Stdout.WriteString("locked\n")
	io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}
func TestCodexHomeInUseSeesAnotherProcess(t *testing.T) {
	root, _, home := sharingFixture(t)
	shm := filepath.Join(home, "logs_2.sqlite-shm")
	writeFile(t, shm, strings.Repeat("\x00", 32768))
	if inUse, e := codexHomeInUse(home); e != nil || inUse {
		t.Fatal("idle home reported in use", e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHoldSQLiteLockHelper$")
	cmd.Env = append(os.Environ(), "MULTI_CODEX_HOLD_LOCK="+shm)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	lines := bufio.NewScanner(stdout)
	for lines.Scan() && lines.Text() != "locked" {
	}
	if inUse, e := codexHomeInUse(home); e != nil || !inUse {
		t.Fatal("open database not detected", e)
	}
	if e := applySharing(root, "2", true, false); e == nil || !strings.Contains(e.Error(), "is open") {
		t.Fatal("changed sharing for an open profile", e)
	}
	stdin.Close()
	cmd.Wait()
	if inUse, e := codexHomeInUse(home); e != nil || inUse {
		t.Fatal("closed database still reported in use", e)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Sharing replaces parts of a profile's Codex home with links to the sharing owner's copies.
// A profile's own data is never deleted: it moves into privateDirName inside the same Codex
// home while sharing is on and moves back when sharing is turned off.
const privateDirName = ".multi-codex-private"

type shareItem struct {
	name     string
	dir      bool
	database bool
}

// Chat storage that Codex already coordinates across processes (SQLite locking and the
// thread-writer-locks folder), so every sharing profile can use the owner's copy live.
var chatItems = []shareItem{
	{name: "sessions", dir: true},
	{name: "archived_sessions", dir: true},
	{name: "attachments", dir: true},
	{name: "generated_images", dir: true},
	{name: "thread-writer-locks", dir: true},
	{name: "state_5.sqlite", database: true},
	{name: "thread_history_1.sqlite", database: true},
	{name: "goals_1.sqlite", database: true},
}

// Codex rewrites the chat-name index by renaming a temporary file over it, which would replace
// a link. It stays private and is set aside with the profile's own chats.
var chatPrivateItems = []shareItem{{name: "session_index.jsonl"}}

// Codex refuses to write memories into a linked folder, so the owner writes them and sharing
// profiles read them through links. Memory databases stay private, so resetting memories in a
// sharing profile cannot clear the owner's memories.
var memoryItems = []shareItem{
	{name: "memories", dir: true},
	{name: "memories_v2", dir: true},
	{name: "memories_extensions", dir: true},
}

// Future Codex releases rename chat databases when their format changes.
var chatDatabasePattern = regexp.MustCompile(`^(state|thread_history|goals)_[0-9]+\.sqlite$`)
var sqliteSidecars = []string{"-wal", "-shm", "-journal"}
var sqliteHomeSetting = regexp.MustCompile(`(?m)^\s*sqlite_home\s*=`)

// occupant names what currently sits at an item's path in a profile's Codex home.
type occupant string

const (
	ownData     occupant = "own"           // the profile's own data
	sharedLink  occupant = "shared"        // a link to the owner's copy
	sharingData occupant = "while-sharing" // local data kept only while the profile shares chats
)

func chatOccupant(chats bool) occupant {
	if chats {
		return sharedLink
	}
	return ownData
}
func chatPrivateOccupant(chats bool) occupant {
	if chats {
		return sharingData
	}
	return ownData
}
func memoryOccupant(chats, memories bool) occupant {
	switch {
	case memories:
		return sharedLink
	case chats:
		// Codex builds a profile's own memories from the chats it can see, so memories built
		// from shared chats are kept apart from the ones built from its own chats.
		return sharingData
	}
	return ownData
}

func sharingOwnerID(c Config) string {
	if c.SharingOwner != "" {
		return c.SharingOwner
	}
	return "1"
}
func sharingOwner(c Config) Profile {
	for _, p := range append(append([]Profile(nil), c.Profiles...), c.RemovedProfiles...) {
		if p.ID == sharingOwnerID(c) {
			return p
		}
	}
	return primaryProfile(c)
}
func sharingProfiles(c Config) []Profile {
	var shared []Profile
	for _, p := range append(append([]Profile(nil), c.Profiles...), c.RemovedProfiles...) {
		if p.ShareChats || p.ShareMemories {
			shared = append(shared, p)
		}
	}
	return shared
}
func parseSharingMode(mode string) (chats, memories bool, e error) {
	switch strings.ToLower(mode) {
	case "all":
		return true, true, nil
	case "chats":
		return true, false, nil
	case "memories":
		return false, true, nil
	case "off":
		return false, false, nil
	}
	return false, false, errors.New("choose all, chats, memories, or off")
}
func sharingModeName(chats, memories bool) string {
	switch {
	case chats && memories:
		return "all"
	case chats:
		return "chats"
	case memories:
		return "memories"
	}
	return "off"
}
func sharingSummary(chats, memories bool) string {
	switch {
	case chats && memories:
		return "shared chats · shared memories"
	case chats:
		return "shared chats · own memories"
	case memories:
		return "own chats · shared memories"
	}
	return "own chats · own memories"
}
func sharingSupported() error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		// SQLite on Windows does not follow links when it places its write-ahead log.
		return errors.New("sharing chats and memories is available on macOS and Linux")
	}
	return nil
}

// checkSharingChange explains why a profile cannot change sharing right now, if it cannot.
func checkSharingChange(c Config, p Profile) error {
	if e := sharingSupported(); e != nil {
		return e
	}
	owner := sharingOwner(c)
	if p.ID == owner.ID {
		return fmt.Errorf("#%s %s holds the shared chats and memories; choose another profile", owner.ID, launcherName(owner.Name))
	}
	if filepath.Clean(p.CodexHome) == filepath.Clean(owner.CodexHome) {
		return fmt.Errorf("#%s already uses the owner's Codex folder", p.ID)
	}
	if !isDir(owner.CodexHome) {
		return fmt.Errorf("the owner's Codex folder is missing: %s", owner.CodexHome)
	}
	if !isDir(p.CodexHome) {
		return fmt.Errorf("profile %s home missing", p.ID)
	}
	for _, home := range []string{p.CodexHome, owner.CodexHome} {
		b, e := os.ReadFile(filepath.Join(home, "config.toml"))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if sqliteHomeSetting.Match(b) {
			return fmt.Errorf("%s sets sqlite_home, so its chats are stored elsewhere and cannot be shared", filepath.Join(home, "config.toml"))
		}
	}
	inUse, e := codexHomeInUse(p.CodexHome)
	if e != nil {
		return e
	}
	if inUse {
		return fmt.Errorf("%s is open; quit it, then try again", launcherName(p.Name))
	}
	return nil
}

// applySharing moves a closed profile to the requested sharing mode and saves it.
func applySharing(root, id string, chats, memories bool) error {
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	p, e := profile(c, id)
	if e != nil {
		return e
	}
	if e = checkSharingChange(c, p); e != nil {
		return e
	}
	m := newSharingMove(p.CodexHome, sharingOwner(c).CodexHome)
	if e = m.apply(p.ShareChats, p.ShareMemories, chats, memories); e != nil {
		return e
	}
	latest, e := readConfig(root)
	if e != nil {
		return e
	}
	for i := range latest.Profiles {
		if latest.Profiles[i].ID == p.ID {
			latest.Profiles[i].ShareChats = chats
			latest.Profiles[i].ShareMemories = memories
		}
	}
	return saveConfig(root, latest)
}

func setSharingOwner(root, id string) error {
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	p, e := profile(c, id)
	if e != nil {
		return e
	}
	if shared := sharingProfiles(c); len(shared) > 0 {
		var ids []string
		for _, s := range shared {
			ids = append(ids, "#"+s.ID)
		}
		return fmt.Errorf("turn off sharing for %s first; the owner can only change while nothing is shared", strings.Join(ids, ", "))
	}
	c.SharingOwner = p.ID
	if p.ID == "1" {
		c.SharingOwner = ""
	}
	return saveConfig(root, c)
}

type sharingMove struct {
	home, owner, private, stamp string
	state                       map[string]occupant
}

func newSharingMove(home, owner string) *sharingMove {
	return &sharingMove{home: home, owner: owner, private: filepath.Join(home, privateDirName), stamp: time.Now().Format("20060102-150405")}
}
func (m *sharingMove) statePath() string { return filepath.Join(m.private, "state.json") }

// items lists everything sharing manages, including chat databases from newer Codex releases
// that the owner already uses and any database a previous change moved.
func (m *sharingMove) items() (linked, chatPrivate, memory []shareItem) {
	linked = append([]shareItem(nil), chatItems...)
	known := map[string]bool{}
	for _, item := range linked {
		known[item.name] = true
	}
	var names []string
	entries, _ := os.ReadDir(m.owner)
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	for name := range m.state {
		names = append(names, name)
	}
	for _, name := range names {
		if !known[name] && chatDatabasePattern.MatchString(name) {
			known[name] = true
			linked = append(linked, shareItem{name: name, database: true})
		}
	}
	return linked, chatPrivateItems, memoryItems
}
func (m *sharingMove) loadState() error {
	m.state = map[string]occupant{}
	b, e := os.ReadFile(m.statePath())
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &m.state); e != nil {
		return fmt.Errorf("cannot read %s: %w", m.statePath(), e)
	}
	return nil
}

// apply moves each item between occupants. Progress is recorded after every item, so an
// interrupted change can simply be run again.
func (m *sharingMove) apply(oldChats, oldMemories, chats, memories bool) error {
	if e := m.loadState(); e != nil {
		return e
	}
	linked, chatPrivate, memory := m.items()
	type step struct {
		item     shareItem
		from, to occupant
	}
	var steps []step
	for _, item := range linked {
		steps = append(steps, step{item, chatOccupant(oldChats), chatOccupant(chats)})
	}
	for _, item := range chatPrivate {
		steps = append(steps, step{item, chatPrivateOccupant(oldChats), chatPrivateOccupant(chats)})
	}
	for _, item := range memory {
		steps = append(steps, step{item, memoryOccupant(oldChats, oldMemories), memoryOccupant(chats, memories)})
	}
	for _, s := range steps {
		from := s.from
		if recorded, ok := m.state[s.item.name]; ok {
			from = recorded
		}
		if e := m.transition(s.item, from, s.to); e != nil {
			return fmt.Errorf("could not move %s: %w", s.item.name, e)
		}
		m.state[s.item.name] = s.to
		if e := m.saveState(); e != nil {
			return e
		}
	}
	return m.tidy(chats || memories)
}
func (m *sharingMove) saveState() error {
	b, e := json.MarshalIndent(m.state, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(m.statePath(), append(b, '\n'), 0600)
}

const privateReadme = `Multi Codex keeps this profile's own chats and memories here while the profile
shares chats or memories with another profile. They move back when sharing is
turned off with: multi-codex-app share ID off

Nothing in this folder is deleted by Multi Codex.

own/            this profile's own chats and memories
while-sharing/  files this profile keeps only while it shares chats, such as
                memories Codex built from the shared chats
detached/       anything Multi Codex found in the way of a link, kept as found
`

// tidy writes an explanation while data is set aside and removes empty bookkeeping afterwards.
func (m *sharingMove) tidy(sharing bool) error {
	readme := filepath.Join(m.private, "README.txt")
	if sharing {
		return atomicWrite(readme, []byte(privateReadme), 0600)
	}
	for _, slot := range []occupant{ownData, sharingData} {
		_ = os.Remove(filepath.Join(m.private, string(slot)))
	}
	if entries, e := os.ReadDir(m.private); e == nil {
		for _, entry := range entries {
			if name := entry.Name(); name != "state.json" && name != "README.txt" {
				// Something is still set aside; keep the explanation and progress record.
				return atomicWrite(readme, []byte(privateReadme), 0600)
			}
		}
	}
	_ = os.Remove(m.statePath())
	_ = os.Remove(readme)
	_ = os.Remove(m.private)
	return nil
}

func (m *sharingMove) transition(item shareItem, from, to occupant) error {
	if from == to {
		if to == sharedLink {
			// Repair a link that something replaced.
			return m.link(item)
		}
		return nil
	}
	if from == sharedLink {
		if e := m.unlink(item); e != nil {
			return e
		}
	} else if e := m.moveItem(item, m.home, filepath.Join(m.private, string(from))); e != nil {
		return e
	}
	if to == sharedLink {
		return m.link(item)
	}
	return m.moveItem(item, filepath.Join(m.private, string(to)), m.home)
}
func itemFiles(item shareItem) []string {
	names := []string{item.name}
	if item.database {
		for _, suffix := range sqliteSidecars {
			names = append(names, item.name+suffix)
		}
	}
	return names
}
func (m *sharingMove) ownerPath(item shareItem) string { return filepath.Join(m.owner, item.name) }
func (m *sharingMove) isLink(path string, item shareItem) bool {
	target, e := os.Readlink(path)
	return e == nil && target == m.ownerPath(item)
}

// moveItem moves an item with its SQLite sidecar files, which only make sense together.
func (m *sharingMove) moveItem(item shareItem, fromDir, toDir string) error {
	main := filepath.Join(fromDir, item.name)
	if m.isLink(main, item) {
		// A link left by an interrupted change holds no data.
		if e := os.Remove(main); e != nil {
			return e
		}
	}
	present := false
	for _, name := range itemFiles(item) {
		if _, e := os.Lstat(filepath.Join(fromDir, name)); e == nil {
			present = true
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	if !present {
		return nil
	}
	for _, name := range itemFiles(item) {
		if e := m.clear(filepath.Join(toDir, name), item); e != nil {
			return e
		}
	}
	if e := os.MkdirAll(toDir, 0700); e != nil {
		return e
	}
	for _, name := range itemFiles(item) {
		if e := os.Rename(filepath.Join(fromDir, name), filepath.Join(toDir, name)); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	return nil
}
func (m *sharingMove) link(item shareItem) error {
	target := m.ownerPath(item)
	if e := ensureOwnerItem(target, item); e != nil {
		return e
	}
	path := filepath.Join(m.home, item.name)
	if !m.isLink(path, item) {
		if e := m.clear(path, item); e != nil {
			return e
		}
		if e := os.Symlink(target, path); e != nil {
			return e
		}
	}
	// SQLite keeps the write-ahead log beside the owner's database; stale local ones are kept aside.
	for _, name := range itemFiles(item)[1:] {
		if e := m.clear(filepath.Join(m.home, name), item); e != nil {
			return e
		}
	}
	return nil
}
func (m *sharingMove) unlink(item shareItem) error {
	for _, name := range itemFiles(item) {
		if e := m.clear(filepath.Join(m.home, name), item); e != nil {
			return e
		}
	}
	return nil
}

// clear empties a path: our own links are removed, and anything else is kept under detached/.
func (m *sharingMove) clear(path string, item shareItem) error {
	if _, e := os.Lstat(path); errors.Is(e, os.ErrNotExist) {
		return nil
	} else if e != nil {
		return e
	}
	if m.isLink(path, item) {
		return os.Remove(path)
	}
	place := "codex-home"
	if filepath.Clean(filepath.Dir(path)) != filepath.Clean(m.home) {
		place = filepath.Base(filepath.Dir(path))
	}
	dir := filepath.Join(m.private, "detached", m.stamp, place)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	dest := filepath.Join(dir, filepath.Base(path))
	for n := 2; ; n++ {
		if _, e := os.Lstat(dest); errors.Is(e, os.ErrNotExist) {
			break
		}
		dest = filepath.Join(dir, filepath.Base(path)+"-"+strconv.Itoa(n))
	}
	return os.Rename(path, dest)
}

// ensureOwnerItem creates an empty folder or database in the owner when it has none yet, so
// links never dangle. SQLite treats an empty file as a new database.
func ensureOwnerItem(path string, item shareItem) error {
	info, e := os.Stat(path)
	if e == nil {
		if info.IsDir() != item.dir {
			return fmt.Errorf("the owner's %s has an unexpected type", path)
		}
		return nil
	}
	if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if item.dir {
		return os.MkdirAll(path, 0700)
	}
	f, e := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(e, os.ErrExist) {
		return nil
	}
	if e != nil {
		return e
	}
	return f.Close()
}

// sharingProblems reports links that are missing or replaced, for doctor.
func sharingProblems(c Config, p Profile) []string {
	if !p.ShareChats && !p.ShareMemories {
		return nil
	}
	m := newSharingMove(p.CodexHome, sharingOwner(c).CodexHome)
	if e := m.loadState(); e != nil {
		return []string{e.Error()}
	}
	linked, _, memory := m.items()
	var expected []shareItem
	if p.ShareChats {
		expected = append(expected, linked...)
	}
	if p.ShareMemories {
		expected = append(expected, memory...)
	}
	var problems []string
	newest := map[string]int{}
	for _, item := range expected {
		if kind, version, ok := chatDatabaseVersion(item.name); ok && version > newest[kind] {
			newest[kind] = version
		}
		if !m.isLink(filepath.Join(m.home, item.name), item) {
			problems = append(problems, item.name+" is not linked to the owner's copy")
		}
	}
	if p.ShareChats {
		entries, _ := os.ReadDir(m.home)
		for _, entry := range entries {
			if kind, version, ok := chatDatabaseVersion(entry.Name()); ok && version > newest[kind] {
				problems = append(problems, entry.Name()+" is newer than the owner's chats; open the owner's app once so Codex updates it")
			}
		}
	}
	return problems
}
func chatDatabaseVersion(name string) (string, int, bool) {
	if !chatDatabasePattern.MatchString(name) {
		return "", 0, false
	}
	stem := strings.TrimSuffix(name, ".sqlite")
	cut := strings.LastIndex(stem, "_")
	version, e := strconv.Atoi(stem[cut+1:])
	return stem[:cut], version, e == nil
}

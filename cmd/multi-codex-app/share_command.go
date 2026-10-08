package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const shareUsage = "usage: multi-codex-app share [ID all|chats|memories|off] | share owner ID"

func shareCommand(args []string) error {
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	switch {
	case len(args) == 0:
		printSharing(os.Stdout, c)
		return nil
	case len(args) == 2 && args[0] == "owner":
		if e = setSharingOwner(root, args[1]); e != nil {
			return e
		}
		c, e = readConfig(root)
		if e != nil {
			return e
		}
		owner := sharingOwner(c)
		fmt.Printf("Shared chats and memories will live in #%s %s.\n", owner.ID, launcherName(owner.Name))
		return nil
	case len(args) == 2:
		chats, memories, e := parseSharingMode(args[1])
		if e != nil {
			return fmt.Errorf("%v; %s", e, shareUsage)
		}
		p, e := profile(c, args[0])
		if e != nil {
			return e
		}
		if e = applySharing(root, p.ID, chats, memories); e != nil {
			return e
		}
		fmt.Println(sharingResult(c, p, chats, memories))
		return nil
	}
	return errors.New(shareUsage)
}

func printSharing(w io.Writer, c Config) {
	owner := sharingOwner(c)
	fmt.Fprintf(w, "Shared chats and memories live in #%s %s (%s).\n", owner.ID, launcherName(owner.Name), owner.CodexHome)
	entries := append([]Profile(nil), c.Profiles...)
	for _, p := range c.RemovedProfiles {
		if p.ShareChats || p.ShareMemories {
			entries = append(entries, p)
		}
	}
	for i, p := range entries {
		state := sharingSummary(p.ShareChats, p.ShareMemories)
		if p.ID == owner.ID {
			state = "owner"
		}
		if i >= len(c.Profiles) {
			state += " (removed)"
		}
		fmt.Fprintf(w, "%s  %-24s  %s\n", p.ID, terminalText(launcherName(p.Name)), state)
	}
	if sharingSupported() == nil {
		fmt.Fprintln(w, "Change one: multi-codex-app share ID all|chats|memories|off")
	}
}

func sharingResult(c Config, p Profile, chats, memories bool) string {
	owner := sharingOwner(c)
	name := fmt.Sprintf("#%s %s", p.ID, launcherName(p.Name))
	ownerName := fmt.Sprintf("#%s %s", owner.ID, launcherName(owner.Name))
	var lines []string
	switch {
	case chats && memories:
		lines = append(lines, name+" now uses the shared chats and memories in "+ownerName+".")
	case chats:
		lines = append(lines, name+" now uses the shared chats in "+ownerName+" and keeps its own memories.")
	case memories:
		lines = append(lines, name+" now reads the shared memories in "+ownerName+" and keeps its own chats.")
	default:
		lines = append(lines, name+" is back on its own chats and memories.")
		if p.ShareChats {
			lines = append(lines, "Chats it started while sharing stay in the shared history.")
		}
	}
	if chats {
		lines = append(lines, "Every app can read shared chats; only the account that started a chat can continue it.")
	}
	if memories {
		lines = append(lines, ownerName+" writes new memories while you use it; this profile reads them.")
	}
	if chats || memories {
		lines = append(lines, fmt.Sprintf("Its own data waits in %s until you run: multi-codex-app share %s off", filepath.Join(p.CodexHome, privateDirName), p.ID))
	}
	return strings.Join(lines, "\n")
}

package main

import (
	"fmt"
	"strings"
)

var sharingModes = []struct {
	label           string
	chats, memories bool
}{
	{"Chats and memories", true, true},
	{"Chats only — keep its own memories", true, false},
	{"Memories only — keep its own chats", false, true},
	{"Neither — its own chats and memories", false, false},
}

func (w *wizardPrompt) shareSettings(root string) error {
	if e := sharingSupported(); e != nil {
		fmt.Fprintln(w.out, "     "+e.Error()+".")
		return nil
	}
	for {
		c, e := readConfig(root)
		if e != nil {
			return e
		}
		w.section("03", "SHARE CHATS AND MEMORIES")
		w.sharingOverview(c)
		n, e := w.menu("Choose an action", []string{
			"Change one profile — chats, memories, both, or neither",
			"Share chats and memories with every profile",
			"Stop sharing for every profile",
			"Choose the owner — the profile whose folder holds shared data",
		})
		if e != nil {
			return e
		}
		switch n {
		case 0:
			return nil
		case 1:
			e = w.shareOne(root, c)
		case 2:
			e = w.shareEvery(root, c, true, true)
		case 3:
			e = w.shareEvery(root, c, false, false)
		case 4:
			e = w.chooseOwner(root, c)
		}
		if e != nil {
			return e
		}
	}
}
func (w *wizardPrompt) sharingOverview(c Config) {
	s := styleFor(w.out)
	owner := sharingOwner(c)
	fmt.Fprintf(w.out, "     Shared chats and memories live in %s's folder (the owner).\n\n", terminalText(launcherName(owner.Name)))
	for _, p := range c.Profiles {
		state := sharingSummary(p.ShareChats, p.ShareMemories)
		if p.ID == owner.ID {
			state = "owner"
		}
		fmt.Fprintf(w.out, "     %s  %s  %s  %s\n", s.accent("#"+p.ID), s.paint("1", launcherName(p.Name)), s.icon(iconColor(p)), s.dim(state))
	}
	fmt.Fprintln(w.out)
	for _, line := range []string{
		"· Shared chats are live: every sharing app sees the same chats.",
		"  Only the account that started a chat can continue it.",
		"· Shared memories are written by the owner's app and read by the others.",
		"  They grow while you use the owner, so make it your most-used profile.",
		"· Turning sharing off gives a profile back its own chats and memories.",
		"  Nothing is deleted. Chats it started while sharing stay shared.",
		"· Quit an app before changing its sharing.",
	} {
		fmt.Fprintln(w.out, "     "+s.dim(line))
	}
	fmt.Fprintln(w.out)
}
func (w *wizardPrompt) shareOne(root string, c Config) error {
	owner := sharingOwner(c)
	var p Profile
	for {
		value, e := w.ask("Which profile? (number, 0 to go back)", "0")
		if e != nil {
			return e
		}
		if value == "0" {
			return nil
		}
		p, e = profile(c, value)
		if e != nil {
			fmt.Fprintln(w.out, "     Choose an existing profile from the list.")
			continue
		}
		if p.ID == owner.ID {
			fmt.Fprintf(w.out, "     #%s holds the shared chats and memories. Choose another profile, or change the owner.\n", p.ID)
			continue
		}
		break
	}
	fmt.Fprintln(w.out)
	n, e := w.menu(fmt.Sprintf("What should #%s share?", p.ID), sharingLabels())
	if e != nil || n == 0 {
		return e
	}
	mode := sharingModes[n-1]
	if mode.chats == p.ShareChats && mode.memories == p.ShareMemories && len(sharingProblems(c, p)) == 0 {
		fmt.Fprintln(w.out, "     Nothing changed.")
		return nil
	}
	apply, e := w.reviewSharing([]Profile{p}, mode.chats, mode.memories)
	if e != nil || !apply {
		return e
	}
	w.applySharingChange(root, c, p, mode.chats, mode.memories)
	return nil
}
func (w *wizardPrompt) shareEvery(root string, c Config, chats, memories bool) error {
	owner := sharingOwner(c)
	var targets []Profile
	for _, p := range c.Profiles {
		if p.ID != owner.ID && (p.ShareChats != chats || p.ShareMemories != memories) {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		fmt.Fprintln(w.out, "     Nothing to change.")
		return nil
	}
	var blocked []string
	for _, p := range targets {
		if e := checkSharingChange(c, p); e != nil {
			blocked = append(blocked, fmt.Sprintf("#%s: %v", p.ID, e))
		}
	}
	if len(blocked) > 0 {
		fmt.Fprintln(w.out, "     Nothing was changed yet:")
		for _, reason := range blocked {
			fmt.Fprintln(w.out, "     "+terminalText(reason))
		}
		return nil
	}
	apply, e := w.reviewSharing(targets, chats, memories)
	if e != nil || !apply {
		return e
	}
	for _, p := range targets {
		if !w.applySharingChange(root, c, p, chats, memories) {
			break
		}
	}
	return nil
}
func (w *wizardPrompt) reviewSharing(targets []Profile, chats, memories bool) (bool, error) {
	w.section("04", "APPLY SHARING")
	s := styleFor(w.out)
	ownDataAside, chatsLeave := false, false
	for _, p := range targets {
		fmt.Fprintf(w.out, "     %s  %s\n", s.dim("#"+p.ID), s.paint("1", launcherName(p.Name)))
		fmt.Fprintf(w.out, "         %s  →  %s\n", sharingSummary(p.ShareChats, p.ShareMemories), s.accent(sharingSummary(chats, memories)))
		if !p.ShareChats && !p.ShareMemories && (chats || memories) {
			ownDataAside = true
		}
		if p.ShareChats && !chats {
			chatsLeave = true
		}
	}
	fmt.Fprintln(w.out)
	if ownDataAside {
		fmt.Fprintln(w.out, s.dim("     Own chats and memories wait in each profile's Codex folder until sharing is off."))
	}
	if chatsLeave {
		fmt.Fprintln(w.out, s.dim("     Chats started while sharing stay in the shared history."))
	}
	fmt.Fprintln(w.out, s.dim("     Nothing is deleted."))
	return w.yesNo("Apply this change?", true)
}
func (w *wizardPrompt) applySharingChange(root string, c Config, p Profile, chats, memories bool) bool {
	if e := applySharing(root, p.ID, chats, memories); e != nil {
		fmt.Fprintf(w.out, "     #%s was not changed: %s\n", p.ID, terminalText(e.Error()))
		return false
	}
	for _, line := range strings.Split(sharingResult(c, p, chats, memories), "\n") {
		fmt.Fprintln(w.out, "     "+terminalText(line))
	}
	return true
}
func (w *wizardPrompt) chooseOwner(root string, c Config) error {
	if len(sharingProfiles(c)) > 0 {
		fmt.Fprintln(w.out, "     Stop sharing for every profile first; the owner can only change while nothing is shared.")
		return nil
	}
	owner := sharingOwner(c)
	for {
		value, e := w.ask("Which profile should hold shared chats and memories?", owner.ID)
		if e != nil {
			return e
		}
		p, e := profile(c, value)
		if e != nil {
			fmt.Fprintln(w.out, "     Choose an existing profile from the list.")
			continue
		}
		if p.ID == owner.ID {
			fmt.Fprintln(w.out, "     Nothing changed.")
			return nil
		}
		w.section("04", "CHOOSE THE OWNER")
		fmt.Fprintf(w.out, "     #%s  %s will hold shared chats and memories.\n", p.ID, terminalText(launcherName(p.Name)))
		fmt.Fprintln(w.out, styleFor(w.out).dim("     Its own chats and memories become the shared ones."))
		apply, e := w.yesNo("Apply this change?", true)
		if e != nil || !apply {
			return e
		}
		if e = setSharingOwner(root, p.ID); e != nil {
			fmt.Fprintln(w.out, "     "+terminalText(e.Error()))
			return nil
		}
		fmt.Fprintf(w.out, "     #%s now holds shared chats and memories.\n", p.ID)
		return nil
	}
}
func sharingLabels() []string {
	var labels []string
	for _, mode := range sharingModes {
		labels = append(labels, mode.label)
	}
	return labels
}

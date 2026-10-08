package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type installedLauncher struct{ ID, Name, Path, Icon, Kind string }
type installationReport struct {
	AppPath, AppStatus, HelperStatus string
	Profiles                         []Profile
	Launchers                        []installedLauncher
	Warnings                         []string
	SharingOwner                     string // set only while some profile shares
}

func inspectInstallation(root string, c Config, home, platform, appOverride string) installationReport {
	report := installationReport{Profiles: c.Profiles, AppPath: c.AppPath, HelperStatus: "not installed"}
	if len(sharingProfiles(c)) > 0 {
		report.SharingOwner = sharingOwner(c).ID
	}
	if appOverride != "" {
		report.AppPath, _ = filepath.Abs(appOverride)
	}
	if report.AppPath == "" {
		report.AppPath, _ = detectApp()
	}
	report.AppStatus = "not found"
	if report.AppPath != "" && checkApp(report.AppPath) == nil {
		report.AppStatus = "found"
	}
	switch platform {
	case "darwin":
		if regularFile(helperPath(root)) && regularFile(filepath.Join(home, "Library/LaunchAgents", helperID+".plist")) {
			report.HelperStatus = "installed"
		}
		seen := map[string]bool{}
		dirs := []string{filepath.Join(home, "Applications"), filepath.Join(home, "Applications/Multi Codex Profiles"), "/Applications"}
		for _, p := range c.Profiles {
			if p.LauncherPath != "" {
				dirs = append(dirs, filepath.Dir(p.LauncherPath))
			}
		}
		for _, dir := range dirs {
			entries, e := os.ReadDir(dir)
			if e != nil {
				continue
			}
			for _, entry := range entries {
				path := filepath.Join(dir, entry.Name())
				if seen[path] || !strings.HasSuffix(entry.Name(), ".app") {
					continue
				}
				seen[path] = true
				v, e := bundleValues(path)
				if e != nil {
					continue
				}
				id := strings.TrimPrefix(v["CFBundleIdentifier"], helperID+".profile")
				n, err := strconv.Atoi(id)
				managed := err == nil && n > 0 && strconv.Itoa(n) == id && ownsLauncher(path, root, id)
				legacy := legacyLauncher(path)
				if !managed && !legacy {
					continue
				}
				kind := "managed"
				if legacy {
					id = "—"
					kind = "existing Edi launcher"
				}
				name := v["CFBundleDisplayName"]
				if name == "" {
					name = v["CFBundleName"]
				}
				if name == "" {
					name = strings.TrimSuffix(entry.Name(), ".app")
				}
				icon := v["CFBundleIconFile"]
				if icon != "" && filepath.Ext(icon) == "" {
					icon += ".icns"
				}
				artwork := "missing"
				if icon != "" && filepath.Base(icon) == icon {
					artwork = inspectIcon(filepath.Join(path, "Contents/Resources", icon))
				}
				report.Launchers = append(report.Launchers, installedLauncher{ID: id, Name: name, Path: path, Icon: artwork, Kind: kind})
			}
		}
	case "linux":
		data := os.Getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local/share")
		}
		dir := filepath.Join(data, "applications")
		if regularFile(filepath.Join(dir, "multi-codex-helper.desktop")) {
			report.HelperStatus = "installed"
		}
		cli := c.CLIPath
		if cli == "" {
			cli = filepath.Join(root, "bin", "multi-codex-app")
		}
		files, _ := filepath.Glob(filepath.Join(dir, "multi-codex-profile-*.desktop"))
		for _, path := range files {
			values := desktopValues(path)
			id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "multi-codex-profile-"), ".desktop")
			expected := "Exec=" + desktopQuote(cli) + " --root " + desktopQuote(root) + " launch " + id
			if values["Exec"] != strings.TrimPrefix(expected, "Exec=") {
				continue
			}
			report.Launchers = append(report.Launchers, installedLauncher{ID: id, Name: values["Name"], Path: path, Icon: inspectIcon(values["Icon"]), Kind: "managed"})
		}
	case "windows":
		inspectWindowsInstallation(root, c, &report)
	}
	return report
}
func regularFile(path string) bool { s, e := os.Stat(path); return e == nil && s.Mode().IsRegular() }
func desktopValues(path string) map[string]string {
	b, _ := os.ReadFile(path)
	values := map[string]string{}
	inside := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inside = line == "[Desktop Entry]"
			continue
		}
		if inside {
			if key, value, ok := strings.Cut(line, "="); ok {
				values[key] = value
			}
		}
	}
	return values
}
func (w *wizardPrompt) report(r installationReport) {
	s := styleFor(w.out)
	w.section("01", "YOUR CURRENT INSTALLATION")
	fmt.Fprintf(w.out, "     Codex Desktop  %s", s.accent(r.AppStatus))
	if r.AppPath != "" {
		fmt.Fprintf(w.out, "  %s", s.dim(terminalText(r.AppPath)))
	}
	fmt.Fprintln(w.out)
	fmt.Fprintf(w.out, "     Helper         %s\n", s.accent(r.HelperStatus))
	fmt.Fprintf(w.out, "\n     %s\n", s.accent(fmt.Sprintf("%d profiles configured · %d launchers found", len(r.Profiles), len(r.Launchers))))
	seen := map[string]bool{}
	for _, p := range r.Profiles {
		found := false
		for _, l := range r.Launchers {
			if l.Kind == "managed" && l.ID == p.ID {
				found = true
				seen[l.Path] = true
				fmt.Fprintf(w.out, "\n     %s  %s  %s\n", s.dim("#"+p.ID), s.paint("1", terminalText(l.Name)), s.icon(l.Icon))
				state := "launcher installed"
				if l.Icon != iconColor(p) {
					state += " · configured icon: " + iconColor(p)
				}
				if l.Name != launcherName(p.Name) {
					state += " · configured name: " + terminalText(launcherName(p.Name))
				}
				fmt.Fprintln(w.out, "         "+s.dim(state))
			}
		}
		if !found {
			fmt.Fprintf(w.out, "\n     %s  %s  %s\n         %s\n", s.dim("#"+p.ID), s.paint("1", terminalText(launcherName(p.Name))), s.icon(iconColor(p)), s.paint("33", "launcher missing · showing saved preferences"))
		}
		if !isDir(p.CodexHome) || (p.UserDataDir != "" && !isDir(p.UserDataDir)) {
			fmt.Fprintln(w.out, "         "+s.paint("33", "profile data directory unavailable"))
		}
		if p.ID == r.SharingOwner {
			fmt.Fprintln(w.out, "         "+s.dim("holds the shared chats and memories"))
		} else if r.SharingOwner != "" && (p.ShareChats || p.ShareMemories) {
			fmt.Fprintln(w.out, "         "+s.dim(sharingSummary(p.ShareChats, p.ShareMemories)))
		}
	}
	for _, l := range r.Launchers {
		if !seen[l.Path] {
			fmt.Fprintf(w.out, "\n     %s  %s\n         %s\n", s.paint("1", terminalText(l.Name)), s.icon(l.Icon), s.dim(l.Kind+" · not in this profile configuration"))
		}
	}
	if len(r.Profiles) == 0 && len(r.Launchers) == 0 {
		fmt.Fprintln(w.out, s.dim("     No Multi Codex profiles or launchers installed yet."))
	}
	for _, warning := range r.Warnings {
		fmt.Fprintln(w.out, "     "+s.paint("33", terminalText(warning)))
	}
	fmt.Fprintln(w.out, "\n"+s.dim("     Checked saved settings, launcher names, and installed icon artwork."))
}
func reportCurrentInstallation(w *wizardPrompt, root string, c Config, app string) {
	home, _ := os.UserHomeDir()
	w.report(inspectInstallation(root, c, home, runtime.GOOS, app))
	if len(c.RemovedProfiles) > 0 {
		fmt.Fprintf(w.out, "     %d removed profiles have retained data. Use list --removed / restore ID.\n", len(c.RemovedProfiles))
	}
}

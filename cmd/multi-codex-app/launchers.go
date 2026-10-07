package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"
)

type LauncherBackup struct {
	Original string `json:"original"`
	Backup   string `json:"backup"`
}

func launcherName(name string) string {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "codex ") || strings.HasPrefix(lower, "codex(") {
		return name
	}
	return "Codex " + name
}
func validateNames(profiles []Profile) error {
	seen := map[string]bool{}
	for _, p := range profiles {
		if strings.TrimSpace(p.Name) != p.Name || p.Name == "" || utf8.RuneCountInString(p.Name) > 80 || strings.ContainsAny(p.Name, `/\:*?"<>|`) || strings.HasSuffix(p.Name, ".") {
			return errors.New("names must be 1–80 characters with no path separators or filesystem-reserved characters")
		}
		for _, r := range p.Name {
			if unicode.IsControl(r) {
				return errors.New("names cannot contain control characters")
			}
		}
		if strings.EqualFold(p.Name, "Codex") {
			return errors.New("choose a profile-specific name, such as Personal or Codex (Work)")
		}
		key := strings.ToLower(launcherName(p.Name))
		if seen[key] {
			return errors.New("another profile already uses that launcher name")
		}
		seen[key] = true
	}
	return nil
}
func bundleValues(path string) (map[string]string, error) {
	b, e := os.Open(filepath.Join(path, "Contents", "Info.plist"))
	if e != nil {
		return nil, e
	}
	defer b.Close()
	decoder := xml.NewDecoder(b)
	values := map[string]string{}
	key := ""
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "key" {
			if e = decoder.DecodeElement(&key, &start); e != nil {
				return nil, e
			}
		}
		if start.Name.Local == "string" {
			var value string
			if e = decoder.DecodeElement(&value, &start); e != nil {
				return nil, e
			}
			if key != "" {
				values[key] = value
				key = ""
			}
		}
	}
	return values, nil
}
func ownsLauncher(path, root, id string) bool {
	v, e := bundleValues(path)
	return e == nil && v["CFBundleIdentifier"] == helperID+".profile"+id && filepath.Clean(v["MultiCodexRoot"]) == filepath.Clean(root)
}
func legacyLauncher(path string) bool {
	v, e := bundleValues(path)
	return e == nil && (v["CFBundleIdentifier"] == "com.edihasaj.codex-primary-launcher" || v["CFBundleIdentifier"] == "com.edihasaj.codex-secondary-launcher")
}
func managedLaunchers(root, home, id, recorded string) []string {
	var paths []string
	for _, dir := range []string{filepath.Join(home, "Applications"), filepath.Join(home, "Applications", "Multi Codex Profiles")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".app") {
				p := filepath.Join(dir, entry.Name())
				if ownsLauncher(p, root, id) {
					paths = append(paths, p)
				}
			}
		}
	}
	if recorded != "" && ownsLauncher(recorded, root, id) {
		found := false
		for _, p := range paths {
			if p == recorded {
				found = true
			}
		}
		if !found {
			paths = append(paths, recorded)
		}
	}
	return paths
}

const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

func retireLauncher(path, root, kind string) (string, error) {
	dir, e := os.MkdirTemp(filepath.Join(root, "retired-launchers"), kind+"-")
	if e != nil {
		return "", e
	}
	target := filepath.Join(dir, filepath.Base(path))
	_ = exec.Command(lsregister, "-u", path).Run()
	if e = os.Rename(path, target); e != nil {
		_ = os.Remove(dir)
		return "", e
	}
	return target, nil
}
func installMacLaunchers(root string, c *Config, home, helper string, pin bool) error {
	if e := validateNames(c.Profiles); e != nil {
		return e
	}
	apps := filepath.Join(home, "Applications")
	if e := os.MkdirAll(apps, 0755); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Join(root, "retired-launchers"), 0700); e != nil {
		return e
	}
	// Check every destination before changing an existing launcher or its Dock pin.
	for _, p := range c.Profiles {
		target := filepath.Join(apps, launcherName(p.Name)+".app")
		if _, e := os.Lstat(target); e == nil && !ownsLauncher(target, root, p.ID) && !legacyLauncher(target) {
			return fmt.Errorf("%s already exists and belongs to another app; choose a different name", target)
		}
	}
	stage, e := os.MkdirTemp(apps, ".multi-codex-stage-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	staged := map[string]string{}
	for _, p := range c.Profiles {
		path := filepath.Join(stage, p.ID+".app")
		if e = extractAsset("macos/MultiCodexHelper", filepath.Join(path, "Contents", "MacOS", "MultiCodexHelper"), 0755); e != nil {
			return e
		}
		if e = atomicWrite(filepath.Join(path, "Contents", "Info.plist"), []byte(helperPlist(*c, root, p.ID)), 0644); e != nil {
			return e
		}
		icon := filepath.Join(path, "Contents", "Resources", "ProfileIcon.icns")
		if e = os.MkdirAll(filepath.Dir(icon), 0755); e != nil {
			return e
		}
		if e = runQuiet(helper, "--icon", iconFile(root, p, "png"), icon); e != nil {
			return e
		}
		if e = runQuiet("/usr/bin/xattr", "-cr", path); e != nil {
			return e
		}
		if e = runQuiet("/usr/bin/codesign", "--force", "--sign", "-", path); e != nil {
			return e
		}
		staged[p.ID] = path
	}
	var destinations, relocations []string
	for i, p := range c.Profiles {
		target := filepath.Join(apps, launcherName(p.Name)+".app")
		// Repair stale Dock bookmarks even when earlier numbered wrappers were retired.
		numbered := filepath.Join(apps, "Multi Codex Profiles", "Codex Profile "+p.ID+".app")
		if numbered != target {
			relocations = append(relocations, numbered, target)
		}
		oldPaths := managedLaunchers(root, home, p.ID, p.LauncherPath)
		if legacyLauncher(target) {
			backup, e := retireLauncher(target, root, "edi")
			if e != nil {
				return e
			}
			c.LegacyLaunchers = append(c.LegacyLaunchers, LauncherBackup{Original: target, Backup: backup})
			// Record recoverable Edi backups immediately, even if later registration fails.
			if e = saveConfig(root, *c); e != nil {
				return e
			}
		}
		for _, old := range oldPaths {
			if _, e := retireLauncher(old, root, "profile-"+p.ID); e != nil {
				return e
			}
			if old != target {
				relocations = append(relocations, old, target)
			}
		}
		if e = os.Rename(staged[p.ID], target); e != nil {
			return e
		}
		if e = runQuiet(lsregister, "-f", target); e != nil {
			return e
		}
		_ = exec.Command("/usr/bin/mdimport", target).Run()
		c.Profiles[i].LauncherPath = target
		destinations = append(destinations, target)
	}
	if len(relocations) > 0 {
		if e = runQuiet(helper, append([]string{"--relocate-pins"}, relocations...)...); e != nil {
			return e
		}
	}
	if pin {
		if e = runQuiet(helper, append([]string{"--pin"}, destinations...)...); e != nil {
			return e
		}
	}
	return saveConfig(root, *c)
}
func renameProfile(id, name string) error {
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	p, e := profile(c, id)
	if e != nil {
		return e
	}
	for i := range c.Profiles {
		if c.Profiles[i].ID == p.ID {
			c.Profiles[i].Name = strings.TrimSpace(name)
		}
	}
	if e = validateConfig(c); e != nil {
		return e
	}
	if e = applyAppearance(root, &c); e != nil {
		return e
	}
	fmt.Printf("Profile %s is now %s. Launcher: %s\n", p.ID, strings.TrimSpace(name), launcherName(name))
	return saveConfig(root, c)
}

func applyAppearance(root string, c *Config) error {
	if e := validateConfig(*c); e != nil {
		return e
	}
	if e := installIcons(root); e != nil {
		return e
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		if e := installMacLaunchers(root, c, home, helperPath(root), false); e != nil {
			return e
		}
		return runQuiet(helperPath(root), "--refresh-icons")
	}
	previous, e := readConfig(root)
	if e != nil {
		return e
	}
	if e = saveConfig(root, *c); e != nil {
		return e
	}
	if e = installIntegration(root, c, false); e != nil {
		_ = saveConfig(root, previous)
		return e
	}
	return saveConfig(root, *c)
}

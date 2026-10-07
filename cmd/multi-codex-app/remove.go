package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func removeProfile(id string) error {
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	return removeProfileAt(root, c, id, "")
}
func removeProfileAt(root string, c Config, id, home string) error {
	expected, e := profile(c, id)
	if e != nil {
		return e
	}
	latest, e := readConfig(root)
	if e != nil {
		return e
	}
	current, e := profile(latest, expected.ID)
	if e != nil {
		return e
	}
	if current.Name != expected.Name || current.CodexHome != expected.CodexHome || current.UserDataDir != expected.UserDataDir {
		return errors.New("this profile changed while the wizard was open; select it again")
	}
	c = latest
	next, p, e := archivedConfig(c, id)
	if e != nil {
		return e
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if len(next.Profiles) == 0 {
		// Uninstall reads the current manifest to remove the final owned launcher and restore routing.
		if e = uninstallAt(root, c, home); e != nil {
			return e
		}
	} else {
		switch runtime.GOOS {
		case "darwin":
			paths := managedLaunchers(root, home, p.ID, p.LauncherPath)
			if len(paths) > 0 {
				if e = runQuiet(helperPath(root), append([]string{"--unpin"}, paths...)...); e != nil {
					return e
				}
				if e = os.MkdirAll(filepath.Join(root, "retired-launchers"), 0700); e != nil {
					return e
				}
				for _, path := range paths {
					if _, e = retireLauncher(path, root, "removed-"+p.ID); e != nil {
						return e
					}
				}
			}
		case "linux":
			data := os.Getenv("XDG_DATA_HOME")
			if data == "" {
				data = filepath.Join(home, ".local/share")
			}
			path := filepath.Join(data, "applications", "multi-codex-profile-"+p.ID+".desktop")
			if _, err := os.Lstat(path); err == nil {
				expected := desktopQuote(c.CLIPath) + " --root " + desktopQuote(root) + " launch " + p.ID
				if desktopValues(path)["Exec"] != expected {
					return errors.New("launcher belongs to another installation; it was not removed")
				}
				retired := filepath.Join(root, "retired-launchers")
				if e = os.MkdirAll(retired, 0700); e != nil {
					return e
				}
				dir, e := os.MkdirTemp(retired, "removed-"+p.ID+"-")
				if e != nil {
					return e
				}
				if e = os.Rename(path, filepath.Join(dir, filepath.Base(path))); e != nil {
					return e
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		case "windows":
			if e = extractAsset("windows/integrate.ps1", filepath.Join(root, "integrate.ps1"), 0600); e != nil {
				return e
			}
			cmd := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join(root, "integrate.ps1"), "-ConfigPath", filepath.Join(root, "config.json"), "-RemoveProfileID", p.ID)
			if e = cmd.Run(); e != nil {
				return errors.New("could not remove the selected Windows shortcut")
			}
		default:
			return errors.New("unsupported OS")
		}
	}
	if e = saveConfig(root, next); e != nil {
		return e
	}
	fmt.Printf("Removed #%s (%s). Saved profile data retained at %s", p.ID, p.Name, p.CodexHome)
	if p.UserDataDir != "" {
		fmt.Printf(" and %s", p.UserDataDir)
	}
	fmt.Println(".")
	return nil
}
func primaryProfile(c Config) Profile {
	for _, p := range append(append([]Profile(nil), c.Profiles...), c.RemovedProfiles...) {
		if p.ID == "1" {
			return p
		}
	}
	home, _ := os.UserHomeDir()
	return Profile{ID: "1", Name: "Primary", CodexHome: filepath.Join(home, ".codex")}
}

// Restore an archived entry without copying or deleting its private data.
func restoreProfile(id string) error {
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	for i, p := range c.RemovedProfiles {
		if p.ID == id || strings.EqualFold(p.Name, id) {
			c.RemovedProfiles = append(c.RemovedProfiles[:i], c.RemovedProfiles[i+1:]...)
			c.Profiles = append(c.Profiles, p)
			if e = validateConfig(c); e != nil {
				return e
			}
			return persistInstallation(root, &c, false, p.ID)
		}
	}
	return fmt.Errorf("removed profile %q not found", id)
}

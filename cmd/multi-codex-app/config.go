package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type Profile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CodexHome    string `json:"codexHome"`
	UserDataDir  string `json:"userDataDir"`
	LauncherPath string `json:"launcherPath,omitempty"`
	IconColor    string `json:"iconColor,omitempty"`
}
type Config struct {
	Version         int              `json:"version"`
	AppPath         string           `json:"appPath"`
	CLIPath         string           `json:"cliPath"`
	PreviousHandler string           `json:"previousHandler,omitempty"`
	Profiles        []Profile        `json:"profiles"`
	LegacyLaunchers []LauncherBackup `json:"legacyLaunchers,omitempty"`
}

func stateRoot() string {
	if p := os.Getenv("MULTI_CODEX_ROOT"); p != "" {
		a, _ := filepath.Abs(p)
		return a
	}
	h, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(h, "Library", "Application Support", "Multi Codex")
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "MultiCodex")
	default:
		if p := os.Getenv("XDG_DATA_HOME"); p != "" {
			return filepath.Join(p, "multi-codex-app")
		}
		return filepath.Join(h, ".local", "share", "multi-codex-app")
	}
}
func readConfig(root string) (Config, error) {
	var c Config
	b, e := os.ReadFile(filepath.Join(root, "config.json"))
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	if e != nil {
		return c, e
	}
	return c, validateConfig(c)
}
func validateConfig(c Config) error {
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	if !filepath.IsAbs(c.AppPath) || !filepath.IsAbs(c.CLIPath) {
		return errors.New("app and CLI paths must be absolute")
	}
	seen := map[string]bool{}
	for _, p := range c.Profiles {
		n, e := strconv.Atoi(p.ID)
		if e != nil || n < 1 || strconv.Itoa(n) != p.ID || seen[p.ID] {
			return errors.New("invalid or duplicate profile ID")
		}
		seen[p.ID] = true
		if p.IconColor != "" && !validIconColor(p.IconColor) {
			return errors.New("invalid icon color; run multi-codex-app icons")
		}
		if strings.TrimSpace(p.Name) == "" || strings.ContainsAny(p.Name, "\r\n\x00") {
			return errors.New("profile names must be nonempty single lines")
		}
		if !filepath.IsAbs(p.CodexHome) || (p.ID != "1" && !filepath.IsAbs(p.UserDataDir)) {
			return errors.New("profile paths must be absolute")
		}
	}
	if len(c.Profiles) == 0 {
		return errors.New("no profiles configured")
	}
	return validateNames(c.Profiles)
}
func saveConfig(root string, c Config) error {
	if e := validateConfig(c); e != nil {
		return e
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return atomicWrite(filepath.Join(root, "config.json"), append(b, '\n'), 0600)
}
func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".multi-codex-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	// Windows cannot rename over an existing file. Move it aside and recover on failure.
	if runtime.GOOS == "windows" {
		backup := path + ".previous"
		if _, e := os.Stat(path); e == nil {
			if e = os.Rename(path, backup); e != nil {
				return e
			}
			if e = os.Rename(tmp, path); e != nil {
				_ = os.Rename(backup, path)
				return e
			}
			_ = os.Remove(backup)
			return nil
		}
	}
	return os.Rename(tmp, path)
}
func ensureProfiles(c *Config, count int, root, home string) error {
	if count < 1 || count > 100 {
		return errors.New("choose between 1 and 100 profiles")
	}
	if count < len(c.Profiles) {
		return fmt.Errorf("%d profiles already exist; setup never removes profiles", len(c.Profiles))
	}
	next := 1
	for _, p := range c.Profiles {
		n, _ := strconv.Atoi(p.ID)
		if n >= next {
			next = n + 1
		}
	}
	for len(c.Profiles) < count {
		id := strconv.Itoa(next)
		p := Profile{ID: id, Name: "Profile " + id, CodexHome: filepath.Join(root, "profiles", id, "codex"), UserDataDir: filepath.Join(root, "profiles", id, "desktop")}
		if next == 1 {
			p.Name = "Primary"
			p.CodexHome = filepath.Join(home, ".codex")
			p.UserDataDir = ""
		}
		if next == 2 {
			p.Name = "Secondary"
			legacyHome := filepath.Join(home, ".codex-work")
			legacyData := filepath.Join(home, "Library", "Application Support", "Codex Second")
			if runtime.GOOS == "darwin" && isDir(legacyHome) && isDir(legacyData) {
				p.CodexHome = legacyHome
				p.UserDataDir = legacyData
			}
		}
		c.Profiles = append(c.Profiles, p)
		next++
	}
	return nil
}
func isDir(p string) bool { s, e := os.Stat(p); return e == nil && s.IsDir() }
func profile(c Config, id string) (Profile, error) {
	for _, p := range c.Profiles {
		if p.ID == id || strings.EqualFold(p.Name, id) {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("profile %q not found; run multi-codex-app list", id)
}

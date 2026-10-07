package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const helperID = "io.github.am-will.multi-codex-app"

func extractAsset(asset, path string, mode os.FileMode) error {
	b, e := assets.ReadFile("assets/" + asset)
	if e != nil {
		return e
	}
	return atomicWrite(path, b, mode)
}
func installIntegration(root string, c *Config, dock bool, onlyIDs ...string) error {
	home, _ := os.UserHomeDir()
	if e := installIcons(root); e != nil {
		return e
	}
	switch runtime.GOOS {
	case "darwin":
		return installMac(root, c, dock, home, onlyIDs...)
	case "linux":
		if e := extractAsset("linux/chooser.py", filepath.Join(root, "chooser.py"), 0600); e != nil {
			return e
		}
		data := os.Getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		dir := filepath.Join(data, "applications")
		for _, p := range c.Profiles {
			if !includesProfile(onlyIDs, p.ID) {
				continue
			}
			target := filepath.Join(dir, "multi-codex-profile-"+p.ID+".desktop")
			if _, err := os.Lstat(target); err == nil {
				expected := desktopQuote(c.CLIPath) + " --root " + desktopQuote(root) + " launch " + p.ID
				if desktopValues(target)["Exec"] != expected {
					return errors.New("launcher belongs to another installation; choose an unused profile number")
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			body := "[Desktop Entry]\nType=Application\nName=" + launcherName(p.Name) + "\nComment=Independent Codex profile\nExec=" + desktopQuote(c.CLIPath) + " --root " + desktopQuote(root) + " launch " + p.ID + "\nIcon=" + iconFile(root, p, "png") + "\nTerminal=false\nCategories=Development;\n"
			if e := atomicWrite(filepath.Join(dir, "multi-codex-profile-"+p.ID+".desktop"), []byte(body), 0755); e != nil {
				return e
			}
		}
		previous, _ := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/codex").Output()
		if c.PreviousHandler == "" {
			c.PreviousHandler = strings.TrimSpace(string(previous))
		}
		body := "[Desktop Entry]\nType=Application\nName=Multi Codex Helper\nExec=" + desktopQuote(c.CLIPath) + " --root " + desktopQuote(root) + " callback %u\nNoDisplay=true\nTerminal=false\nMimeType=x-scheme-handler/codex;\n"
		if e := atomicWrite(filepath.Join(dir, "multi-codex-helper.desktop"), []byte(body), 0755); e != nil {
			return e
		}
		if e := runQuiet("xdg-mime", "default", "multi-codex-helper.desktop", "x-scheme-handler/codex"); e != nil {
			return e
		}
		if _, e := exec.LookPath("update-desktop-database"); e == nil {
			_ = runQuiet("update-desktop-database", dir)
		}
		fmt.Println("Linux adapter is experimental; callback popup requires python3-tk. Pin launchers from your app menu.")
	case "windows":
		if e := extractAsset("windows/chooser.ps1", filepath.Join(root, "chooser.ps1"), 0600); e != nil {
			return e
		}
		if e := extractAsset("windows/integrate.ps1", filepath.Join(root, "integrate.ps1"), 0600); e != nil {
			return e
		}
		subset := *c
		subset.Profiles = nil
		for _, p := range c.Profiles {
			if includesProfile(onlyIDs, p.ID) {
				subset.Profiles = append(subset.Profiles, p)
			}
		}
		adapterConfig, e := writeIconAdapterConfig(root, subset)
		if e != nil {
			return e
		}
		defer os.Remove(adapterConfig)
		cmd := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join(root, "integrate.ps1"), "-ConfigPath", adapterConfig)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e := cmd.Run(); e != nil {
			return errors.New("Windows integration failed")
		}
		fmt.Println("Windows adapter is experimental. Start-menu shortcuts created; pin them manually to the taskbar.")
	default:
		return errors.New("unsupported OS")
	}
	if runtime.GOOS != "windows" {
		if e := installCLILink(home, c.CLIPath); e != nil {
			return e
		}
	}
	return nil
}
func installCLILink(home, cli string) error {
	dir := filepath.Join(home, ".local", "bin")
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	link := filepath.Join(dir, "multi-codex-app")
	if target, e := os.Readlink(link); e == nil {
		if target == cli {
			return nil
		}
		return errors.New("~/.local/bin/multi-codex-app points elsewhere; refusing to replace it")
	}
	if _, e := os.Lstat(link); e == nil {
		return errors.New("~/.local/bin/multi-codex-app exists; refusing to replace it")
	}
	return os.Symlink(cli, link)
}
func helperPlist(c Config, root, profileID string) string {
	role := "Multi Codex Helper"
	id := helperID
	extra := "<key>CFBundleURLTypes</key><array><dict><key>CFBundleURLName</key><string>Codex callback chooser</string><key>CFBundleURLSchemes</key><array><string>codex</string></array></dict></array>"
	if profileID != "" {
		if p, e := profile(c, profileID); e == nil {
			role = launcherName(p.Name)
		} else {
			role = "Codex Profile " + profileID
		}
		id += ".profile" + profileID
		extra = "<key>MultiCodexProfileID</key><string>" + profileID + "</string>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + id + `</string><key>CFBundleExecutable</key><string>MultiCodexHelper</string><key>CFBundleName</key><string>` + plistEscape(role) + `</string><key>CFBundleDisplayName</key><string>` + plistEscape(role) + `</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>0.2.0</string><key>CFBundleVersion</key><string>5</string><key>LSUIElement</key><true/><key>CFBundleIconFile</key><string>ProfileIcon.icns</string><key>NSAppleEventsUsageDescription</key><string>Route a connection to the Codex profile you choose.</string><key>MultiCodexRoot</key><string>` + plistEscape(root) + `</string><key>MultiCodexCLI</key><string>` + plistEscape(c.CLIPath) + `</string>` + extra + `</dict></plist>`
}
func installMac(root string, c *Config, dock bool, home string, onlyIDs ...string) error {
	// Repair private lock permissions from pre-release builds before restarting.
	lockPath := filepath.Join(root, "helper.lock")
	if info, err := os.Lstat(lockPath); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("helper lock is not a regular file")
		}
		if err := os.Chmod(lockPath, 0600); err != nil {
			return err
		}
	}
	bundle := filepath.Join(root, "Multi Codex Helper.app")
	helper := helperPath(root)
	// The release embeds a compiled helper, so users don't need Swift/Xcode.
	if e := extractAsset("macos/MultiCodexHelper", helper, 0755); e != nil {
		return fmt.Errorf("this binary has no macOS helper; use a published release or scripts/build.sh: %w", e)
	}
	if e := atomicWrite(filepath.Join(bundle, "Contents", "Info.plist"), []byte(helperPlist(*c, root, "")), 0644); e != nil {
		return e
	}
	command := exec.Command(helper, "--handler")
	command.Env = append(os.Environ(), "MULTI_CODEX_ROOT="+root)
	previous, e := command.Output()
	if e != nil {
		return errors.New("cannot read macOS protocol handler")
	}
	if c.PreviousHandler == "" {
		s := strings.TrimSpace(string(previous))
		if s == "local.codex.callback-router" || s == helperID || s == "" {
			s = "com.openai.codex"
		}
		c.PreviousHandler = s
	}
	if e = saveConfig(root, *c); e != nil {
		return e
	}
	if e = runQuiet("/usr/bin/xattr", "-cr", bundle); e != nil {
		return e
	}
	if e = runQuiet("/usr/bin/codesign", "--force", "--sign", "-", bundle); e != nil {
		return e
	}
	if e = runQuiet("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-f", bundle); e != nil {
		return e
	}
	agent := filepath.Join(home, "Library", "LaunchAgents", helperID+".plist")
	label := "gui/" + strconv.Itoa(os.Getuid()) + "/" + helperID
	// Only our helper is stopped for updates. Existing Codex sessions remain alive.
	_ = exec.Command("launchctl", "bootout", label).Run()
	body := `<?xml version="1.0"?><plist version="1.0"><dict><key>Label</key><string>` + helperID + `</string><key>ProgramArguments</key><array><string>` + plistEscape(helper) + `</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>ProcessType</key><string>Interactive</string></dict></plist>`
	if e = atomicWrite(agent, []byte(body), 0644); e != nil {
		return e
	}
	// Retire the old two-profile callback router after the new helper has built successfully.
	legacy := filepath.Join(home, "Library", "LaunchAgents", "local.codex.callback-router.plist")
	if _, e = os.Stat(legacy); e == nil {
		_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid())+"/local.codex.callback-router").Run()
		if e = os.Rename(legacy, legacy+".disabled-by-multi-codex"); e != nil {
			return e
		}
	}
	// launchd can briefly reject bootstrap while an earlier bootout finishes.
	for attempt := 0; attempt < 30; attempt++ {
		e = runQuiet("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), agent)
		if e == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if e != nil {
		return e
	}
	// Wait for registration and a live supervised helper before reporting success.
	ready := false
	for attempt := 0; attempt < 60; attempt++ {
		job, jobErr := exec.Command("launchctl", "print", label).Output()
		owner, ownerErr := exec.Command(helper, "--handler").Output()
		if jobErr == nil && ownerErr == nil && strings.Contains(string(job), "\n\tstate = running\n") && strings.TrimSpace(string(owner)) == helperID {
			ready = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		return errors.New("helper did not become ready; run doctor, then retry setup")
	}
	if e = installMacLaunchers(root, c, home, helper, dock, onlyIDs...); e != nil {
		return e
	}
	if e = runQuiet(helper, "--refresh-icons"); e != nil {
		return e
	}
	return installCLILink(home, c.CLIPath)
}
func doctor() error {
	c, e := readConfig(stateRoot())
	if e != nil {
		return e
	}
	fmt.Printf("multi-codex-app %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
	if e = checkApp(c.AppPath); e != nil {
		return e
	}
	fmt.Println("Desktop app found:", c.AppPath)
	fmt.Printf("Profiles: %d\n", len(c.Profiles))
	for _, p := range c.Profiles {
		if !isDir(p.CodexHome) {
			return fmt.Errorf("profile %s home missing", p.ID)
		}
		if p.UserDataDir != "" && !isDir(p.UserDataDir) {
			return fmt.Errorf("profile %s desktop data missing", p.ID)
		}
	}
	if runtime.GOOS == "darwin" {
		cmd := exec.Command(helperPath(stateRoot()), "--status")
		cmd.Env = append(os.Environ(), "MULTI_CODEX_ROOT="+stateRoot())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	fmt.Println("Profile paths OK. Desktop callback adapters are experimental on this OS.")
	return nil
}
func uninstall() error {
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	home, _ := os.UserHomeDir()
	return uninstallAt(root, c, home)
}
func uninstallAt(root string, c Config, home string) error {
	var e error
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("launchctl", "bootout", "gui/"+strconv.Itoa(os.Getuid())+"/"+helperID).Run()
		cmd := exec.Command(helperPath(root), "--restore")
		cmd.Env = append(os.Environ(), "MULTI_CODEX_ROOT="+root)
		if e = cmd.Run(); e != nil {
			return errors.New("could not restore protocol handler")
		}
		_ = os.Remove(filepath.Join(home, "Library", "LaunchAgents", helperID+".plist"))
		var paths []string
		for _, p := range c.Profiles {
			paths = append(paths, managedLaunchers(root, home, p.ID, p.LauncherPath)...)
		}
		if e = runQuiet(helperPath(root), append([]string{"--unpin"}, paths...)...); e != nil {
			return e
		}
		for _, p := range paths {
			_ = exec.Command(lsregister, "-u", p).Run()
			if e = os.RemoveAll(p); e != nil {
				return e
			}
		}
		for _, backup := range c.LegacyLaunchers {
			if _, err := os.Lstat(backup.Original); errors.Is(err, os.ErrNotExist) && legacyLauncher(backup.Backup) {
				if err = os.Rename(backup.Backup, backup.Original); err != nil {
					return err
				}
				_ = exec.Command(lsregister, "-f", backup.Original).Run()
			}
		}
	case "linux":
		if c.PreviousHandler != "" {
			if e = runQuiet("xdg-mime", "default", c.PreviousHandler, "x-scheme-handler/codex"); e != nil {
				return e
			}
		}
		data := os.Getenv("XDG_DATA_HOME")
		if data == "" {
			data = filepath.Join(home, ".local", "share")
		}
		dir := filepath.Join(data, "applications")
		helperEntry := filepath.Join(dir, "multi-codex-helper.desktop")
		if desktopValues(helperEntry)["Exec"] == desktopQuote(c.CLIPath)+" --root "+desktopQuote(root)+" callback %u" {
			_ = os.Remove(helperEntry)
		}
		for _, p := range c.Profiles {
			entry := filepath.Join(dir, "multi-codex-profile-"+p.ID+".desktop")
			if desktopValues(entry)["Exec"] == desktopQuote(c.CLIPath)+" --root "+desktopQuote(root)+" launch "+p.ID {
				_ = os.Remove(entry)
			}
		}
	case "windows":
		cmd := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join(root, "integrate.ps1"), "-ConfigPath", filepath.Join(root, "config.json"), "-Uninstall")
		if e = cmd.Run(); e != nil {
			return errors.New("could not restore Windows integration")
		}
	}
	// Keep the manifest and CLI, so profiles can be reinstalled without losing their IDs.
	fmt.Println("Helper integration and managed launchers removed. Profile data and CLI retained; setup reinstalls them.")
	return nil
}

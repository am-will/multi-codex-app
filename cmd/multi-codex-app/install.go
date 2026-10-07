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
)

const helperID = "io.github.am-will.multi-codex-app"

func extractAsset(asset, path string, mode os.FileMode) error {
	b, e := assets.ReadFile("assets/" + asset)
	if e != nil {
		return e
	}
	return atomicWrite(path, b, mode)
}
func installIntegration(root string, c *Config, dock bool) error {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return installMac(root, c, dock, home)
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
			body := "[Desktop Entry]\nType=Application\nName=Codex — " + p.Name + "\nComment=Independent Codex profile\nExec=" + desktopQuote(c.CLIPath) + " --root " + desktopQuote(root) + " launch " + p.ID + "\nIcon=utilities-terminal\nTerminal=false\nCategories=Development;\n"
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
		cmd := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join(root, "integrate.ps1"), "-ConfigPath", filepath.Join(root, "config.json"))
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
		role = "Codex Profile " + profileID
		id += ".profile" + profileID
		extra = "<key>MultiCodexProfileID</key><string>" + profileID + "</string>"
	}
	return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>` + id + `</string><key>CFBundleExecutable</key><string>MultiCodexHelper</string><key>CFBundleName</key><string>` + role + `</string><key>CFBundlePackageType</key><string>APPL</string><key>CFBundleShortVersionString</key><string>0.1.0</string><key>CFBundleVersion</key><string>1</string><key>LSUIElement</key><true/><key>CFBundleIconFile</key><string>ProfileIcon.icns</string><key>NSAppleEventsUsageDescription</key><string>Route a connection to the Codex profile you choose.</string><key>MultiCodexRoot</key><string>` + plistEscape(root) + `</string><key>MultiCodexCLI</key><string>` + plistEscape(c.CLIPath) + `</string>` + extra + `</dict></plist>`
}
func installMac(root string, c *Config, dock bool, home string) error {
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
	launchers := filepath.Join(home, "Applications", "Multi Codex Profiles")
	var paths []string
	for _, p := range c.Profiles {
		path := filepath.Join(launchers, "Codex Profile "+p.ID+".app")
		bin := filepath.Join(path, "Contents", "MacOS", "MultiCodexHelper")
		if e = extractAsset("macos/MultiCodexHelper", bin, 0755); e != nil {
			return e
		}
		if e = atomicWrite(filepath.Join(path, "Contents", "Info.plist"), []byte(helperPlist(*c, root, p.ID)), 0644); e != nil {
			return e
		}
		icons := filepath.Join(path, "Contents", "Resources", "ProfileIcon.icns")
		if e = os.MkdirAll(filepath.Dir(icons), 0755); e != nil {
			return e
		}
		if e = runQuiet(helper, "--icon", p.ID, icons); e != nil {
			return e
		}
		if e = runQuiet("/usr/bin/xattr", "-cr", path); e != nil {
			return e
		}
		if e = runQuiet("/usr/bin/codesign", "--force", "--sign", "-", path); e != nil {
			return e
		}
		paths = append(paths, path)
	}
	if e = runQuiet("/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister", "-f", bundle); e != nil {
		return e
	}
	agent := filepath.Join(home, "Library", "LaunchAgents", helperID+".plist")
	label := "gui/" + strconv.Itoa(os.Getuid()) + "/" + helperID
	// Only our helper is stopped for updates. Existing Codex sessions remain alive.
	_ = exec.Command("launchctl", "bootout", label).Run()
	body := `<?xml version="1.0"?><plist version="1.0"><dict><key>Label</key><string>` + helperID + `</string><key>ProgramArguments</key><array><string>` + plistEscape(helper) + `</string></array><key>RunAtLoad</key><true/><key>ProcessType</key><string>Interactive</string></dict></plist>`
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
	if e = runQuiet("launchctl", "bootstrap", "gui/"+strconv.Itoa(os.Getuid()), agent); e != nil {
		return e
	}
	if dock {
		if e = runQuiet(helper, append([]string{"--pin"}, paths...)...); e != nil {
			return e
		}
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
			paths = append(paths, filepath.Join(home, "Applications", "Multi Codex Profiles", "Codex Profile "+p.ID+".app"))
		}
		if e = runQuiet(helperPath(root), append([]string{"--unpin"}, paths...)...); e != nil {
			return e
		}
		for _, p := range paths {
			_ = os.RemoveAll(p)
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
		_ = os.Remove(filepath.Join(dir, "multi-codex-helper.desktop"))
		for _, p := range c.Profiles {
			_ = os.Remove(filepath.Join(dir, "multi-codex-profile-"+p.ID+".desktop"))
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

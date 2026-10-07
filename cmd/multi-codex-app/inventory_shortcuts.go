package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func inspectWindowsInstallation(root string, c Config, report *installationReport) {
	script, e := assets.ReadFile("assets/windows/inventory.ps1")
	if e != nil {
		report.Warnings = append(report.Warnings, "Windows shortcut inspection unavailable")
		return
	}
	file, e := os.CreateTemp("", "multi-codex-inventory-*.ps1")
	if e != nil {
		report.Warnings = append(report.Warnings, "Could not prepare Windows shortcut inspection")
		return
	}
	defer os.Remove(file.Name())
	if _, e = file.Write(script); e != nil {
		file.Close()
		report.Warnings = append(report.Warnings, "Could not prepare Windows shortcut inspection")
		return
	}
	file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-File", file.Name()).Output()
	if e != nil {
		report.Warnings = append(report.Warnings, "Windows shortcut inspection failed; saved preferences are still shown")
		return
	}
	var result struct {
		Helper    bool                                                   `json:"helper"`
		Shortcuts []struct{ Name, Path, Target, Arguments, Icon string } `json:"shortcuts"`
	}
	if json.Unmarshal(b, &result) != nil {
		report.Warnings = append(report.Warnings, "Could not read Windows shortcut inspection")
		return
	}
	if result.Helper {
		report.HelperStatus = "installed"
	}
	cli := c.CLIPath
	if cli == "" {
		cli = filepath.Join(root, "bin", "multi-codex-app.exe")
	}
	for _, shortcut := range result.Shortcuts {
		prefix := "--root \"" + root + "\" launch "
		if !strings.EqualFold(shortcut.Target, cli) || !strings.HasPrefix(shortcut.Arguments, prefix) {
			continue
		}
		icon := shortcut.Icon
		if index := strings.LastIndex(icon, ","); index >= 0 {
			icon = icon[:index]
		}
		icon = strings.Trim(icon, "\"")
		report.Launchers = append(report.Launchers, installedLauncher{ID: strings.TrimPrefix(shortcut.Arguments, prefix), Name: shortcut.Name, Path: shortcut.Path, Icon: inspectIcon(icon), Kind: "managed"})
	}
}

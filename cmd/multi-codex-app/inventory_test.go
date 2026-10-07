package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureLauncher(t *testing.T, root, home string, p Profile, actualColor string) string {
	t.Helper()
	app := filepath.Join(home, "Applications", launcherName(p.Name)+".app")
	if e := os.MkdirAll(filepath.Join(app, "Contents/Resources"), 0755); e != nil {
		t.Fatal(e)
	}
	c := Config{Profiles: []Profile{p}}
	if e := os.WriteFile(filepath.Join(app, "Contents/Info.plist"), []byte(helperPlist(c, root, p.ID)), 0644); e != nil {
		t.Fatal(e)
	}
	raw, e := assets.ReadFile("assets/icons/" + actualColor + ".png")
	if e != nil {
		t.Fatal(e)
	}
	// A valid ICNS PNG frame, matching what iconutil writes.
	data := make([]byte, 16)
	copy(data, "icns")
	binary.BigEndian.PutUint32(data[4:8], uint32(16+len(raw)))
	copy(data[8:], "ic10")
	binary.BigEndian.PutUint32(data[12:16], uint32(8+len(raw)))
	if e := os.WriteFile(filepath.Join(app, "Contents/Resources/ProfileIcon.icns"), append(data, raw...), 0644); e != nil {
		t.Fatal(e)
	}
	return app
}
func TestInstalledIconInspection(t *testing.T) {
	r := t.TempDir()
	for _, p := range iconPalettes() {
		for _, extension := range []string{"png", "ico"} {
			raw, e := assets.ReadFile("assets/icons/" + p.Name + "." + extension)
			if e != nil {
				t.Fatal(e)
			}
			path := filepath.Join(r, p.Name+"."+extension)
			os.WriteFile(path, raw, 0600)
			if got := inspectIcon(path); got != p.Name {
				t.Fatalf("%s: detected %s", path, got)
			}
		}
	}
	if inspectIcon(filepath.Join(r, "missing.icns")) != "missing" {
		t.Fatal("missing artwork mislabeled")
	}
	for i, raw := range [][]byte{[]byte("icns\x00\x00\x00\x08bad!\xff\xff\xff\xff"), []byte{0, 0, 1, 0, 255, 255}, []byte("custom artwork")} {
		path := filepath.Join(r, string(rune('a'+i)))
		os.WriteFile(path, raw, 0600)
		if inspectIcon(path) != "custom" {
			t.Fatal("invalid artwork mislabeled")
		}
	}
}
func TestReportReadsActualNamesAndIconsBeforeFirstQuestion(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	p := Profile{ID: "1", Name: "Saved Name", IconColor: "blue", CodexHome: root}
	actual := p
	actual.Name = "Actual Launcher"
	app := fixtureLauncher(t, root, home, actual, "yellow")
	p.LauncherPath = app
	c := Config{Profiles: []Profile{p}}
	report := inspectInstallation(root, c, home, "darwin", "")
	var out bytes.Buffer
	w := wizardPrompt{out: &out, reader: bufio.NewReader(strings.NewReader("\n"))}
	w.banner()
	w.report(report)
	w.ask("How many profiles?", "1")
	text := out.String()
	for _, want := range []string{"Actual Launcher", "YELLOW", "configured icon: blue", "configured name: Codex Saved Name", "1 profiles configured"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Index(text, "YOUR CURRENT INSTALLATION") > strings.Index(text, "How many profiles?") {
		t.Fatal("report appeared after first question")
	}
	if got := inspectIcon(filepath.Join(app, "Contents/Resources/ProfileIcon.icns")); got != "yellow" {
		t.Fatal("ICNS color not inspected", got)
	}
	// The scan and report must not change the installed launcher.
	v, e := bundleValues(app)
	if e != nil || v["CFBundleName"] != "Codex Actual Launcher" {
		t.Fatal("inspection mutated launcher")
	}
}
func TestReportMissingLaunchersAndOrphans(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	p := Profile{ID: "2", Name: "Work", IconColor: "teal", CodexHome: root, UserDataDir: root}
	orphan := Profile{ID: "9", Name: "Earlier Install"}
	fixtureLauncher(t, root, home, orphan, "purple")
	report := inspectInstallation(root, Config{Profiles: []Profile{p}}, home, "darwin", "")
	var out bytes.Buffer
	w := wizardPrompt{out: &out}
	w.report(report)
	text := out.String()
	for _, want := range []string{"Codex Work", "launcher missing", "TEAL", "Earlier Install", "not in this profile configuration"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}
func TestLinuxReportReadsInstalledDesktopEntry(t *testing.T) {
	root, home, data := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	dir := filepath.Join(data, "applications")
	os.MkdirAll(dir, 0755)
	cli := filepath.Join(root, "bin/cli")
	icon := filepath.Join(root, "purple.png")
	raw, _ := assets.ReadFile("assets/icons/purple.png")
	os.WriteFile(icon, raw, 0600)
	body := "[Desktop Entry]\nName=Codex Existing Work\nExec=" + desktopQuote(cli) + " --root " + desktopQuote(root) + " launch 2\nIcon=" + icon + "\n"
	os.WriteFile(filepath.Join(dir, "multi-codex-profile-2.desktop"), []byte(body), 0600)
	report := inspectInstallation(root, Config{CLIPath: cli}, home, "linux", "")
	if len(report.Launchers) != 1 || report.Launchers[0].Name != "Codex Existing Work" || report.Launchers[0].Icon != "purple" {
		t.Fatal(report)
	}
}
func TestTerminalOutputSuppressesColorsAndUnsafeControls(t *testing.T) {
	var out bytes.Buffer
	if styleFor(&out).paint("31", "hello") != "hello" {
		t.Fatal("redirected report contains escapes")
	}
	t.Setenv("NO_COLOR", "")
	if styleFor(os.Stdout).color {
		t.Fatal("NO_COLOR ignored")
	}
	if strings.Contains(terminalText("name\x1b[2J\n"), "\x1b") || strings.Contains(terminalText("name\n"), "\n") {
		t.Fatal("unsafe terminal control passed through")
	}
}

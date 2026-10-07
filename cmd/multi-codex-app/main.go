package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

//go:embed assets
var assets embed.FS
var version = "dev"

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, "multi-codex-app:", e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) >= 2 && args[0] == "--root" {
		if !filepath.IsAbs(args[1]) {
			return errors.New("root must be absolute")
		}
		os.Setenv("MULTI_CODEX_ROOT", args[1])
		args = args[2:]
	}
	if len(args) == 0 {
		return setup(nil, false)
	}
	switch args[0] {
	case "wizard":
		return setup(append([]string{"--wizard"}, args[1:]...), false)
	case "setup", "install":
		return setup(args[1:], false)
	case "add":
		return setup(args[1:], true)
	case "list":
		c, e := readConfig(stateRoot())
		if e != nil {
			return e
		}
		for _, p := range c.Profiles {
			fmt.Printf("%s  %-24s  %s\n", p.ID, p.Name, iconColor(p))
		}
		return nil
	case "launch":
		if len(args) != 2 {
			return errors.New("usage: multi-codex-app launch ID")
		}
		c, e := readConfig(stateRoot())
		if e != nil {
			return e
		}
		p, e := profile(c, args[1])
		if e != nil {
			return e
		}
		return launch(c, p, "")
	case "rename":
		if len(args) != 3 {
			return errors.New("usage: multi-codex-app rename ID NAME")
		}
		return renameProfile(args[1], args[2])
	case "icons":
		listIcons()
		return nil
	case "icon":
		if len(args) != 3 {
			return errors.New("usage: multi-codex-app icon ID COLOR; run icons to see colors")
		}
		return setIcon(args[1], args[2])
	case "callback":
		if len(args) != 2 {
			return errors.New("expected one callback URL")
		}
		return callback(args[1])
	case "update":
		return update()
	case "uninstall":
		return uninstall()
	case "doctor":
		return doctor()
	case "version", "--version":
		fmt.Println(version)
		return nil
	case "help", "--help", "-h":
		fmt.Print(help)
		return nil
	default:
		return fmt.Errorf("unknown command %q; use --help", args[0])
	}
}

const help = `multi-codex-app — independent Codex desktop profiles

  wizard                                    Guided count, names, colors, and launchers
  setup [--count N] [--app PATH] [--no-dock]  Install or refresh; wizard without N
  add [--count N]                            Add N more profiles (guided without N)
  list                                      Show profile IDs, names, and icon colors
  launch ID                                 Open or focus one profile
  rename ID NAME                            Rename picker and OS launcher
  icons                                     Show the five icon colors
  icon ID COLOR                             Change one profile icon
  update                                    Install the newest release, keeping profiles
  doctor                                    Check paths and callback routing (no tokens)
  uninstall                                 Remove integration; preserve profile data

macOS: native callback chooser and optional Dock launchers.
Windows/Linux: experimental launch and callback adapters; see README.
Inspired by Edi Hasaj: https://edihasaj.com/posts/two-codex-accounts-two-dock-icons-macos
`

func setup(args []string, add bool) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	n := fs.Int("count", 0, "total profiles (or profiles to add)")
	app := fs.String("app", "", "installed desktop app path")
	noDock := fs.Bool("no-dock", false, "do not pin macOS launchers")
	forceWizard := fs.Bool("wizard", false, "choose names and colors interactively")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected setup arguments")
	}
	root := stateRoot()
	c, e := readConfig(root)
	configErr := e
	fresh := errors.Is(configErr, os.ErrNotExist)
	if fresh {
		c.Version = 1
	}
	var wizard *wizardPrompt
	if *n == 0 || *forceWizard {
		wizard, e = openWizard()
		if e != nil {
			display := &wizardPrompt{out: os.Stdout}
			display.banner()
			reportCurrentInstallation(display, root, c, *app)
			return e
		}
		defer wizard.close()
		wizard.banner()
		reportCurrentInstallation(wizard, root, c, *app)
	}
	if configErr != nil && !fresh {
		return fmt.Errorf("cannot use saved configuration: %w", configErr)
	}
	if *n == 0 {
		wizard.section("02", "CHOOSE YOUR PROFILE COUNT")
		question := "How many Codex profiles in total?"
		def := 2
		if !fresh {
			def = len(c.Profiles)
		}
		if add {
			question = "How many additional Codex profiles?"
			def = 1
		}
		*n, e = wizard.count(question, def)
		if e != nil {
			return e
		}
	}
	count := *n
	if add {
		if fresh {
			return errors.New("run setup before adding profiles")
		}
		if count < 1 {
			return errors.New("add count must be positive")
		}
		count += len(c.Profiles)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	existingCount := len(c.Profiles)
	if e = ensureProfiles(&c, count, root, home); e != nil {
		return e
	}
	if wizard != nil {
		first := 0
		if add {
			first = existingCount
		}
		if e = wizard.appearance(&c, first); e != nil {
			return e
		}
		if runtime.GOOS == "darwin" && !*noDock {
			pin, err := wizard.yesNo("Add launcher icons to the Dock?", true)
			if err != nil {
				return err
			}
			*noDock = !pin
		}
	}
	if *app != "" {
		c.AppPath, e = filepath.Abs(*app)
		if e != nil {
			return e
		}
	}
	if c.AppPath == "" {
		c.AppPath, e = detectApp()
		if e != nil && wizard != nil {
			c.AppPath, e = wizard.ask("Path to the installed Codex desktop app", "")
			if e == nil {
				c.AppPath, e = filepath.Abs(c.AppPath)
			}
		}
		if e != nil {
			return e
		}
	}
	if e = checkApp(c.AppPath); e != nil {
		return e
	}
	if wizard != nil {
		apply, err := wizard.review(c, !*noDock)
		if err != nil {
			return err
		}
		if !apply {
			fmt.Fprintln(wizard.out, "Cancelled. Settings were not applied.")
			return nil
		}
	}
	cliName := "multi-codex-app"
	if runtime.GOOS == "windows" {
		cliName += ".exe"
	}
	c.CLIPath = filepath.Join(root, "bin", cliName)
	self, e := os.Executable()
	if e != nil {
		return e
	}
	if filepath.Clean(self) != filepath.Clean(c.CLIPath) {
		b, e := os.ReadFile(self)
		if e != nil {
			return e
		}
		if e = atomicWrite(c.CLIPath, b, 0755); e != nil {
			return e
		}
	}
	for _, p := range c.Profiles {
		if e = os.MkdirAll(p.CodexHome, 0700); e != nil {
			return e
		}
		if p.UserDataDir != "" {
			if e = os.MkdirAll(p.UserDataDir, 0700); e != nil {
				return e
			}
		}
	}
	// Save before wiring launchers: all helpers use this single manifest.
	if e = saveConfig(root, c); e != nil {
		return e
	}
	if e = installIntegration(root, &c, !*noDock); e != nil {
		return e
	}
	if e = saveConfig(root, c); e != nil {
		return e
	}
	fmt.Printf("Installed %d profiles. Your saved profile data is retained.\n", len(c.Profiles))
	fmt.Printf("CLI: %s\n", c.CLIPath)
	fmt.Println("Use: multi-codex-app list / launch 2 / add / update")
	return nil
}
func update() error {
	c, e := readConfig(stateRoot())
	if e != nil {
		return e
	}
	script := "install.sh"
	shell := "sh"
	if runtime.GOOS == "windows" {
		script = "install.ps1"
		shell = "powershell.exe"
	}
	// Fetch the installer as data, then execute the local file. No shell interpolation.
	b, e := download("https://raw.githubusercontent.com/am-will/multi-codex-app/main/" + script)
	if e != nil {
		return e
	}
	if runtime.GOOS == "windows" {
		// Let this executable exit before the new installer replaces it.
		installer := filepath.Join(stateRoot(), "update-installer.ps1")
		if e := atomicWrite(installer, b, 0600); e != nil {
			return e
		}
		cmd := exec.Command("powershell.exe", "-NoProfile", "-File", installer, "-Count", strconv.Itoa(len(c.Profiles)))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if e := cmd.Start(); e != nil {
			return e
		}
		return cmd.Process.Release()
	}
	f, e := os.CreateTemp("", "multi-codex-update-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	f.Close()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		newName := f.Name() + ".ps1"
		if e = os.Rename(f.Name(), newName); e != nil {
			return e
		}
		defer os.Remove(newName)
		cmd = exec.Command(shell, "-NoProfile", "-File", newName, "-Count", strconv.Itoa(len(c.Profiles)))
	} else {
		cmd = exec.Command(shell, f.Name(), "--count", strconv.Itoa(len(c.Profiles)))
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

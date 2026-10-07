package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func detectApp() (string, error) {
	home, _ := os.UserHomeDir()
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{"/Applications/ChatGPT.app", "/Applications/Codex.app", filepath.Join(home, "Applications", "ChatGPT.app"), filepath.Join(home, "Applications", "Codex.app")}
	case "windows":
		for _, base := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles")} {
			for _, rel := range []string{"Programs/OpenAI/ChatGPT/ChatGPT.exe", "Programs/Codex/Codex.exe", "Programs/ChatGPT/ChatGPT.exe", "ChatGPT/ChatGPT.exe", "Codex/Codex.exe"} {
				candidates = append(candidates, filepath.Join(base, filepath.FromSlash(rel)))
			}
		}
	default:
		for _, name := range []string{"codex-desktop", "chatgpt-desktop", "chatgpt"} {
			if p, e := exec.LookPath(name); e == nil {
				candidates = append(candidates, p)
			}
		}
		candidates = append(candidates, "/opt/codex/codex", "/opt/ChatGPT/chatgpt")
	}
	for _, p := range candidates {
		if checkApp(p) == nil {
			return p, nil
		}
	}
	return "", errors.New("desktop app not found; install it first, then run setup --app /absolute/path/to/app (Windows: path to desktop .exe)")
}
func checkApp(path string) error {
	s, e := os.Stat(path)
	if e != nil {
		return fmt.Errorf("desktop app not found: %s", path)
	}
	if runtime.GOOS == "darwin" {
		if !s.IsDir() || filepath.Ext(path) != ".app" {
			return errors.New("--app must point to the desktop .app bundle")
		}
		if !isDir(filepath.Join(path, "Contents", "MacOS")) {
			return errors.New("invalid app bundle")
		}
	} else if s.IsDir() {
		return errors.New("--app must point to the desktop executable")
	}
	return nil
}
func helperPath(root string) string {
	return filepath.Join(root, "Multi Codex Helper.app", "Contents", "MacOS", "MultiCodexHelper")
}
func appEnvironment(p Profile) []string {
	env := []string{}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		switch k {
		case "CODEX_HOME", "CODEX_ELECTRON_USER_DATA_PATH", "CODEX_DESKTOP_RELAUNCH_OPEN_EVENTS":
			continue
		}
		env = append(env, v)
	}
	return append(env, "CODEX_HOME="+p.CodexHome)
}
func launch(c Config, p Profile, uri string) error {
	if e := checkApp(c.AppPath); e != nil {
		return e
	}
	if uri != "" {
		if e := validateURI(uri); e != nil {
			return e
		}
	}
	if runtime.GOOS == "darwin" {
		if uri != "" {
			cmd := exec.Command(helperPath(stateRoot()), "--deliver", p.ID)
			cmd.Env = append(os.Environ(), "MULTI_CODEX_ROOT="+stateRoot())
			cmd.Stdin = strings.NewReader(uri)
			cmd.Stderr = os.Stderr
			return cmd.Run()
		}
		// Focus a known running profile; launch only if absent.
		focus := exec.Command(helperPath(stateRoot()), "--focus", p.ID)
		focus.Env = append(os.Environ(), "MULTI_CODEX_ROOT="+stateRoot())
		if focus.Run() == nil {
			return nil
		}
		args := []string{"-n", c.AppPath, "--env", "CODEX_HOME=" + p.CodexHome}
		if p.UserDataDir != "" {
			args = append(args, "--args", "--user-data-dir="+p.UserDataDir)
		}
		cmd := exec.Command("/usr/bin/open", args...)
		cmd.Env = appEnvironment(p)
		if cmd.Run() != nil {
			return errors.New("could not launch Codex")
		}
		return nil
	}
	args := []string{}
	if p.UserDataDir != "" {
		args = append(args, "--user-data-dir="+p.UserDataDir)
	}
	if uri != "" {
		args = append(args, uri)
	}
	cmd := exec.Command(c.AppPath, args...)
	cmd.Env = appEnvironment(p)
	// Callback arguments are never logged, even on launch failures.
	if e := cmd.Start(); e != nil {
		return errors.New("could not start the selected desktop profile")
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
func validateURI(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "codex" || u.User != nil || u.Host == "" || u.Port() != "" || len(raw) > 65536 || strings.ContainsAny(raw, "\r\n\x00") {
		return errors.New("invalid Codex callback URL")
	}
	return nil
}
func callback(raw string) error {
	if e := validateURI(raw); e != nil {
		return e
	}
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	u, _ := url.Parse(raw)
	if runtime.GOOS == "darwin" {
		return errors.New("macOS callbacks are received directly by Multi Codex Helper")
	}
	id := "1"
	if u.Host == "connector" && u.Path == "/oauth_callback" {
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("powershell.exe", "-NoProfile", "-STA", "-File", filepath.Join(root, "chooser.ps1"), "-ConfigPath", filepath.Join(root, "config.json"))
		} else {
			cmd = exec.Command("python3", filepath.Join(root, "chooser.py"), filepath.Join(root, "config.json"))
		}
		b, e := cmd.Output()
		if e != nil {
			return errors.New("callback chooser unavailable; Linux needs Python 3 and Tk (python3-tk)")
		}
		id = strings.TrimSpace(string(b))
		if id == "" {
			return nil
		}
	}
	p, e := profile(c, id)
	if e != nil {
		return errors.New("invalid callback destination")
	}
	return launch(c, p, raw)
}
func download(address string) ([]byte, error) {
	client := http.Client{Timeout: 60 * time.Second}
	r, e := client.Get(address)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("download returned HTTP %d", r.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 128<<20))
	return b, e
}
func runQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if e := cmd.Run(); e != nil {
		return fmt.Errorf("%s failed", filepath.Base(name))
	}
	return nil
}
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
func desktopQuote(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "`", "\\`", "$", "\\$", "%", "%%")
	return "\"" + r.Replace(s) + "\""
}
func plistEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(s)
}

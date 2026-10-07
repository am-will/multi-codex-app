package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type wizardPrompt struct {
	reader *bufio.Reader
	out    io.Writer
	close  func() error
}

func openWizard() (*wizardPrompt, error) {
	if runtime.GOOS == "windows" {
		return &wizardPrompt{reader: bufio.NewReader(os.Stdin), out: os.Stdout, close: func() error { return nil }}, nil
	}
	terminal, e := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if e != nil {
		return nil, errors.New("no interactive terminal; use setup --count N, then rename/icon commands")
	}
	return &wizardPrompt{reader: bufio.NewReader(terminal), out: terminal, close: terminal.Close}, nil
}
func (w *wizardPrompt) ask(question, def string) (string, error) {
	fmt.Fprintf(w.out, "%s [%s]: ", question, def)
	line, e := w.reader.ReadString('\n')
	if e != nil {
		return "", errors.New("wizard stopped before an answer was received; settings were not applied")
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = def
	}
	return line, nil
}
func (w *wizardPrompt) count(question string, def int) (int, error) {
	for {
		value, e := w.ask(question, strconv.Itoa(def))
		if e != nil {
			return 0, e
		}
		n, e := strconv.Atoi(value)
		if e == nil && n >= 1 && n <= 100 {
			return n, nil
		}
		fmt.Fprintln(w.out, "Choose a number from 1 to 100.")
	}
}
func (w *wizardPrompt) yesNo(question string, def bool) (bool, error) {
	initial := "yes"
	if !def {
		initial = "no"
	}
	for {
		value, e := w.ask(question, initial)
		if e != nil {
			return false, e
		}
		switch strings.ToLower(value) {
		case "yes", "y":
			return true, nil
		case "no", "n":
			return false, nil
		}
		fmt.Fprintln(w.out, "Enter yes or no.")
	}
}
func (w *wizardPrompt) appearance(c *Config, first int) error {
	fmt.Fprintln(w.out, "\nName and color each profile. Press Enter to keep the shown value.")
	fmt.Fprintln(w.out, "Icon colors: white, yellow, blue, purple, teal.")
	for i := first; i < len(c.Profiles); i++ {
		fmt.Fprintf(w.out, "\nProfile %s\n", c.Profiles[i].ID)
		for {
			name, e := w.ask("Name", c.Profiles[i].Name)
			if e != nil {
				return e
			}
			previous := c.Profiles[i].Name
			c.Profiles[i].Name = name
			if e = validateNames(c.Profiles); e == nil {
				break
			}
			c.Profiles[i].Name = previous
			fmt.Fprintln(w.out, e)
		}
		for {
			color, e := w.ask("Icon color", iconColor(c.Profiles[i]))
			if e != nil {
				return e
			}
			color = strings.ToLower(color)
			if validIconColor(color) {
				c.Profiles[i].IconColor = color
				break
			}
			fmt.Fprintln(w.out, "Choose white, yellow, blue, purple, or teal.")
		}
	}
	return nil
}
func (w *wizardPrompt) review(c Config, pin bool) (bool, error) {
	fmt.Fprintln(w.out, "\nYour Codex profiles:")
	for _, p := range c.Profiles {
		fmt.Fprintf(w.out, "  %s  %-28s %s\n", p.ID, launcherName(p.Name), iconColor(p))
	}
	if runtime.GOOS == "darwin" {
		fmt.Fprintf(w.out, "Add Dock pins: %t\n", pin)
	}
	fmt.Fprintln(w.out, "Existing profile sign-ins and data are retained.")
	return w.yesNo("Apply these settings?", true)
}

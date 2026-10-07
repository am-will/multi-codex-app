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
	s := styleFor(w.out)
	fmt.Fprintf(w.out, "\n     %s %s\n     %s ", s.paint("1", question), s.dim("["+terminalText(def)+"]"), s.accent("›"))
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
	w.section("03", "MAKE EACH PROFILE YOURS")
	w.palette()
	for i := first; i < len(c.Profiles); i++ {
		fmt.Fprintf(w.out, "\n     %s\n", styleFor(w.out).accent("PROFILE "+c.Profiles[i].ID))
		if e := w.editFields(c, i, true, true); e != nil {
			return e
		}
	}
	return nil
}
func (w *wizardPrompt) review(c Config, pin bool) (bool, error) {
	w.section("04", "READY TO APPLY")
	s := styleFor(w.out)
	for _, p := range c.Profiles {
		fmt.Fprintf(w.out, "     %s  %s  %s\n", s.dim("#"+p.ID), s.paint("1", launcherName(p.Name)), s.icon(iconColor(p)))
	}
	if runtime.GOOS == "darwin" {
		value := "no"
		if pin {
			value = "yes"
		}
		fmt.Fprintf(w.out, "\n     Dock pins: %s\n", s.accent(value))
	}
	fmt.Fprintln(w.out, s.dim("     Existing sign-ins and profile data are retained."))
	return w.yesNo("Apply these settings?", true)
}

func (w *wizardPrompt) editFields(c *Config, index int, name, color bool) error {
	if name {
		for {
			name, e := w.ask("Name", c.Profiles[index].Name)
			if e != nil {
				return e
			}
			previous := c.Profiles[index].Name
			c.Profiles[index].Name = name
			if e = validateNames(c.Profiles); e == nil {
				break
			}
			c.Profiles[index].Name = previous
			fmt.Fprintln(w.out, e)
		}
	}
	if color {

		for {
			color, e := w.ask("Icon color (name or 1–5)", iconColor(c.Profiles[index]))
			if e != nil {
				return e
			}
			color = strings.ToLower(color)
			if n, err := strconv.Atoi(color); err == nil && n >= 1 && n <= len(iconPalettes()) {
				color = iconPalettes()[n-1].Name
			}
			if validIconColor(color) {
				c.Profiles[index].IconColor = color
				break
			}
			fmt.Fprintln(w.out, "Choose white, yellow, blue, purple, or teal.")
		}
	}
	return nil
}

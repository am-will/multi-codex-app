package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

type terminalStyle struct{ color bool }

func styleFor(out io.Writer) terminalStyle {
	f, ok := out.(*os.File)
	if !ok || noColorSet() || os.Getenv("TERM") == "dumb" {
		return terminalStyle{}
	}
	s, e := f.Stat()
	return terminalStyle{color: e == nil && s.Mode()&os.ModeCharDevice != 0}
}
func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, s)
}
func (s terminalStyle) paint(code, text string) string {
	text = terminalText(text)
	if !s.color {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
func (s terminalStyle) accent(text string) string { return s.paint("1;38;2;117;218;210", text) }
func (s terminalStyle) dim(text string) string    { return s.paint("2", text) }
func (s terminalStyle) icon(color string) string {
	for _, p := range iconPalettes() {
		if p.Name == color {
			var r, g, b, fr, fg, fb int
			fmt.Sscanf(p.Background, "#%02x%02x%02x", &r, &g, &b)
			fmt.Sscanf(p.Foreground, "#%02x%02x%02x", &fr, &fg, &fb)
			return s.paint(fmt.Sprintf("48;2;%d;%d;%d;38;2;%d;%d;%d", r, g, b, fr, fg, fb), " "+strings.ToUpper(color)+" ")
		}
	}
	return s.dim(strings.ToUpper(color))
}
func (w *wizardPrompt) banner() {
	s := styleFor(w.out)
	fmt.Fprintln(w.out)
	fmt.Fprintln(w.out, s.accent("  ╭──────────────────────────────────────────────────────╮"))
	fmt.Fprintln(w.out, s.accent("  │  ✦  MULTI CODEX                                      │"))
	fmt.Fprintln(w.out, s.accent("  │")+"     Your accounts. Your names. Your colors.          "+s.accent("│"))
	fmt.Fprintln(w.out, s.accent("  ╰──────────────────────────────────────────────────────╯"))
	fmt.Fprintln(w.out, s.dim("     Setup wizard · press Enter to keep a shown value"))
}
func (w *wizardPrompt) section(number, title string) {
	s := styleFor(w.out)
	fmt.Fprintln(w.out, "\n  "+s.accent(number+"  "+title))
}
func (w *wizardPrompt) palette() {
	s := styleFor(w.out)
	var colors []string
	for i, p := range iconPalettes() {
		colors = append(colors, s.dim(fmt.Sprintf("%d", i+1))+" "+s.icon(p.Name))
	}
	fmt.Fprintln(w.out, "     "+strings.Join(colors, "  "))
}

func noColorSet() bool { _, set := os.LookupEnv("NO_COLOR"); return set }

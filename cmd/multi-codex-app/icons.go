package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type IconPalette struct {
	Name       string `json:"name"`
	Background string `json:"background"`
	Foreground string `json:"foreground"`
}

func iconPalettes() []IconPalette {
	b, _ := assets.ReadFile("assets/icons/palette.json")
	var palettes []IconPalette
	_ = json.Unmarshal(b, &palettes)
	return palettes
}
func validIconColor(color string) bool {
	for _, p := range iconPalettes() {
		if p.Name == color {
			return true
		}
	}
	return false
}
func iconColor(p Profile) string {
	if p.IconColor != "" {
		return p.IconColor
	}
	n, _ := strconv.Atoi(p.ID)
	if n < 1 {
		n = 1
	}
	palettes := iconPalettes()
	return palettes[(n-1)%len(palettes)].Name
}
func installIcons(root string) error {
	for _, palette := range iconPalettes() {
		for _, extension := range []string{"png", "ico"} {
			name := palette.Name + "." + extension
			if e := extractAsset("icons/"+name, filepath.Join(root, "icons", name), 0644); e != nil {
				return e
			}
		}
	}
	return nil
}
func setIcon(id, color string) error {
	color = strings.ToLower(strings.TrimSpace(color))
	if !validIconColor(color) {
		return fmt.Errorf("unknown icon color %q; choose white, yellow, blue, purple, or teal", color)
	}
	root := stateRoot()
	c, e := readConfig(root)
	if e != nil {
		return e
	}
	p, e := profile(c, id)
	if e != nil {
		return e
	}
	for i := range c.Profiles {
		if c.Profiles[i].ID == p.ID {
			c.Profiles[i].IconColor = color
		}
	}
	if e = applyAppearance(root, &c); e != nil {
		return e
	}
	fmt.Printf("Profile %s (%s) uses the %s icon.\n", p.ID, p.Name, color)
	return nil
}
func listIcons() {
	for _, p := range iconPalettes() {
		fmt.Printf("%-7s  background %s, logo %s\n", p.Name, p.Background, p.Foreground)
	}
}
func iconFile(root string, p Profile, extension string) string {
	return filepath.Join(root, "icons", iconColor(p)+"."+extension)
}

// Used by shortcut adapters; old manifests receive the same deterministic defaults.
func effectiveIconConfig(c Config) Config {
	c.Profiles = append([]Profile(nil), c.Profiles...)
	for i := range c.Profiles {
		c.Profiles[i].IconColor = iconColor(c.Profiles[i])
	}
	return c
}
func writeIconAdapterConfig(root string, c Config) (string, error) {
	b, e := json.Marshal(effectiveIconConfig(c))
	if e != nil {
		return "", e
	}
	f, e := os.CreateTemp(root, ".shortcut-config-*.json")
	if e != nil {
		return "", e
	}
	name := f.Name()
	if _, e = f.Write(b); e != nil {
		f.Close()
		os.Remove(name)
		return "", e
	}
	if e = f.Close(); e != nil {
		os.Remove(name)
		return "", e
	}
	return name, nil
}

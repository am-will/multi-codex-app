package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"math"
	"path/filepath"
	"testing"
)

func luminance(hex string) float64 {
	var r, g, b int
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	linear := func(v int) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return .2126*linear(r) + .7152*linear(g) + .0722*linear(b)
}
func TestIconContrastAndArtifacts(t *testing.T) {
	palettes := iconPalettes()
	if len(palettes) != 5 {
		t.Fatal("expected five colors")
	}
	for _, p := range palettes {
		a, b := luminance(p.Background), luminance(p.Foreground)
		if a < b {
			a, b = b, a
		}
		if ratio := (a + .05) / (b + .05); ratio < 4.5 {
			t.Errorf("%s contrast %.2f too low", p.Name, ratio)
		}
		raw, e := assets.ReadFile("assets/icons/" + p.Name + ".png")
		if e != nil {
			t.Fatal(e)
		}
		image, e := png.Decode(bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		if image.Bounds().Dx() != 1024 || image.Bounds().Dy() != 1024 {
			t.Fatal("missing full-resolution icon")
		}
		_, _, _, alpha := image.At(0, 0).RGBA()
		if alpha != 0 {
			t.Fatal("icon corners must be transparent")
		}
		raw, e = assets.ReadFile("assets/icons/" + p.Name + ".ico")
		if e != nil {
			t.Fatal(e)
		}
		if len(raw) < 102 || binary.LittleEndian.Uint16(raw[2:4]) != 1 || binary.LittleEndian.Uint16(raw[4:6]) != 6 {
			t.Fatal("missing Windows icon sizes")
		}
	}
}
func TestIconDefaultsAndPersistence(t *testing.T) {
	for i, color := range []string{"white", "yellow", "blue", "purple", "teal", "white"} {
		if got := iconColor(Profile{ID: fmt.Sprint(i + 1)}); got != color {
			t.Fatalf("default %d: %s", i+1, got)
		}
	}
	r := t.TempDir()
	p := Profile{ID: "3", Name: "Personal", CodexHome: filepath.Join(r, "auth"), UserDataDir: filepath.Join(r, "desktop"), IconColor: "blue"}
	c := Config{Version: 1, AppPath: filepath.Join(r, "app"), CLIPath: filepath.Join(r, "cli"), Profiles: []Profile{p}}
	if e := saveConfig(r, c); e != nil {
		t.Fatal(e)
	}
	got, e := readConfig(r)
	if e != nil || got.Profiles[0] != p {
		t.Fatal("appearance did not preserve identity", e)
	}
	if e := ensureProfiles(&got, 5, r, r); e != nil {
		t.Fatal(e)
	}
	if got.Profiles[0] != p {
		t.Fatal("adding profiles changed chosen color")
	}
	c.Profiles[0].IconColor = "../bad"
	if validateConfig(c) == nil {
		t.Fatal("accepted invalid icon path")
	}
	if setIcon("3", "../bad") == nil {
		t.Fatal("CLI accepted unknown color")
	}
}

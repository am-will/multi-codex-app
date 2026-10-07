package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
)

// Inspect the installed artwork rather than assuming the manifest matches it.
func inspectIcon(path string) string {
	f, e := os.Open(path)
	if e != nil {
		if os.IsNotExist(e) {
			return "missing"
		}
		return "unreadable"
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return "unreadable"
	}
	b, e := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	if e != nil || len(b) > 16<<20 {
		return "unreadable"
	}
	var frames [][]byte
	switch {
	case len(b) >= 8 && string(b[:4]) == "icns":
		for offset := 8; offset+8 <= len(b); {
			size := int(binary.BigEndian.Uint32(b[offset+4 : offset+8]))
			if size < 8 || size > len(b)-offset {
				return "custom"
			}
			frames = append(frames, b[offset+8:offset+size])
			offset += size
		}
	case len(b) >= 6 && binary.LittleEndian.Uint16(b[2:4]) == 1:
		count := int(binary.LittleEndian.Uint16(b[4:6]))
		if count > 1024 || 6+count*16 > len(b) {
			return "custom"
		}
		for i := 0; i < count; i++ {
			entry := 6 + i*16
			size := int(binary.LittleEndian.Uint32(b[entry+8 : entry+12]))
			offset := int(binary.LittleEndian.Uint32(b[entry+12 : entry+16]))
			if offset > len(b) || size > len(b)-offset {
				return "custom"
			}
			frames = append(frames, b[offset:offset+size])
		}
	default:
		frames = append(frames, b)
	}
	var largest image.Image
	for _, frame := range frames {
		c, e := png.DecodeConfig(bytes.NewReader(frame))
		if e != nil || c.Width > 4096 || c.Height > 4096 {
			continue
		}
		if largest != nil && c.Width <= largest.Bounds().Dx() {
			continue
		}
		img, e := png.Decode(bytes.NewReader(frame))
		if e == nil {
			largest = img
		}
	}
	if largest == nil {
		return "custom"
	}
	return paletteOfImage(largest)
}
func paletteOfImage(img image.Image) string {
	b := img.Bounds()
	for _, p := range iconPalettes() {
		var pr, pg, pb int
		fmt.Sscanf(p.Background, "#%02x%02x%02x", &pr, &pg, &pb)
		matches := 0
		for _, point := range [][2]int{{50, 15}, {15, 50}, {85, 50}, {50, 85}} {
			r, g, blue, a := img.At(b.Min.X+b.Dx()*point[0]/100, b.Min.Y+b.Dy()*point[1]/100).RGBA()
			distance := absInt(int(r>>8)-pr) + absInt(int(g>>8)-pg) + absInt(int(blue>>8)-pb)
			if a >= 0xf000 && distance <= 18 {
				matches++
			}
		}
		if matches == 4 {
			return p.Name
		}
	}
	return "custom"
}
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMutedShadingDoesNotGainSaturatedSpeckles(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			p := color.RGBA{50, 100, 180, 255}
			if (x/8+y/8)%2 == 1 {
				p = color.RGBA{180, 100, 50, 255}
			}
			if x >= 16 && x < 112 && y >= 16 && y < 112 {
				// A pale, faintly pink gradient like distant mountains in haze.
				level := uint8(210 + (y-16)*27/95)
				p = color.RGBA{level + 18, level, level + 11, 255}
			}
			if x >= 80 && x < 104 && y >= 24 && y < 48 {
				// A real saturated detail within the muted region must survive.
				p = color.RGBA{242, 127, 165, 255}
			}
			if x >= 80 && x < 104 && y >= 64 && y < 104 {
				// Soft skin tones must keep their warmth, unlike near-gray haze.
				p = color.RGBA{250, 224, 199, 255}
				if y < 84 {
					p = color.RGBA{218, 194, 180, 255}
				}
			}
			src.SetRGBA(x, y, p)
		}
	}
	mask, _ := detectBackground(src)
	if mask[64*128+64] {
		t.Fatal("test gradient was unexpectedly classified as flat background")
	}
	out := filepath.Join(t.TempDir(), "preview.png")
	if err := processImageWithCleanup(src, out, 1, 1, 5, true); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	shades := map[color.RGBA]bool{}
	for y := 20; y < 108; y++ {
		for x := 24; x < 72; x++ {
			p := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			if max(p.R, p.G, p.B)-min(p.R, p.G, p.B) > 20 {
				t.Fatalf("saturated speckle in muted shading at (%d,%d): %v", x, y, p)
			}
			shades[p] = true
		}
	}
	if len(shades) < 2 {
		t.Fatalf("lost tonal variation in gradient: %d shades", len(shades))
	}
	for y := 28; y < 44; y++ {
		for x := 84; x < 100; x++ {
			if p := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA); p != (color.RGBA{242, 127, 165, 255}) {
				t.Fatalf("real pink detail changed at (%d,%d): %v", x, y, p)
			}
		}
	}
	for _, startY := range []int{68, 88} {
		warmPixels := 0
		for y := startY; y < startY+12; y++ {
			for x := 84; x < 100; x++ {
				p := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
				if int(p.R)-int(p.B) > 40 {
					warmPixels++
				}
			}
		}
		if warmPixels < 16*12/10 {
			t.Fatalf("soft skin tone at y=%d lost its warmth: only %d warm pixels", startY, warmPixels)
		}
	}
	allowed := map[color.RGBA]bool{}
	for _, p := range palette {
		allowed[color.RGBA{uint8(math.Round(p.R)), uint8(math.Round(p.G)), uint8(math.Round(p.B)), 255}] = true
	}
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if p := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA); !allowed[p] {
				t.Fatalf("pixel outside map palette at (%d,%d): %v", x, y, p)
			}
		}
	}
}
